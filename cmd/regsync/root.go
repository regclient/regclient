package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/throttle"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

const (
	usageDesc = `Utility for mirroring docker repositories
More details at https://github.com/regclient/regclient`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regsync"
)

type actionType int

const (
	actionCheck actionType = iota
	actionCopy
	actionMissing
)

type rootCmd struct {
	confFile  string
	verbosity string
	logopts   []string
	format    string // for Go template formatting of various commands
	missing   bool
}

// TODO: remove globals, configure tests with t.Parallel
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
		Use:           "regsync <cmd>",
		Short:         "Utility for mirroring docker repositories",
		Long:          usageDesc,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "run the regsync server",
		Long:  `Sync registries according to the configuration.`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  rootOpts.runServer,
	}
	var checkCmd = &cobra.Command{
		Use:   "check",
		Short: "processes each sync command once but skip actual copy",
		Long: `Processes each sync command in the configuration file in order.
Manifests are checked to see if a copy is needed, but only log, skip copying.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: rootOpts.runCheck,
	}
	var onceCmd = &cobra.Command{
		Use:   "once",
		Short: "processes each sync command once, ignoring cron schedule",
		Long: `Processes each sync command in the configuration file in order.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: rootOpts.runOnce,
	}

	var configCmd = &cobra.Command{
		Use:   "config",
		Short: "Show the config",
		Long:  `Show the config`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  rootOpts.runConfig,
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  `Show the version`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  rootOpts.runVersion,
	}

	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.confFile, "config", "c", "", "Config file")
	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.InfoLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	versionCmd.Flags().StringVar(&rootOpts.format, "format", "{{printPretty .}}", "Format output with go template syntax")
	onceCmd.Flags().BoolVar(&rootOpts.missing, "missing", false, "Only copy tags that are missing on target")

	_ = rootTopCmd.MarkPersistentFlagFilename("config")
	_ = serverCmd.MarkPersistentFlagRequired("config")
	_ = checkCmd.MarkPersistentFlagRequired("config")
	_ = onceCmd.MarkPersistentFlagRequired("config")
	_ = configCmd.MarkPersistentFlagRequired("config")

	rootTopCmd.AddCommand(serverCmd)
	rootTopCmd.AddCommand(checkCmd)
	rootTopCmd.AddCommand(onceCmd)
	rootTopCmd.AddCommand(configCmd)
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

// runConfig processes the file in one pass, ignoring cron
func (rootOpts *rootCmd) runConfig(cmd *cobra.Command, args []string) error {
	err := rootOpts.loadConf()
	if err != nil {
		return err
	}

	return ConfigWrite(conf, cmd.OutOrStdout())
}

// runOnce processes the file in one pass, ignoring cron
func (rootOpts *rootCmd) runOnce(cmd *cobra.Command, args []string) error {
	err := rootOpts.loadConf()
	if err != nil {
		return err
	}
	action := actionCopy
	if rootOpts.missing {
		action = actionMissing
	}
	ctx := cmd.Context()
	var wg sync.WaitGroup
	var mainErr error
	for _, s := range conf.Sync {
		s := s
		if conf.Defaults.Parallel > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := rootOpts.process(ctx, s, action)
				if err != nil {
					if mainErr == nil {
						mainErr = err
					}
					return
				}
			}()
		} else {
			err := rootOpts.process(ctx, s, action)
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
	// TODO: switch to joining array of errors once 1.20 is the minimum version
	var mainErr error
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	for _, s := range conf.Sync {
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
			_, errCron := c.AddFunc(sched, func() {
				log.WithFields(logrus.Fields{
					"source": s.Source,
					"target": s.Target,
					"type":   s.Type,
				}).Debug("Running task")
				wg.Add(1)
				defer wg.Done()
				err := rootOpts.process(ctx, s, actionCopy)
				if mainErr == nil {
					mainErr = err
				}
			})
			if errCron != nil {
				log.WithFields(logrus.Fields{
					"source": s.Source,
					"target": s.Target,
					"sched":  sched,
					"err":    errCron,
				}).Error("Failed to schedule cron")
				if mainErr != nil {
					mainErr = errCron
				}
			}
			// immediately copy any images that are missing from target
			if conf.Defaults.Parallel > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := rootOpts.process(ctx, s, actionMissing)
					if err != nil {
						if mainErr == nil {
							mainErr = err
						}
						return
					}
				}()
			} else {
				err := rootOpts.process(ctx, s, actionMissing)
				if err != nil {
					if mainErr == nil {
						mainErr = err
					}
				}
			}
		} else {
			log.WithFields(logrus.Fields{
				"source": s.Source,
				"target": s.Target,
				"type":   s.Type,
			}).Error("No schedule or interval found, ignoring")
		}
	}
	// wait for any initial copies to finish before scheduling
	wg.Wait()
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

// run check is used for a dry-run
func (rootOpts *rootCmd) runCheck(cmd *cobra.Command, args []string) error {
	err := rootOpts.loadConf()
	if err != nil {
		return err
	}
	var mainErr error
	ctx := cmd.Context()
	for _, s := range conf.Sync {
		err := rootOpts.process(ctx, s, actionCheck)
		if err != nil {
			if mainErr == nil {
				mainErr = err
			}
		}
	}
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
	if conf.Defaults.CacheCount > 0 && conf.Defaults.CacheTime > 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithCache(conf.Defaults.CacheTime, conf.Defaults.CacheCount)))
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
func (rootOpts *rootCmd) process(ctx context.Context, s ConfigSync, action actionType) error {
	switch s.Type {
	case "registry":
		if err := rootOpts.processRegistry(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	case "repository":
		if err := rootOpts.processRepo(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	case "image":
		if err := rootOpts.processImage(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	default:
		log.WithFields(logrus.Fields{
			"step": s,
			"type": s.Type,
		}).Error("Type not recognized, must be one of: registry, repository, or image")
		return ErrInvalidInput
	}
	return nil
}

func (rootOpts *rootCmd) processRegistry(ctx context.Context, s ConfigSync, src, tgt string, action actionType) error {
	last := ""
	var retErr error
	for {
		repoOpts := []scheme.RepoOpts{}
		if last != "" {
			repoOpts = append(repoOpts, scheme.WithRepoLast(last))
		}
		sRepos, err := rc.RepoList(ctx, src, repoOpts...)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": src,
				"error":  err,
			}).Error("Failed to list source repositories")
			return err
		}
		sRepoList, err := sRepos.GetRepos()
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": src,
				"error":  err,
			}).Error("Failed to list source repositories")
			return err
		}
		if len(sRepoList) == 0 || last == sRepoList[len(sRepoList)-1] {
			break
		}
		last = sRepoList[len(sRepoList)-1]
		// filter repos according to allow/deny rules
		sRepoList, err = filterList(s.Repos, sRepoList)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": src,
				"allow":  s.Repos.Allow,
				"deny":   s.Repos.Deny,
				"error":  err,
			}).Error("Failed processing repo filters")
			return err
		}
		for _, repo := range sRepoList {
			if err := rootOpts.processRepo(ctx, s, fmt.Sprintf("%s/%s", src, repo), fmt.Sprintf("%s/%s", tgt, repo), action); err != nil {
				retErr = err
			}
		}
	}
	return retErr
}

func (rootOpts *rootCmd) processRepo(ctx context.Context, s ConfigSync, src, tgt string, action actionType) error {
	sRepoRef, err := ref.New(src)
	if err != nil {
		log.WithFields(logrus.Fields{
			"source": src,
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
	sTagsList, err := sTags.GetTags()
	if err != nil {
		log.WithFields(logrus.Fields{
			"source": sRepoRef.CommonName(),
			"error":  err,
		}).Error("Failed getting source tags")
		return err
	}
	sTagList, err := filterList(s.Tags, sTagsList)
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
			"available": sTagsList,
		}).Warn("No matching tags found")
		return nil
	}
	// if only copying missing entries, delete tags that already exist on target
	if action == actionMissing {
		tRepoRef, err := ref.New(tgt)
		if err != nil {
			log.WithFields(logrus.Fields{
				"target": tgt,
				"error":  err,
			}).Error("Failed parsing target")
			return err
		}
		tTags, err := rc.TagList(ctx, tRepoRef)
		if err != nil {
			log.WithFields(logrus.Fields{
				"target": tRepoRef.CommonName(),
				"error":  err,
			}).Debug("Failed getting target tags")
		}
		tTagList := []string{}
		if err == nil {
			tTagList, err = tTags.GetTags()
			if err != nil {
				log.WithFields(logrus.Fields{
					"target": tRepoRef.CommonName(),
					"error":  err,
				}).Debug("Failed getting target tags")
			}
		}
		sI := len(sTagList) - 1
		tI := len(tTagList) - 1
		for sI >= 0 && tI >= 0 {
			switch strings.Compare(sTagList[sI], tTagList[tI]) {
			case 0:
				sTagList = append(sTagList[:sI], sTagList[sI+1:]...)
				sI--
				tI--
			case -1:
				tI--
			case 1:
				sI--
			default:
				log.WithFields(logrus.Fields{
					"result": strings.Compare(sTagList[sI], tTagList[tI]),
					"left":   sTagList[sI],
					"right":  tTagList[tI],
				}).Warn("strings.Compare unexpected result")
				sI--
				tI--
			}
		}
	}
	var retErr error
	for _, tag := range sTagList {
		if err := rootOpts.processImage(ctx, s, fmt.Sprintf("%s:%s", src, tag), fmt.Sprintf("%s:%s", tgt, tag), action); err != nil {
			retErr = err
		}
	}
	return retErr
}

func (rootOpts *rootCmd) processImage(ctx context.Context, s ConfigSync, src, tgt string, action actionType) error {
	sRef, err := ref.New(src)
	if err != nil {
		log.WithFields(logrus.Fields{
			"source": src,
			"error":  err,
		}).Error("Failed parsing source")
		return err
	}
	tRef, err := ref.New(tgt)
	if err != nil {
		log.WithFields(logrus.Fields{
			"target": tgt,
			"error":  err,
		}).Error("Failed parsing target")
		return err
	}
	err = rootOpts.processRef(ctx, s, sRef, tRef, action)
	if err != nil {
		log.WithFields(logrus.Fields{
			"target": tRef.CommonName(),
			"source": sRef.CommonName(),
			"error":  err,
		}).Error("Failed to sync")
	}
	if err := rc.Close(ctx, tRef); err != nil {
		log.WithFields(logrus.Fields{
			"ref":   tRef.CommonName(),
			"error": err,
		}).Error("Error closing ref")
	}
	return err
}

// process a sync step
func (rootOpts *rootCmd) processRef(ctx context.Context, s ConfigSync, src, tgt ref.Ref, action actionType) error {
	mSrc, err := rc.ManifestHead(ctx, src, regclient.WithManifestRequireDigest())
	if err != nil && errors.Is(err, types.ErrUnsupportedAPI) {
		mSrc, err = rc.ManifestGet(ctx, src)
	}
	if err != nil {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"error":  err,
		}).Error("Failed to lookup source manifest")
		return err
	}
	fastCheck := (s.FastCheck != nil && *s.FastCheck)
	forceRecursive := (s.ForceRecursive != nil && *s.ForceRecursive)
	referrers := (s.Referrers != nil && *s.Referrers)
	digestTags := (s.DigestTags != nil && *s.DigestTags)
	mTgt, err := rc.ManifestHead(ctx, tgt, regclient.WithManifestRequireDigest())
	tgtExists := (err == nil)
	tgtMatches := false
	if err == nil && manifest.GetDigest(mSrc).String() == manifest.GetDigest(mTgt).String() {
		tgtMatches = true
	}
	if tgtMatches && (fastCheck || (!forceRecursive && !referrers && !digestTags)) {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Debug("Image matches")
		return nil
	}
	if tgtExists && action == actionMissing {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Debug("target exists")
		return nil
	}

	// skip when source manifest is an unsupported type
	smt := manifest.GetMediaType(mSrc)
	found := false
	for _, mt := range s.MediaTypes {
		if mt == smt {
			found = true
			break
		}
	}
	if !found {
		log.WithFields(logrus.Fields{
			"ref":       src.CommonName(),
			"mediaType": manifest.GetMediaType(mSrc),
			"allowed":   s.MediaTypes,
		}).Info("Skipping unsupported media type")
		return nil
	}

	// if platform is defined and source is a list, resolve the source platform
	if mSrc.IsList() && s.Platform != "" {
		platDigest, err := getPlatformDigest(ctx, src, s.Platform, mSrc)
		if err != nil {
			return err
		}
		src.Digest = platDigest.String()
		if tgtExists && platDigest.String() == manifest.GetDigest(mTgt).String() {
			tgtMatches = true
		}
		if tgtMatches && (s.ForceRecursive == nil || !*s.ForceRecursive) {
			log.WithFields(logrus.Fields{
				"source":   src.CommonName(),
				"platform": s.Platform,
				"target":   tgt.CommonName(),
			}).Debug("Image matches for platform")
			return nil
		}
	}
	if tgtMatches {
		log.WithFields(logrus.Fields{
			"source":     src.CommonName(),
			"target":     tgt.CommonName(),
			"forced":     forceRecursive,
			"digestTags": digestTags,
			"referrers":  referrers,
		}).Info("Image refreshing")
	} else {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Info("Image sync needed")
	}
	if action == actionCheck {
		return nil
	}

	// wait for parallel tasks
	err = throttleC.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire throttle: %w", err)
	}
	// delay for rate limit on source
	if s.RateLimit.Min > 0 && manifest.GetRateLimit(mSrc).Set {
		// refresh current rate limit after acquiring throttle
		mSrc, err = rc.ManifestHead(ctx, src)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": src.CommonName(),
				"error":  err,
			}).Error("rate limit check failed")
			_ = throttleC.Release(ctx)
			return err
		}
		// delay if rate limit exceeded
		rlSrc := manifest.GetRateLimit(mSrc)
		for rlSrc.Remain < s.RateLimit.Min {
			err = throttleC.Release(ctx)
			if err != nil {
				return fmt.Errorf("failed to release throttle: %w", err)
			}
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
			err = throttleC.Acquire(ctx)
			if err != nil {
				return fmt.Errorf("failed to reacquire throttle: %w", err)
			}
			mSrc, err = rc.ManifestHead(ctx, src)
			if err != nil {
				log.WithFields(logrus.Fields{
					"source": src.CommonName(),
					"error":  err,
				}).Error("rate limit check failed")
				_ = throttleC.Release(ctx)
				return err
			}
			rlSrc = manifest.GetRateLimit(mSrc)
		}
		log.WithFields(logrus.Fields{
			"source":        src.CommonName(),
			"source-remain": rlSrc.Remain,
			"step-min":      s.RateLimit.Min,
		}).Debug("Rate limit passed")
	}
	defer throttleC.Release(ctx)

	// verify context has not been canceled while waiting for throttle
	select {
	case <-ctx.Done():
		return ErrCanceled
	default:
	}

	// run backup
	if tgtExists && !tgtMatches && s.Backup != "" {
		// expand template
		data := struct {
			Ref  ref.Ref
			Step ConfigSync
			Sync ConfigSync
		}{Ref: tgt, Step: s, Sync: s}
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
			backupRef, err = ref.New(backupStr)
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
		defer rc.Close(ctx, backupRef)
		// run copy from tgt ref to backup ref
		log.WithFields(logrus.Fields{
			"original": tgt.CommonName(),
			"backup":   backupRef.CommonName(),
		}).Info("Saving backup")
		err = rc.ImageCopy(ctx, tgt, backupRef)
		if err != nil {
			// Possible registry corruption with existing image, only warn and continue/overwrite
			log.WithFields(logrus.Fields{
				"original": tgt.CommonName(),
				"template": s.Backup,
				"backup":   backupRef.CommonName(),
				"error":    err,
			}).Warn("Failed to backup existing image")
		}
	}

	opts := []regclient.ImageOpts{}
	if s.DigestTags != nil && *s.DigestTags {
		opts = append(opts, regclient.ImageWithDigestTags())
	}
	if s.Referrers != nil && *s.Referrers {
		if s.ReferrerFilters == nil || len(s.ReferrerFilters) == 0 {
			opts = append(opts, regclient.ImageWithReferrers())
		} else {
			for _, filter := range s.ReferrerFilters {
				rOpts := []scheme.ReferrerOpts{}
				if filter.ArtifactType != "" {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(types.MatchOpt{ArtifactType: filter.ArtifactType}))
				}
				if filter.Annotations != nil {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(types.MatchOpt{Annotations: filter.Annotations}))
				}
				opts = append(opts, regclient.ImageWithReferrers(rOpts...))
			}
		}
	}
	if s.FastCheck != nil && *s.FastCheck {
		opts = append(opts, regclient.ImageWithFastCheck())
	}
	if s.ForceRecursive != nil && *s.ForceRecursive {
		opts = append(opts, regclient.ImageWithForceRecursive())
	}
	if s.IncludeExternal != nil && *s.IncludeExternal {
		opts = append(opts, regclient.ImageWithIncludeExternal())
	}
	if len(s.Platforms) > 0 {
		opts = append(opts, regclient.ImageWithPlatforms(s.Platforms))
	}

	// Copy the image
	log.WithFields(logrus.Fields{
		"source": src.CommonName(),
		"target": tgt.CommonName(),
	}).Debug("Image sync running")
	err = rc.ImageCopy(ctx, src, tgt, opts...)
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

func filterList(ad AllowDeny, in []string) ([]string, error) {
	var result []string
	// apply allow list
	if len(ad.Allow) > 0 {
		result = make([]string, len(in))
		for _, filter := range ad.Allow {
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
	if len(ad.Deny) > 0 {
		for _, filter := range ad.Deny {
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
	manifests map[string]manifest.Manifest
}

func init() {
	manifestCache.manifests = map[string]manifest.Manifest{}
}

// getPlatformDigest resolves a manifest list to a specific platform's digest
// This uses the above cache to only call ManifestGet when a new manifest list digest is seen
func getPlatformDigest(ctx context.Context, r ref.Ref, platStr string, origMan manifest.Manifest) (digest.Digest, error) {
	plat, err := platform.Parse(platStr)
	if err != nil {
		log.WithFields(logrus.Fields{
			"platform": platStr,
			"err":      err,
		}).Warn("Could not parse platform")
		return "", err
	}
	// cache manifestGet response
	manifestCache.mu.Lock()
	getMan, ok := manifestCache.manifests[manifest.GetDigest(origMan).String()]
	if !ok {
		getMan, err = rc.ManifestGet(ctx, r)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": r.CommonName(),
				"error":  err,
			}).Error("Failed to get source manifest")
			manifestCache.mu.Unlock()
			return "", err
		}
		manifestCache.manifests[manifest.GetDigest(origMan).String()] = getMan
	}
	manifestCache.mu.Unlock()
	descPlat, err := manifest.GetPlatformDesc(getMan, &plat)
	if err != nil {
		pl, _ := manifest.GetPlatformList(getMan)
		var ps []string
		for _, p := range pl {
			ps = append(ps, p.String())
		}
		log.WithFields(logrus.Fields{
			"platform":  plat,
			"err":       err,
			"platforms": strings.Join(ps, ", "),
		}).Warn("Platform could not be found in source manifest list")
		return "", ErrNotFound
	}
	return descPlat.Digest, nil
}
