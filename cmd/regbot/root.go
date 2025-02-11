package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/cmd/regbot/sandbox"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/cobradoc"
	"github.com/regclient/regclient/internal/pqueue"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
)

const (
	usageDesc = `Utility for automating repository actions
More details at <https://github.com/regclient/regclient>`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regbot"
)

type rootCmd struct {
	confFile  string
	dryRun    bool
	verbosity string
	logopts   []string
	format    string // for Go template formatting of various commands
	log       *slog.Logger
	conf      *Config
	rc        *regclient.RegClient
	throttle  *pqueue.Queue[struct{}]
}

func NewRootCmd() (*cobra.Command, *rootCmd) {
	rootOpts := rootCmd{
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
	var rootTopCmd = &cobra.Command{
		Use:           "regbot <cmd>",
		Short:         "Utility for automating repository actions",
		Long:          usageDesc,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "run the regbot server",
		Long:  `Runs the various scripts according to their schedule.`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  rootOpts.runServer,
	}
	var onceCmd = &cobra.Command{
		Use:   "once",
		Short: "runs each script once",
		Long: `Each script is executed once ignoring any scheduling. The command
returns after the last script completes.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: rootOpts.runOnce,
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  `Show the version`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  rootOpts.runVersion,
	}

	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.confFile, "config", "c", "", "Config file")
	rootTopCmd.PersistentFlags().BoolVarP(&rootOpts.dryRun, "dry-run", "", false, "Dry Run, skip all external actions")
	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", slog.LevelInfo.String(), "Log level (trace, debug, info, warn, error)")
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	_ = rootTopCmd.MarkPersistentFlagFilename("config")
	_ = serverCmd.MarkPersistentFlagRequired("config")
	_ = onceCmd.MarkPersistentFlagRequired("config")

	rootTopCmd.AddCommand(serverCmd)
	rootTopCmd.AddCommand(onceCmd)
	rootTopCmd.AddCommand(versionCmd)
	rootTopCmd.AddCommand(cobradoc.NewCmd(rootTopCmd.Name(), "cli-doc"))

	rootTopCmd.PersistentPreRunE = rootOpts.rootPreRun
	return rootTopCmd, &rootOpts
}

func (rootOpts *rootCmd) rootPreRun(cmd *cobra.Command, args []string) error {
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(rootOpts.verbosity))
	if err != nil {
		// handle custom levels
		if rootOpts.verbosity == strings.ToLower("trace") {
			lvl = types.LevelTrace
		} else {
			return fmt.Errorf("unable to parse verbosity %s: %v", rootOpts.verbosity, err)
		}
	}
	formatJSON := false
	for _, opt := range rootOpts.logopts {
		if opt == "json" {
			formatJSON = true
		}
	}
	if formatJSON {
		rootOpts.log = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	} else {
		rootOpts.log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	}
	return nil
}

func (rootOpts *rootCmd) runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()
	return template.Writer(os.Stdout, rootOpts.format, info)
}

// runOnce processes the file in one pass, ignoring cron
func (rootOpts *rootCmd) runOnce(cmd *cobra.Command, args []string) error {
	err := rootOpts.loadConf()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var wg sync.WaitGroup
	var mainErr error
	for _, s := range rootOpts.conf.Scripts {
		s := s
		if rootOpts.conf.Defaults.Parallel > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := rootOpts.process(ctx, s)
				if err != nil {
					if mainErr == nil {
						mainErr = err
					}
					return
				}
			}()
		} else {
			err := rootOpts.process(ctx, s)
			if err != nil {
				if mainErr == nil {
					mainErr = err
				}
			}
		}
	}
	wg.Wait()
	return mainErr
}

// runServer stays running with cron scheduled tasks
func (rootOpts *rootCmd) runServer(cmd *cobra.Command, args []string) error {
	err := rootOpts.loadConf()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var wg sync.WaitGroup
	var mainErr error
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	for _, s := range rootOpts.conf.Scripts {
		s := s
		sched := s.Schedule
		if sched == "" && s.Interval != 0 {
			sched = "@every " + s.Interval.String()
		}
		if sched != "" {
			rootOpts.log.Debug("Scheduled task",
				slog.String("name", s.Name),
				slog.String("sched", sched))
			_, errCron := c.AddFunc(sched, func() {
				rootOpts.log.Debug("Running task",
					slog.String("name", s.Name))
				wg.Add(1)
				defer wg.Done()
				err := rootOpts.process(ctx, s)
				if mainErr == nil {
					mainErr = err
				}
			})
			if errCron != nil {
				rootOpts.log.Error("Failed to schedule cron",
					slog.String("name", s.Name),
					slog.String("sched", sched),
					slog.String("err", errCron.Error()))
				if mainErr != nil {
					mainErr = errCron
				}
			}
		} else {
			rootOpts.log.Error("No schedule or interval found, ignoring",
				slog.String("name", s.Name))
		}
	}
	c.Start()
	// wait on interrupt signal
	done := ctx.Done()
	if done != nil {
		<-done
	}
	rootOpts.log.Info("Stopping server")
	// clean shutdown
	c.Stop()
	rootOpts.log.Debug("Waiting on running tasks")
	wg.Wait()
	return mainErr
}

func (rootOpts *rootCmd) loadConf() error {
	var err error
	if rootOpts.confFile == "-" {
		rootOpts.conf, err = ConfigLoadReader(os.Stdin)
		if err != nil {
			return err
		}
	} else if rootOpts.confFile != "" {
		r, err := os.Open(rootOpts.confFile)
		if err != nil {
			return err
		}
		defer r.Close()
		rootOpts.conf, err = ConfigLoadReader(r)
		if err != nil {
			return err
		}
	} else {
		return ErrMissingInput
	}
	// use a throttle to control parallelism
	concurrent := rootOpts.conf.Defaults.Parallel
	if concurrent <= 0 {
		concurrent = 1
	}
	rootOpts.log.Debug("Configuring parallel settings",
		slog.Int("concurrent", concurrent))
	rootOpts.throttle = pqueue.New(pqueue.Opts[struct{}]{Max: concurrent})
	// set the regclient, loading docker creds unless disabled, and inject logins from config file
	rcOpts := []regclient.Opt{
		regclient.WithSlog(rootOpts.log),
	}
	if rootOpts.conf.Defaults.BlobLimit != 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithBlobLimit(rootOpts.conf.Defaults.BlobLimit)))
	}
	if !rootOpts.conf.Defaults.SkipDockerConf {
		rcOpts = append(rcOpts, regclient.WithDockerCreds(), regclient.WithDockerCerts())
	}
	if rootOpts.conf.Defaults.UserAgent != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(rootOpts.conf.Defaults.UserAgent))
	} else {
		info := version.GetInfo()
		if info.VCSTag != "" {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSTag+")"))
		} else {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSRef+")"))
		}
	}
	rcHosts := []config.Host{}
	for _, host := range rootOpts.conf.Creds {
		if host.Scheme != "" {
			rootOpts.log.Warn("Scheme is deprecated, for http set TLS to disabled",
				slog.String("name", host.Name))
		}
		rcHosts = append(rcHosts, host)
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHost(rcHosts...))
	}
	rootOpts.rc = regclient.New(rcOpts...)
	return nil
}

// process a sync step
func (rootOpts *rootCmd) process(ctx context.Context, s ConfigScript) error {
	rootOpts.log.Debug("Starting script",
		slog.String("script", s.Name))
	// add a timeout to the context
	if s.Timeout > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, s.Timeout)
		ctx = ctxTimeout
		defer cancel()
	}
	sbOpts := []sandbox.Opt{
		sandbox.WithContext(ctx),
		sandbox.WithRegClient(rootOpts.rc),
		sandbox.WithSlog(rootOpts.log),
		sandbox.WithThrottle(rootOpts.throttle),
	}
	if rootOpts.dryRun {
		sbOpts = append(sbOpts, sandbox.WithDryRun())
	}
	sb := sandbox.New(s.Name, sbOpts...)
	defer sb.Close()
	err := sb.RunScript(s.Script)
	if err != nil {
		rootOpts.log.Warn("Error running script",
			slog.String("script", s.Name),
			slog.String("error", err.Error()))
		return ErrScriptFailed
	}
	rootOpts.log.Debug("Finished script",
		slog.String("script", s.Name))
	return nil
}
