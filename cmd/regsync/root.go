package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

const (
	usageDesc = `Utility for mirroring docker repositories
More details at https://github.com/regclient/regclient`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regsync"
)

var rootOpts struct {
	confFile  string
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

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the config",
	Long:  `Show the config`,
	Args:  cobra.RangeArgs(0, 0),
	RunE:  runConfig,
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
	rootCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.InfoLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	rootCmd.MarkPersistentFlagFilename("config")
	serverCmd.MarkPersistentFlagRequired("config")
	checkCmd.MarkPersistentFlagRequired("config")
	onceCmd.MarkPersistentFlagRequired("config")
	configCmd.MarkPersistentFlagRequired("config")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(onceCmd)
	rootCmd.AddCommand(configCmd)
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

// runConfig processes the file in one pass, ignoring cron
func runConfig(cmd *cobra.Command, args []string) error {
	err := loadConf()
	if err != nil {
		return err
	}

	return ConfigWrite(conf, cmd.OutOrStdout())
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
	for _, s := range conf.Sync {
		s := s
		if conf.Defaults.Parallel > 0 {
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
		} else {
			err := s.process(ctx, "copy")
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
			// immediately copy any images that are missing from target
			if conf.Defaults.Parallel > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := s.process(ctx, "missing")
					if err != nil {
						if mainErr == nil {
							mainErr = err
						}
						return
					}
				}()
			} else {
				err := s.process(ctx, "missing")
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
	err := loadConf()
	if err != nil {
		return err
	}
	var mainErr error
	ctx := cmd.Context()
	for _, s := range conf.Sync {
		err := s.process(ctx, "check")
		if err != nil {
			if mainErr == nil {
				mainErr = err
			}
		}
	}
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
func (s ConfigSync) process(ctx context.Context, action string) error {
	var retErr error
	switch s.Type {
	case "registry":
		last := ""
		for {
			repoOpts := []scheme.RepoOpts{}
			if last != "" {
				repoOpts = append(repoOpts, scheme.WithRepoLast(last))
			}
			sRepos, err := rc.RepoList(ctx, s.Source, repoOpts...)
			if err != nil {
				log.WithFields(logrus.Fields{
					"source": s.Source,
					"error":  err,
				}).Error("Failed to list source repositories")
				return err
			}
			sRepoList, err := sRepos.GetRepos()
			if err != nil {
				log.WithFields(logrus.Fields{
					"source": s.Source,
					"error":  err,
				}).Error("Failed to list source repositories")
				return err
			}
			if len(sRepoList) == 0 || last == sRepoList[len(sRepoList)-1] {
				break
			}
			last = sRepoList[len(sRepoList)-1]
			for _, repo := range sRepoList {
				sRepoRef, err := ref.New(fmt.Sprintf("%s/%s", s.Source, repo))
				if err != nil {
					log.WithFields(logrus.Fields{
						"source": s.Source,
						"repo":   repo,
						"error":  err,
					}).Error("Failed to define source reference")
					return err
				}
				sTags, err := rc.TagList(ctx, sRepoRef)
				if err != nil {
					log.WithFields(logrus.Fields{
						"source": sRepoRef.CommonName(),
						"error":  err,
					}).Error("Failed getting source tags")
					retErr = err
					continue
				}
				sTagsList, err := sTags.GetTags()
				if err != nil {
					log.WithFields(logrus.Fields{
						"source": sRepoRef.CommonName(),
						"error":  err,
					}).Error("Failed getting source tags")
					retErr = err
					continue
				}
				sTagList, err := s.filterTags(sTagsList)
				if err != nil {
					log.WithFields(logrus.Fields{
						"source": sRepoRef.CommonName(),
						"allow":  s.Tags.Allow,
						"deny":   s.Tags.Deny,
						"error":  err,
					}).Error("Failed processing tag filters")
					retErr = err
					continue
				}
				if len(sTagList) == 0 {
					log.WithFields(logrus.Fields{
						"source":    sRepoRef.CommonName(),
						"allow":     s.Tags.Allow,
						"deny":      s.Tags.Deny,
						"available": sTagsList,
					}).Info("No matching tags found")
					retErr = err
					continue
				}
				tRepoRef, err := ref.New(fmt.Sprintf("%s/%s", s.Target, repo))
				if err != nil {
					log.WithFields(logrus.Fields{
						"target": s.Target,
						"repo":   repo,
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
						log.WithFields(logrus.Fields{
							"target": tRef.CommonName(),
							"source": sRef.CommonName(),
							"error":  err,
						}).Error("Failed to sync")
						retErr = err
					}
					err = rc.Close(ctx, tRef)
					if err != nil {
						log.WithFields(logrus.Fields{
							"ref":   tRef.CommonName(),
							"error": err,
						}).Error("Error closing ref")
					}
				}
			}
		}
	case "repository":
		sRepoRef, err := ref.New(s.Source)
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
		sTagsList, err := sTags.GetTags()
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": sRepoRef.CommonName(),
				"error":  err,
			}).Error("Failed getting source tags")
			return err
		}
		sTagList, err := s.filterTags(sTagsList)
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
		tRepoRef, err := ref.New(s.Target)
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
				log.WithFields(logrus.Fields{
					"target": tRef.CommonName(),
					"source": sRef.CommonName(),
					"error":  err,
				}).Error("Failed to sync")
				retErr = err
			}
			err = rc.Close(ctx, tRef)
			if err != nil {
				log.WithFields(logrus.Fields{
					"ref":   tRef.CommonName(),
					"error": err,
				}).Error("Error closing ref")
			}
		}

	case "image":
		sRef, err := ref.New(s.Source)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": s.Source,
				"error":  err,
			}).Error("Failed parsing source")
			return err
		}
		tRef, err := ref.New(s.Target)
		if err != nil {
			log.WithFields(logrus.Fields{
				"target": s.Target,
				"error":  err,
			}).Error("Failed parsing target")
			return err
		}
		err = s.processRef(ctx, sRef, tRef, action)
		if err != nil {
			log.WithFields(logrus.Fields{
				"target": tRef.CommonName(),
				"source": sRef.CommonName(),
				"error":  err,
			}).Error("Failed to sync")
			retErr = err
		}
		err = rc.Close(ctx, tRef)
		if err != nil {
			log.WithFields(logrus.Fields{
				"ref":   tRef.CommonName(),
				"error": err,
			}).Error("Error closing ref")
		}

	default:
		log.WithFields(logrus.Fields{
			"step": s,
			"type": s.Type,
		}).Error("Type not recognized, must be one of: registry, repository, or image")
		return ErrInvalidInput
	}
	return retErr
}

// process a sync step
func (s ConfigSync) processRef(ctx context.Context, src, tgt ref.Ref, action string) error {
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
	mTgt, err := rc.ManifestHead(ctx, tgt, regclient.WithManifestRequireDigest())
	tgtExists := (err == nil)
	tgtMatches := false
	if err == nil && manifest.GetDigest(mSrc).String() == manifest.GetDigest(mTgt).String() {
		tgtMatches = true
	}
	if tgtMatches && (s.ForceRecursive == nil || !*s.ForceRecursive) {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Debug("Image matches")
		return nil
	}
	if tgtExists && action == "missing" {
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
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Info("Image sync forced")
	} else {
		log.WithFields(logrus.Fields{
			"source": src.CommonName(),
			"target": tgt.CommonName(),
		}).Info("Image sync needed")
	}
	if action == "check" {
		return nil
	}

	// wait for parallel tasks
	sem.Acquire(ctx, 1)
	// delay for rate limit on source
	if s.RateLimit.Min > 0 && manifest.GetRateLimit(mSrc).Set {
		// refresh current rate limit after acquiring semaphore
		mSrc, err = rc.ManifestHead(ctx, src)
		if err != nil {
			log.WithFields(logrus.Fields{
				"source": src.CommonName(),
				"error":  err,
			}).Error("rate limit check failed")
			return err
		}
		// delay if rate limit exceeded
		rlSrc := manifest.GetRateLimit(mSrc)
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
				}).Error("rate limit check failed")
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
	defer sem.Release(1)

	// verify context has not been canceled while waiting for semaphore
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
					rOpts = append(rOpts, scheme.WithReferrerAT(filter.ArtifactType))
				}
				if filter.Annotations != nil {
					rOpts = append(rOpts, scheme.WithReferrerAnnotations(filter.Annotations))
				}
				opts = append(opts, regclient.ImageWithReferrers(rOpts...))
			}
		}
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
