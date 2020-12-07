package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/regclient/regclient/cmd/regbot/sandbox"
	"github.com/regclient/regclient/regclient"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

const usageDesc = `Utility for mirroring docker repositories
More details at https://github.com/regclient/regclient`

var rootOpts struct {
	confFile  string
	verbosity string
	logopts   []string
}

var config *Config
var log *logrus.Logger
var rc regclient.RegClient
var sem *semaphore.Weighted

var rootCmd = &cobra.Command{
	Use:   "regbot <cmd>",
	Short: "Utility for automating repository actions",
	Long:  usageDesc,
	// Run: func(cmd *cobra.Command, args []string) {
	// 	// Do Stuff Here
	// },
	// RunE: runServer,
}
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run the regbot server",
	Long:  `Runs the various scripts according to their schedule.`,
	Args:  cobra.RangeArgs(0, 0),
	RunE:  runServer,
}
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "runs each script once in a dry-run mode",
	Long: `Each script is executed once, and commands that would take external
action are only logged rather than executed. The command returns after the last
script completes.`,
	Args: cobra.RangeArgs(0, 0),
	RunE: runCheck,
}
var onceCmd = &cobra.Command{
	Use:   "once",
	Short: "runs each script once",
	Long: `Each script is executed once ignoring any scheduling. The command
returns after the last script completes.`,
	Args: cobra.RangeArgs(0, 0),
	RunE: runOnce,
}

func init() {
	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.InfoLevel,
	}
	rootCmd.PersistentFlags().StringVarP(&rootOpts.confFile, "config", "c", "", "Config file")
	rootCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.InfoLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	rootCmd.MarkPersistentFlagFilename("config")
	rootCmd.MarkPersistentFlagRequired("config")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(onceCmd)
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
	if rootOpts.confFile == "-" {
		config, err = ConfigLoadReader(os.Stdin)
		if err != nil {
			return err
		}
	} else {
		r, err := os.Open(rootOpts.confFile)
		if err != nil {
			return err
		}
		defer r.Close()
		config, err = ConfigLoadReader(r)
		if err != nil {
			return err
		}
	}
	// use a semaphore to control parallelism
	log.WithFields(logrus.Fields{
		"parallel": config.Defaults.Parallel,
	}).Debug("Configuring parallel settings")
	sem = semaphore.NewWeighted(int64(config.Defaults.Parallel))
	// set the regclient, loading docker creds unless disabled, and inject logins from config file
	rcOpts := []regclient.Opt{regclient.WithLog(log)}
	if !config.Defaults.SkipDockerConf {
		rcOpts = append(rcOpts, regclient.WithDockerCreds(), regclient.WithDockerCerts())
	}
	rcHosts := []regclient.ConfigHost{}
	for _, host := range config.Creds {
		rcHosts = append(rcHosts, regclient.ConfigHost{
			Name:    host.Registry,
			User:    host.User,
			Pass:    host.Pass,
			TLS:     host.TLS,
			Scheme:  host.Scheme,
			RegCert: host.RegCert,
		})
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHosts(rcHosts))
	}
	rc = regclient.NewRegClient(rcOpts...)
	return nil
}

// runOnce processes the file in one pass, ignoring cron
func runOnce(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var mainErr error
	for _, s := range config.Scripts {
		s := s
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
	}
	// wait on interrupt signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.WithFields(logrus.Fields{}).Debug("Interrupt received, stopping")
		// clean shutdown
		cancel()
	}()
	wg.Wait()
	return mainErr
}

// runServer stays running with cron scheduled tasks
func runServer(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var mainErr error
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	for _, s := range config.Scripts {
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

// run check is used for a dry-run
func runCheck(cmd *cobra.Command, args []string) error {
	var mainErr error
	ctx := context.Background()
	for _, s := range config.Scripts {
		err := s.process(ctx)
		if err != nil {
			if mainErr == nil {
				mainErr = err
			}
		}
	}
	return mainErr
}

// process a sync step
func (s ConfigScript) process(ctx context.Context) error {
	log.WithFields(logrus.Fields{
		"name": s.Name,
	}).Debug("Starting script")
	sb := sandbox.New(ctx, rc, log)
	defer sb.Close()
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(logrus.Fields{
				"name":  s.Name,
				"error": r,
			}).Error("Runtime error from script")
		}
	}()
	err := sb.RunScript(s.Script)
	if err != nil {
		log.WithFields(logrus.Fields{
			"name":  s.Name,
			"error": err,
		}).Debug("Error running script")
		return ErrScriptFailed
	}
	log.WithFields(logrus.Fields{
		"name": s.Name,
	}).Debug("Finished script")

	return nil
}
