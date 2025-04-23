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

type rootOpts struct {
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

func NewRootCmd() (*cobra.Command, *rootOpts) {
	opts := rootOpts{
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
	cmd := &cobra.Command{
		Use:               "regbot <cmd>",
		Short:             "Utility for automating repository actions",
		Long:              usageDesc,
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: opts.rootPreRun,
	}
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "run the regbot server",
		Long:  `Runs the various scripts according to their schedule.`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  opts.runServer,
	}
	onceCmd := &cobra.Command{
		Use:   "once",
		Short: "runs each script once",
		Long: `Each script is executed once ignoring any scheduling. The command
returns after the last script completes.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: opts.runOnce,
	}
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  `Show the version`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  opts.runVersion,
	}

	cmd.PersistentFlags().StringArrayVar(&opts.logopts, "logopt", []string{}, "Log options")
	cmd.PersistentFlags().StringVarP(&opts.verbosity, "verbosity", "v", slog.LevelInfo.String(), "Log level (trace, debug, info, warn, error)")

	for _, curCmd := range []*cobra.Command{serverCmd, onceCmd} {
		curCmd.Flags().StringVarP(&opts.confFile, "config", "c", "", "Config file")
		_ = curCmd.MarkFlagFilename("config")
		_ = curCmd.MarkFlagRequired("config")
		curCmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "", false, "Dry Run, skip all external actions")
	}

	versionCmd.Flags().StringVarP(&opts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = versionCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	})

	cmd.AddCommand(
		serverCmd,
		onceCmd,
		versionCmd,
		cobradoc.NewCmd(cmd.Name(), "cli-doc"),
	)

	return cmd, &opts
}

func (opts *rootOpts) rootPreRun(cmd *cobra.Command, args []string) error {
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(opts.verbosity))
	if err != nil {
		// handle custom levels
		if opts.verbosity == strings.ToLower("trace") {
			lvl = types.LevelTrace
		} else {
			return fmt.Errorf("unable to parse verbosity %s: %v", opts.verbosity, err)
		}
	}
	formatJSON := false
	for _, opt := range opts.logopts {
		if opt == "json" {
			formatJSON = true
		}
	}
	if formatJSON {
		opts.log = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	} else {
		opts.log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	}
	return nil
}

func (opts *rootOpts) runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()
	return template.Writer(os.Stdout, opts.format, info)
}

// runOnce processes the file in one pass, ignoring cron
func (opts *rootOpts) runOnce(cmd *cobra.Command, args []string) error {
	err := opts.loadConf()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var wg sync.WaitGroup
	var mainErr error
	for _, s := range opts.conf.Scripts {
		if opts.conf.Defaults.Parallel > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := opts.process(ctx, s)
				if err != nil {
					if mainErr == nil {
						mainErr = err
					}
					return
				}
			}()
		} else {
			err := opts.process(ctx, s)
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
func (opts *rootOpts) runServer(cmd *cobra.Command, args []string) error {
	err := opts.loadConf()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var wg sync.WaitGroup
	var mainErr error
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	for _, s := range opts.conf.Scripts {
		sched := s.Schedule
		if sched == "" && s.Interval != 0 {
			sched = "@every " + s.Interval.String()
		}
		if sched != "" {
			opts.log.Debug("Scheduled task",
				slog.String("name", s.Name),
				slog.String("sched", sched))
			_, errCron := c.AddFunc(sched, func() {
				opts.log.Debug("Running task",
					slog.String("name", s.Name))
				wg.Add(1)
				defer wg.Done()
				err := opts.process(ctx, s)
				if mainErr == nil {
					mainErr = err
				}
			})
			if errCron != nil {
				opts.log.Error("Failed to schedule cron",
					slog.String("name", s.Name),
					slog.String("sched", sched),
					slog.String("err", errCron.Error()))
				if mainErr != nil {
					mainErr = errCron
				}
			}
		} else {
			opts.log.Error("No schedule or interval found, ignoring",
				slog.String("name", s.Name))
		}
	}
	c.Start()
	// wait on interrupt signal
	done := ctx.Done()
	if done != nil {
		<-done
	}
	opts.log.Info("Stopping server")
	// clean shutdown
	c.Stop()
	opts.log.Debug("Waiting on running tasks")
	wg.Wait()
	return mainErr
}

func (opts *rootOpts) loadConf() error {
	var err error
	if opts.confFile == "-" {
		opts.conf, err = ConfigLoadReader(os.Stdin)
		if err != nil {
			return err
		}
	} else if opts.confFile != "" {
		r, err := os.Open(opts.confFile)
		if err != nil {
			return err
		}
		defer r.Close()
		opts.conf, err = ConfigLoadReader(r)
		if err != nil {
			return err
		}
	} else {
		return ErrMissingInput
	}
	// use a throttle to control parallelism
	concurrent := opts.conf.Defaults.Parallel
	if concurrent <= 0 {
		concurrent = 1
	}
	opts.log.Debug("Configuring parallel settings",
		slog.Int("concurrent", concurrent))
	opts.throttle = pqueue.New(pqueue.Opts[struct{}]{Max: concurrent})
	// set the regclient, loading docker creds unless disabled, and inject logins from config file
	rcOpts := []regclient.Opt{
		regclient.WithSlog(opts.log),
	}
	if opts.conf.Defaults.BlobLimit != 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithBlobLimit(opts.conf.Defaults.BlobLimit)))
	}
	if !opts.conf.Defaults.SkipDockerConf {
		rcOpts = append(rcOpts, regclient.WithDockerCreds(), regclient.WithDockerCerts())
	}
	if opts.conf.Defaults.UserAgent != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(opts.conf.Defaults.UserAgent))
	} else {
		info := version.GetInfo()
		if info.VCSTag != "" {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSTag+")"))
		} else {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSRef+")"))
		}
	}
	rcHosts := []config.Host{}
	for _, host := range opts.conf.Creds {
		if host.Scheme != "" {
			opts.log.Warn("Scheme is deprecated, for http set TLS to disabled",
				slog.String("name", host.Name))
		}
		rcHosts = append(rcHosts, host)
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHost(rcHosts...))
	}
	opts.rc = regclient.New(rcOpts...)
	return nil
}

// process a sync step
func (opts *rootOpts) process(ctx context.Context, s ConfigScript) error {
	opts.log.Debug("Starting script",
		slog.String("script", s.Name))
	// add a timeout to the context
	if s.Timeout > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, s.Timeout)
		ctx = ctxTimeout
		defer cancel()
	}
	sbOpts := []sandbox.Opt{
		sandbox.WithContext(ctx),
		sandbox.WithRegClient(opts.rc),
		sandbox.WithSlog(opts.log),
		sandbox.WithThrottle(opts.throttle),
	}
	if opts.dryRun {
		sbOpts = append(sbOpts, sandbox.WithDryRun())
	}
	sb := sandbox.New(s.Name, sbOpts...)
	defer sb.Close()
	err := sb.RunScript(s.Script)
	if err != nil {
		opts.log.Warn("Error running script",
			slog.String("script", s.Name),
			slog.String("error", err.Error()))
		return fmt.Errorf("%w%.0w", err, ErrScriptFailed)
	}
	opts.log.Debug("Finished script",
		slog.String("script", s.Name))
	return nil
}
