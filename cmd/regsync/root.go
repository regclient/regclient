package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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
	Use:   "regsync <cmd>",
	Short: "Utility for mirroring docker repositories",
	Long:  usageDesc,
	// Run: func(cmd *cobra.Command, args []string) {
	// 	// Do Stuff Here
	// },
	// RunE: runServer,
}
var serverCmd = &cobra.Command{
	Use: "server",
	// Aliases: []string{"list"},
	Short: "run the regsync server",
	Long:  `Sync registries according to the configuration.`,
	Args:  cobra.RangeArgs(0, 0),
	RunE:  runServer,
}
var checkCmd = &cobra.Command{
	Use: "check",
	// Aliases: []string{"list"},
	Short: "processes each sync command once but skip actual copy",
	Long: `Processes each sync command in the configuration file in order.
Manifests are checked to see if a copy is needed, but only log, skip copying.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
	Args: cobra.RangeArgs(0, 0),
	RunE: runCheck,
}
var onceCmd = &cobra.Command{
	Use: "once",
	// Aliases: []string{"list"},
	Short: "processes each sync command once, ignoring cron schedule",
	Long: `Processes each sync command in the configuration file in order.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
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
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, s := range config.Sync {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.process(ctx, "copy")
			if err != nil {
				log.WithFields(logrus.Fields{
					"source": s.Source,
					"target": s.Target,
					"type":   s.Type,
					"error":  err,
				}).Error("Failed processing sync")
				return
			}
		}()
	}
	wg.Wait()
	return nil
}

// runServer stays running with cron scheduled tasks
func runServer(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	for _, s := range config.Sync {
		s := s
		sched := s.Schedule
		if sched == "" && s.Interval != 0 {
			sched = "@every " + s.Interval.String()
		}
		if sched != "" {
			log.WithFields(logrus.Fields{
				"source": s.Source,
				"target": s.Target,
				"type":   s.Type,
				"sched":  sched,
			}).Debug("Scheduled task")
			c.AddFunc(sched, func() {
				log.WithFields(logrus.Fields{
					"source": s.Source,
					"target": s.Target,
					"type":   s.Type,
				}).Debug("Running task")
				wg.Add(1)
				defer wg.Done()
				s.process(ctx, "copy")
			})
		} else {
			log.WithFields(logrus.Fields{
				"source": s.Source,
				"target": s.Target,
				"type":   s.Type,
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
	return nil
}

// run check is used for a dry-run
func runCheck(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	for _, s := range config.Sync {
		err := s.process(ctx, "check")
		if err != nil {
			return err
		}
	}

	return nil
}

// process a sync step
func (s ConfigSync) process(ctx context.Context, action string) error {
	switch s.Type {
	case "repository":
		sRepoRef, err := regclient.NewRef(s.Source)
		if err != nil {
			return err
		}
		sTags, err := rc.TagList(ctx, sRepoRef)
		if err != nil {
			return err
		}
		tRepoRef, err := regclient.NewRef(s.Target)
		if err != nil {
			return err
		}
		for _, tag := range sTags.Tags {
			sRef := sRepoRef
			sRef.Tag = tag
			tRef := tRepoRef
			tRef.Tag = tag
			err = s.processRef(ctx, sRef, tRef, action)
			if err != nil {
				return err
			}
		}

	case "image":
		sRef, err := regclient.NewRef(s.Source)
		if err != nil {
			return err
		}
		tRef, err := regclient.NewRef(s.Target)
		if err != nil {
			return err
		}
		err = s.processRef(ctx, sRef, tRef, action)
		if err != nil {
			return err
		}

	default:
		log.WithFields(logrus.Fields{
			"action": action,
		}).Error("Unhandled action")
		return ErrInvalidInput
	}
	return nil
}

// process a sync step
func (s ConfigSync) processRef(ctx context.Context, src, tgt regclient.Ref, action string) error {
	mSrc, err := rc.ManifestHead(ctx, src)
	if err != nil {
		return err
	}
	mTgt, err := rc.ManifestHead(ctx, tgt)
	if err == nil && mSrc.GetDigest().String() == mTgt.GetDigest().String() {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Debug("Image matches")
		return nil
	}
	log.WithFields(logrus.Fields{
		"source": src.CommonName(),
		"target": tgt.CommonName(),
	}).Info("Image sync needed")
	if action == "check" {
		return nil
	}

	// wait for parallel tasks
	sem.Acquire(ctx, 1)
	// delay for rate limit on source
	if s.RateLimit.Min > 0 && mSrc.GetRateLimit().Set {
		// refresh current rate limit
		mSrc, err = rc.ManifestHead(ctx, src)
		if err != nil {
			return err
		}
		// delay if rate limit exceeded
		rlSrc := mSrc.GetRateLimit()
		for rlSrc.Remain < s.RateLimit.Min {
			sem.Release(1)
			time.Sleep(s.RateLimit.Retry)
			sem.Acquire(ctx, 1)
			mSrc, err = rc.ManifestHead(ctx, src)
			if err != nil {
				return err
			}
			rlSrc = mSrc.GetRateLimit()
		}
		log.WithFields(logrus.Fields{
			"source":        src.CommonName(),
			"source-remain": rlSrc.Remain,
			"step-limit":    s.RateLimit.Min,
		}).Debug("Rate limit passed")
	}
	defer sem.Release(1)
	// verify context has not been canceled
	select {
	case <-ctx.Done():
		return ErrCanceled
	default:
	}
	log.WithFields(logrus.Fields{
		"source": src.CommonName(),
		"target": tgt.CommonName(),
	}).Debug("Image sync running")
	return rc.ImageCopy(ctx, src, tgt)
}
