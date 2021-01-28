package main

import (
	"context"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd/platforms"
	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/template"
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
	Use:           "regsync <cmd>",
	Short:         "Utility for mirroring docker repositories",
	Long:          usageDesc,
	SilenceUsage:  true,
	SilenceErrors: true,
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
	regclient.UserAgent = "regclient/regsync"
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
	for _, s := range config.Sync {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.process(ctx, "copy")
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
				err := s.process(ctx, "copy")
				if mainErr == nil {
					mainErr = err
				}
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
	return mainErr
}

// run check is used for a dry-run
func runCheck(cmd *cobra.Command, args []string) error {
	var mainErr error
	ctx := context.Background()
	for _, s := range config.Sync {
		err := s.process(ctx, "check")
		if err != nil {
			if mainErr == nil {
				mainErr = err
			}
		}
	}
	return mainErr
}

// process a sync step
func (s ConfigSync) process(ctx context.Context, action string) error {
	switch s.Type {
	case "repository":
		sRepoRef, err := regclient.NewRef(s.Source)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": s.Source,
				"error":  err,
			}).Error("Failed parsing source")
			return err
		}
		sTags, err := rc.TagList(ctx, sRepoRef)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": sRepoRef.CommonName(),
				"error":  err,
			}).Error("Failed getting source tags")
			return err
		}
		sTagList, err := s.filterTags(sTags.Tags)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": sRepoRef.CommonName(),
				"allow":  s.Tags.Allow,
				"deny":   s.Tags.Deny,
				"error":  err,
			}).Error("Failed processing tag filters")
			return err
		}
		if len(sTagList) == 0 {
			log.WithFields(logrus.Fields{
				"source":    sRepoRef.CommonName(),
				"allow":     s.Tags.Allow,
				"deny":      s.Tags.Deny,
				"available": sTags.Tags,
			}).Warn("No matching tags found")
			return nil
		}
		tRepoRef, err := regclient.NewRef(s.Target)
		if err != nil {
			log.WithFields(logrus.Fields{
				"target": s.Target,
				"error":  err,
			}).Error("Failed parsing target")
			return err
		}
		for _, tag := range sTagList {
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
			log.WithFields(logrus.Fields{
				"source": s.Source,
				"error":  err,
			}).Error("Failed parsing source")
			return err
		}
		tRef, err := regclient.NewRef(s.Target)
		if err != nil {
			log.WithFields(logrus.Fields{
				"target": s.Target,
				"error":  err,
			}).Error("Failed parsing target")
			return err
		}
		err = s.processRef(ctx, sRef, tRef, action)
		if err != nil {
			return err
		}

	default:
		log.WithFields(logrus.Fields{
			"step": s,
		}).Error("Type not recognized, must be image or repository")
		return ErrInvalidInput
	}
	return nil
}

// process a sync step
func (s ConfigSync) processRef(ctx context.Context, src, tgt regclient.Ref, action string) error {
	mSrc, err := rc.ManifestHead(ctx, src)
	if err != nil {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"error":  err,
		}).Error("Failed to lookup source manifest")
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
	tgtExists := (err == nil)

	// if platform is defined and source is a list, resolve the source platform
	if mSrc.IsList() && s.Platform != "" {
		platDigest, err := getPlatformDigest(ctx, src, s.Platform, mSrc)
		if err != nil {
			return err
		}
		src.Digest = platDigest.String()
		if tgtExists && platDigest.String() == mTgt.GetDigest().String() {
			log.WithFields(logrus.Fields{
				"source":   src.CommonName(),
				"platform": s.Platform,
				"target":   tgt.CommonName(),
			}).Debug("Image matches for platform")
			return nil
		}
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
		// refresh current rate limit after acquiring semaphore
		mSrc, err = rc.ManifestHead(ctx, src)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": src.CommonName(),
				"error":  err,
			}).Error("Failed to lookup source manifest")
			return err
		}
		// delay if rate limit exceeded
		rlSrc := mSrc.GetRateLimit()
		for rlSrc.Remain < s.RateLimit.Min {
			sem.Release(1)
			log.WithFields(logrus.Fields{
				"source":        src.CommonName(),
				"source-remain": rlSrc.Remain,
				"source-limit":  rlSrc.Limit,
				"step-min":      s.RateLimit.Min,
				"sleep":         s.RateLimit.Retry,
			}).Info("Delaying for rate limit")
			select {
			case <-ctx.Done():
				return ErrCanceled
			case <-time.After(s.RateLimit.Retry):
			}
			sem.Acquire(ctx, 1)
			mSrc, err = rc.ManifestHead(ctx, src)
			if err != nil {
				sem.Release(1)
				log.WithFields(logrus.Fields{
					"source": src.CommonName(),
					"error":  err,
				}).Error("Failed to lookup source manifest")
				return err
			}
			rlSrc = mSrc.GetRateLimit()
		}
		log.WithFields(logrus.Fields{
			"source":        src.CommonName(),
			"source-remain": rlSrc.Remain,
			"step-min":      s.RateLimit.Min,
		}).Debug("Rate limit passed")
	}
	defer sem.Release(1)

	// verify context has not been canceled while waiting for semaphore
	select {
	case <-ctx.Done():
		return ErrCanceled
	default:
	}

	// run backup
	if tgtExists && s.Backup != "" {
		// expand template
		data := struct {
			Ref  regclient.Ref
			Step ConfigSync
		}{Ref: tgt, Step: s}
		backupStr, err := template.String(s.Backup, data)
		if err != nil {
			log.WithFields(logrus.Fields{
				"original":        tgt.CommonName(),
				"backup-template": s.Backup,
				"error":           err,
			}).Error("Failed to expand backup template")
			return err
		}
		backupStr = strings.TrimSpace(backupStr)
		backupRef := tgt
		if strings.ContainsAny(backupStr, ":/") {
			// if the : or / are in the string, parse it as a full reference
			backupRef, err = regclient.NewRef(backupStr)
			if err != nil {
				log.WithFields(logrus.Fields{
					"original": tgt.CommonName(),
					"template": s.Backup,
					"backup":   backupStr,
					"error":    err,
				}).Error("Failed to parse backup reference")
				return err
			}
		} else {
			// else parse backup string as just a tag
			backupRef.Tag = backupStr
		}
		// run copy from tgt ref to backup ref
		log.WithFields(logrus.Fields{
			"original": tgt.CommonName(),
			"backup":   backupRef.CommonName(),
		}).Info("Saving backup")
		err = rc.ImageCopy(ctx, tgt, backupRef)
		if err != nil {
			log.WithFields(logrus.Fields{
				"original": tgt.CommonName(),
				"template": s.Backup,
				"backup":   backupRef.CommonName(),
				"error":    err,
			}).Error("Failed to backup existing image")
			return err
		}
	}

	// Copy the image
	log.WithFields(logrus.Fields{
		"source": src.CommonName(),
		"target": tgt.CommonName(),
	}).Debug("Image sync running")
	err = rc.ImageCopy(ctx, src, tgt)
	if err != nil {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
			"error":  err,
		}).Error("Failed to copy image")
		return err
	}
	return nil
}

func (s ConfigSync) filterTags(in []string) ([]string, error) {
	var result []string
	// apply allow list
	if len(s.Tags.Allow) > 0 {
		result = make([]string, len(in))
		for _, filter := range s.Tags.Allow {
			exp, err := regexp.Compile("^" + filter + "$")
			if err != nil {
				return result, err
			}
			for i := range in {
				if result[i] == "" && exp.MatchString(in[i]) {
					result[i] = in[i]
				}
			}
		}
	} else {
		// by default, everything is allowed
		result = in
	}

	// apply deny list
	if len(s.Tags.Deny) > 0 {
		for _, filter := range s.Tags.Deny {
			exp, err := regexp.Compile("^" + filter + "$")
			if err != nil {
				return result, err
			}
			for i := range result {
				if result[i] != "" && exp.MatchString(result[i]) {
					result[i] = ""
				}
			}
		}
	}

	// compress result list, removing empty elements
	var compressed = make([]string, 0, len(in))
	for i := range result {
		if result[i] != "" {
			compressed = append(compressed, result[i])
		}
	}

	return compressed, nil
}

var manifestCache struct {
	mu        sync.Mutex
	manifests map[string]regclient.Manifest
}

func init() {
	manifestCache.manifests = map[string]regclient.Manifest{}
}

// getPlatformDigest resolves a manifest list to a specific platform's digest
// This uses the above cache to only call ManifestGet when a new manifest list digest is seen
func getPlatformDigest(ctx context.Context, ref regclient.Ref, platStr string, origMan regclient.Manifest) (digest.Digest, error) {
	plat, err := platforms.Parse(platStr)
	if err != nil {
		log.WithFields(logrus.Fields{
			"platform": platStr,
			"err":      err,
		}).Warn("Could not parse platform")
		return "", err
	}
	// cache manifestGet response
	manifestCache.mu.Lock()
	getMan, ok := manifestCache.manifests[origMan.GetDigest().String()]
	if !ok {
		getMan, err = rc.ManifestGet(ctx, ref)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": ref.CommonName(),
				"error":  err,
			}).Error("Failed to get source manifest")
			manifestCache.mu.Unlock()
			return "", err
		}
		manifestCache.manifests[origMan.GetDigest().String()] = getMan
	}
	manifestCache.mu.Unlock()
	descPlat, err := getMan.GetPlatformDesc(&plat)
	if err != nil {
		pl, _ := getMan.GetPlatformList()
		var ps []string
		for _, p := range pl {
			ps = append(ps, platforms.Format(*p))
		}
		log.WithFields(logrus.Fields{
			"platform":  platforms.Format(plat),
			"err":       err,
			"platforms": strings.Join(ps, ", "),
		}).Warn("Platform could not be found in source manifest list")
		return "", ErrNotFound
	}
	return descPlat.Digest, nil
}
