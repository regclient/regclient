package main

import (
	"context"
	"os"
	"sync"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/cmd/regbot/sandbox"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/throttle"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	usageDesc = `Utility for automating repository actions
More details at https://github.com/regclient/regclient`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regbot"
)

type rootCmd struct {
	confFile  string
	dryRun    bool
	verbosity string
	logopts   []string
	format    string // for Go template formatting of various commands
}

// TODO: remove these globals
var (
	conf      *Config
	log       *logrus.Logger
	rc        *regclient.RegClient
	throttleC *throttle.Throttle
)

func init() {
	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.InfoLevel,
	}
}

func NewRootCmd() *cobra.Command {
	rootOpts := rootCmd{}
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
	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.InfoLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	_ = rootTopCmd.MarkPersistentFlagFilename("config")
	_ = serverCmd.MarkPersistentFlagRequired("config")
	_ = onceCmd.MarkPersistentFlagRequired("config")

	rootTopCmd.AddCommand(serverCmd)
	rootTopCmd.AddCommand(onceCmd)
	rootTopCmd.AddCommand(versionCmd)

	rootTopCmd.PersistentPreRunE = rootOpts.rootPreRun
	return rootTopCmd
}

func (rootOpts *rootCmd) rootPreRun(cmd *cobra.Command, args []string) error {
	lvl, err := logrus.ParseLevel(rootOpts.verbosity)
	if err != nil {
		return err
	}
	log.SetLevel(lvl)
	log.Formatter = &logrus.TextFormatter{FullTimestamp: true}
	for _, opt := range rootOpts.logopts {
		if opt == "json" {
			log.Formatter = new(logrus.JSONFormatter)
		}
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
	for _, s := range conf.Scripts {
		s := s
		if conf.Defaults.Parallel > 0 {
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
	for _, s := range conf.Scripts {
		s := s
		sched := s.Schedule
		if sched == "" && s.Interval != 0 {
			sched = "@every " + s.Interval.String()
		}
		if sched != "" {
			log.WithFields(logrus.Fields{
				"name":  s.Name,
				"sched": sched,
			}).Debug("Scheduled task")
			_, errCron := c.AddFunc(sched, func() {
				log.WithFields(logrus.Fields{
					"name": s.Name,
				}).Debug("Running task")
				wg.Add(1)
				defer wg.Done()
				err := rootOpts.process(ctx, s)
				if mainErr == nil {
					mainErr = err
				}
			})
			if errCron != nil {
				log.WithFields(logrus.Fields{
					"name":  s.Name,
					"sched": sched,
					"err":   errCron,
				}).Error("Failed to schedule cron")
				if mainErr != nil {
					mainErr = errCron
				}
			}
		} else {
			log.WithFields(logrus.Fields{
				"name": s.Name,
			}).Error("No schedule or interval found, ignoring")
		}
	}
	c.Start()
	// wait on interrupt signal
	done := ctx.Done()
	if done != nil {
		<-done
	}
	log.WithFields(logrus.Fields{}).Info("Stopping server")
	// clean shutdown
	c.Stop()
	log.WithFields(logrus.Fields{}).Debug("Waiting on running tasks")
	wg.Wait()
	return mainErr
}

func (rootOpts *rootCmd) loadConf() error {
	var err error
	if rootOpts.confFile == "-" {
		conf, err = ConfigLoadReader(os.Stdin)
		if err != nil {
			return err
		}
	} else if rootOpts.confFile != "" {
		r, err := os.Open(rootOpts.confFile)
		if err != nil {
			return err
		}
		defer r.Close()
		conf, err = ConfigLoadReader(r)
		if err != nil {
			return err
		}
	} else {
		return ErrMissingInput
	}
	// use a throttle to control parallelism
	concurrent := conf.Defaults.Parallel
	if concurrent <= 0 {
		concurrent = 1
	}
	log.WithFields(logrus.Fields{
		"concurrent": concurrent,
	}).Debug("Configuring parallel settings")
	throttleC = throttle.New(concurrent)
	// set the regclient, loading docker creds unless disabled, and inject logins from config file
	rcOpts := []regclient.Opt{
		regclient.WithLog(log),
	}
	if conf.Defaults.BlobLimit != 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithBlobLimit(conf.Defaults.BlobLimit)))
	}
	if !conf.Defaults.SkipDockerConf {
		rcOpts = append(rcOpts, regclient.WithDockerCreds(), regclient.WithDockerCerts())
	}
	if conf.Defaults.UserAgent != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(conf.Defaults.UserAgent))
	} else {
		info := version.GetInfo()
		if info.VCSTag != "" {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSTag+")"))
		} else {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSRef+")"))
		}
	}
	rcHosts := []config.Host{}
	for _, host := range conf.Creds {
		if host.Scheme != "" {
			log.WithFields(logrus.Fields{
				"name": host.Name,
			}).Warn("Scheme is deprecated, for http set TLS to disabled")
		}
		rcHosts = append(rcHosts, host)
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHost(rcHosts...))
	}
	rc = regclient.New(rcOpts...)
	return nil
}

// process a sync step
func (rootOpts *rootCmd) process(ctx context.Context, s ConfigScript) error {
	log.WithFields(logrus.Fields{
		"script": s.Name,
	}).Debug("Starting script")
	// add a timeout to the context
	if s.Timeout > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, s.Timeout)
		ctx = ctxTimeout
		defer cancel()
	}
	sbOpts := []sandbox.Opt{
		sandbox.WithContext(ctx),
		sandbox.WithRegClient(rc),
		sandbox.WithLog(log),
		sandbox.WithThrottle(throttleC),
	}
	if rootOpts.dryRun {
		sbOpts = append(sbOpts, sandbox.WithDryRun())
	}
	sb := sandbox.New(s.Name, sbOpts...)
	defer sb.Close()
	err := sb.RunScript(s.Script)
	if err != nil {
		log.WithFields(logrus.Fields{
			"script": s.Name,
			"error":  err,
		}).Warn("Error running script")
		return ErrScriptFailed
	}
	log.WithFields(logrus.Fields{
		"script": s.Name,
	}).Debug("Finished script")

	return nil
}
