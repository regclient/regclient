package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/cmd/regbot/sandbox"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

const (
	usageDesc = `Utility for automating repository actions
More details at https://github.com/regclient/regclient`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regbot"
)

var rootOpts struct {
	confFile  string
	dryRun    bool
	verbosity string
	logopts   []string
	format    string // for Go template formatting of various commands
}

var (
	conf *Config
	log  *logrus.Logger
	rc   *regclient.RegClient
	sem  *semaphore.Weighted
)

var rootCmd = &cobra.Command{
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
	RunE:  runServer,
}
var onceCmd = &cobra.Command{
	Use:   "once",
	Short: "runs each script once",
	Long: `Each script is executed once ignoring any scheduling. The command
returns after the last script completes.`,
	Args: cobra.RangeArgs(0, 0),
	RunE: runOnce,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the version",
	Long:  `Show the version`,
	Args:  cobra.RangeArgs(0, 0),
	RunE:  runVersion,
}

func init() {
	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.InfoLevel,
	}
	rootCmd.PersistentFlags().StringVarP(&rootOpts.confFile, "config", "c", "", "Config file")
	rootCmd.PersistentFlags().BoolVarP(&rootOpts.dryRun, "dry-run", "", false, "Dry Run, skip all external actions")
	rootCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.InfoLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	rootCmd.MarkPersistentFlagFilename("config")
	serverCmd.MarkPersistentFlagRequired("config")
	onceCmd.MarkPersistentFlagRequired("config")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(onceCmd)
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentPreRunE = rootPreRun
}

func rootPreRun(cmd *cobra.Command, args []string) error {
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

func runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()
	return template.Writer(os.Stdout, rootOpts.format, info)
}

// runOnce processes the file in one pass, ignoring cron
func runOnce(cmd *cobra.Command, args []string) error {
	err := loadConf()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	// handle interrupt signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.WithFields(logrus.Fields{}).Debug("Interrupt received, stopping")
		// clean shutdown
		cancel()
	}()
	var wg sync.WaitGroup
	var mainErr error
	for _, s := range conf.Scripts {
		s := s
		if conf.Defaults.Parallel > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.process(ctx)
				if err != nil {
					if mainErr == nil {
						mainErr = err
					}
					return
				}
			}()
		} else {
			err := s.process(ctx)
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
func runServer(cmd *cobra.Command, args []string) error {
	err := loadConf()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(cmd.Context())
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
			c.AddFunc(sched, func() {
				log.WithFields(logrus.Fields{
					"name": s.Name,
				}).Debug("Running task")
				wg.Add(1)
				defer wg.Done()
				err := s.process(ctx)
				if mainErr == nil {
					mainErr = err
				}
			})
		} else {
			log.WithFields(logrus.Fields{
				"name": s.Name,
			}).Error("No schedule or interval found, ignoring")
		}
	}
	c.Start()
	// wait on interrupt signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.WithFields(logrus.Fields{}).Debug("Interrupt received, stopping")
	// clean shutdown
	c.Stop()
	cancel()
	log.WithFields(logrus.Fields{}).Debug("Waiting on running tasks")
	wg.Wait()
	return mainErr
}

func loadConf() error {
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
	// use a semaphore to control parallelism
	concurrent := int64(conf.Defaults.Parallel)
	if concurrent <= 0 {
		concurrent = 1
	}
	log.WithFields(logrus.Fields{
		"concurrent": concurrent,
	}).Debug("Configuring parallel settings")
	sem = semaphore.NewWeighted(concurrent)
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
func (s ConfigScript) process(ctx context.Context) error {
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
		sandbox.WithSemaphore(sem),
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
