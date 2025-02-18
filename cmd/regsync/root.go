package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/cobradoc"
	"github.com/regclient/regclient/internal/pqueue"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

const (
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regsync"
)

type actionType int

const (
	actionCheck actionType = iota
	actionCopy
	actionMissing
)

// throttle is used for limiting concurrent sync steps from running.
// This is separate from the concurrency limits in regclient itself.
type throttle struct{}

type rootCmd struct {
	confFile  string
	verbosity string
	logopts   []string
	log       *slog.Logger
	format    string // for Go template formatting of various commands
	missing   bool
	conf      *Config
	rc        *regclient.RegClient
	throttle  *pqueue.Queue[throttle]
}

func NewRootCmd() (*cobra.Command, *rootCmd) {
	var rootTopCmd = &cobra.Command{
		Use:   "regsync <cmd>",
		Short: "Utility for mirroring docker repositories",
		Long: `Utility for mirroring docker repositories
More details at <https://github.com/regclient/regclient>`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootOpts := rootCmd{
		log: slog.New(slog.NewTextHandler(rootTopCmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelInfo})),
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
	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", slog.LevelInfo.String(), "Log level (trace, debug, info, warn, error)")
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
		rootOpts.log = slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: lvl}))
	} else {
		rootOpts.log = slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: lvl}))
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

	return ConfigWrite(rootOpts.conf, cmd.OutOrStdout())
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
	for _, s := range rootOpts.conf.Sync {
		if rootOpts.conf.Defaults.Parallel > 0 {
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
	for _, s := range rootOpts.conf.Sync {
		sched := s.Schedule
		if sched == "" && s.Interval != 0 {
			sched = "@every " + s.Interval.String()
		}
		if sched != "" {
			rootOpts.log.Debug("Scheduled task",
				slog.String("source", s.Source),
				slog.String("target", s.Target),
				slog.String("type", s.Type),
				slog.String("sched", sched))
			_, errCron := c.AddFunc(sched, func() {
				rootOpts.log.Debug("Running task",
					slog.String("source", s.Source),
					slog.String("target", s.Target),
					slog.String("type", s.Type))
				wg.Add(1)
				defer wg.Done()
				err := rootOpts.process(ctx, s, actionCopy)
				if mainErr == nil {
					mainErr = err
				}
			})
			if errCron != nil {
				rootOpts.log.Error("Failed to schedule cron",
					slog.String("source", s.Source),
					slog.String("target", s.Target),
					slog.String("sched", sched),
					slog.String("err", errCron.Error()))
				if mainErr != nil {
					mainErr = errCron
				}
			}
			// immediately copy any images that are missing from target
			if rootOpts.conf.Defaults.Parallel > 0 {
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
			rootOpts.log.Error("No schedule or interval found, ignoring",
				slog.String("source", s.Source),
				slog.String("target", s.Target),
				slog.String("type", s.Type))
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
	rootOpts.log.Info("Stopping server")
	// clean shutdown
	c.Stop()
	rootOpts.log.Debug("Waiting on running tasks")
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
	for _, s := range rootOpts.conf.Sync {
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
	rootOpts.throttle = pqueue.New(pqueue.Opts[throttle]{Max: concurrent})
	// set the regclient, loading docker creds unless disabled, and inject logins from config file
	rcOpts := []regclient.Opt{
		regclient.WithSlog(rootOpts.log),
	}
	if rootOpts.conf.Defaults.BlobLimit != 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithBlobLimit(rootOpts.conf.Defaults.BlobLimit)))
	}
	if rootOpts.conf.Defaults.CacheCount > 0 && rootOpts.conf.Defaults.CacheTime > 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithCache(rootOpts.conf.Defaults.CacheTime, rootOpts.conf.Defaults.CacheCount)))
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
		rootOpts.log.Error("Type not recognized, must be one of: registry, repository, or image",
			slog.Any("step", s),
			slog.String("type", s.Type))
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
		sRepos, err := rootOpts.rc.RepoList(ctx, src, repoOpts...)
		if err != nil {
			rootOpts.log.Error("Failed to list source repositories",
				slog.String("source", src),
				slog.String("error", err.Error()))
			return err
		}
		sRepoList, err := sRepos.GetRepos()
		if err != nil {
			rootOpts.log.Error("Failed to list source repositories",
				slog.String("source", src),
				slog.String("error", err.Error()))
			return err
		}
		if len(sRepoList) == 0 || last == sRepoList[len(sRepoList)-1] {
			break
		}
		last = sRepoList[len(sRepoList)-1]
		// filter repos according to allow/deny rules
		sRepoList, err = filterList(s.Repos, sRepoList)
		if err != nil {
			rootOpts.log.Error("Failed processing repo filters",
				slog.String("source", src),
				slog.Any("allow", s.Repos.Allow),
				slog.Any("deny", s.Repos.Deny),
				slog.String("error", err.Error()))
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
		rootOpts.log.Error("Failed parsing source",
			slog.String("source", src),
			slog.String("error", err.Error()))
		return err
	}
	sTags, err := rootOpts.rc.TagList(ctx, sRepoRef)
	if err != nil {
		rootOpts.log.Error("Failed getting source tags",
			slog.String("source", sRepoRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	sTagsList, err := sTags.GetTags()
	if err != nil {
		rootOpts.log.Error("Failed getting source tags",
			slog.String("source", sRepoRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	sTagList, err := filterList(s.Tags, sTagsList)
	if err != nil {
		rootOpts.log.Error("Failed processing tag filters",
			slog.String("source", sRepoRef.CommonName()),
			slog.Any("allow", s.Tags.Allow),
			slog.Any("deny", s.Tags.Deny),
			slog.String("error", err.Error()))
		return err
	}
	if len(sTagList) == 0 {
		rootOpts.log.Warn("No matching tags found",
			slog.String("source", sRepoRef.CommonName()),
			slog.Any("allow", s.Tags.Allow),
			slog.Any("deny", s.Tags.Deny),
			slog.Any("available", sTagsList))
		return nil
	}
	// if only copying missing entries, delete tags that already exist on target
	if action == actionMissing {
		tRepoRef, err := ref.New(tgt)
		if err != nil {
			rootOpts.log.Error("Failed parsing target",
				slog.String("target", tgt),
				slog.String("error", err.Error()))
			return err
		}
		tTags, err := rootOpts.rc.TagList(ctx, tRepoRef)
		if err != nil {
			rootOpts.log.Debug("Failed getting target tags",
				slog.String("target", tRepoRef.CommonName()),
				slog.String("error", err.Error()))
		}
		tTagList := []string{}
		if err == nil {
			tTagList, err = tTags.GetTags()
			if err != nil {
				rootOpts.log.Debug("Failed getting target tags",
					slog.String("target", tRepoRef.CommonName()),
					slog.String("error", err.Error()))
			}
		}
		sI := len(sTagList) - 1
		tI := len(tTagList) - 1
		for sI >= 0 && tI >= 0 {
			switch strings.Compare(sTagList[sI], tTagList[tI]) {
			case 0:
				sTagList = slices.Delete(sTagList, sI, sI+1)
				sI--
				tI--
			case -1:
				tI--
			case 1:
				sI--
			default:
				rootOpts.log.Warn("strings.Compare unexpected result",
					slog.Int("result", strings.Compare(sTagList[sI], tTagList[tI])),
					slog.String("left", sTagList[sI]),
					slog.String("right", tTagList[tI]))
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
		rootOpts.log.Error("Failed parsing source",
			slog.String("source", src),
			slog.String("error", err.Error()))
		return err
	}
	tRef, err := ref.New(tgt)
	if err != nil {
		rootOpts.log.Error("Failed parsing target",
			slog.String("target", tgt),
			slog.String("error", err.Error()))
		return err
	}
	err = rootOpts.processRef(ctx, s, sRef, tRef, action)
	if err != nil {
		rootOpts.log.Error("Failed to sync",
			slog.String("target", tRef.CommonName()),
			slog.String("source", sRef.CommonName()),
			slog.String("error", err.Error()))
	}
	if err := rootOpts.rc.Close(ctx, tRef); err != nil {
		rootOpts.log.Error("Error closing ref",
			slog.String("ref", tRef.CommonName()),
			slog.String("error", err.Error()))
	}
	return err
}

// process a sync step
func (rootOpts *rootCmd) processRef(ctx context.Context, s ConfigSync, src, tgt ref.Ref, action actionType) error {
	mSrc, err := rootOpts.rc.ManifestHead(ctx, src, regclient.WithManifestRequireDigest())
	if err != nil && errors.Is(err, errs.ErrUnsupportedAPI) {
		mSrc, err = rootOpts.rc.ManifestGet(ctx, src)
	}
	if err != nil {
		rootOpts.log.Error("Failed to lookup source manifest",
			slog.String("source", src.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	fastCheck := (s.FastCheck != nil && *s.FastCheck)
	forceRecursive := (s.ForceRecursive != nil && *s.ForceRecursive)
	referrers := (s.Referrers != nil && *s.Referrers)
	digestTags := (s.DigestTags != nil && *s.DigestTags)
	mTgt, err := rootOpts.rc.ManifestHead(ctx, tgt, regclient.WithManifestRequireDigest())
	tgtExists := (err == nil)
	tgtMatches := false
	if err == nil && manifest.GetDigest(mSrc).String() == manifest.GetDigest(mTgt).String() {
		tgtMatches = true
	}
	if tgtMatches && (fastCheck || (!forceRecursive && !referrers && !digestTags)) {
		rootOpts.log.Debug("Image matches",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
		return nil
	}
	if tgtExists && action == actionMissing {
		rootOpts.log.Debug("target exists",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
		return nil
	}

	// skip when source manifest is an unsupported type
	smt := manifest.GetMediaType(mSrc)
	if !slices.Contains(s.MediaTypes, smt) {
		rootOpts.log.Info("Skipping unsupported media type",
			slog.String("ref", src.CommonName()),
			slog.String("mediaType", manifest.GetMediaType(mSrc)),
			slog.Any("allowed", s.MediaTypes))
		return nil
	}

	// if platform is defined and source is a list, resolve the source platform
	if mSrc.IsList() && s.Platform != "" {
		platDigest, err := rootOpts.getPlatformDigest(ctx, src, s.Platform, mSrc)
		if err != nil {
			return err
		}
		src.Digest = platDigest.String()
		if tgtExists && platDigest.String() == manifest.GetDigest(mTgt).String() {
			tgtMatches = true
		}
		if tgtMatches && (s.ForceRecursive == nil || !*s.ForceRecursive) {
			rootOpts.log.Debug("Image matches for platform",
				slog.String("source", src.CommonName()),
				slog.String("platform", s.Platform),
				slog.String("target", tgt.CommonName()))
			return nil
		}
	}
	if tgtMatches {
		rootOpts.log.Info("Image refreshing",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()),
			slog.Bool("forced", forceRecursive),
			slog.Bool("digestTags", digestTags),
			slog.Bool("referrers", referrers))
	} else {
		rootOpts.log.Info("Image sync needed",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
	}
	if action == actionCheck {
		return nil
	}

	// wait for parallel tasks
	throttleDone, err := rootOpts.throttle.Acquire(ctx, throttle{})
	if err != nil {
		return fmt.Errorf("failed to acquire throttle: %w", err)
	}
	// delay for rate limit on source
	if s.RateLimit.Min > 0 && manifest.GetRateLimit(mSrc).Set {
		// refresh current rate limit after acquiring throttle
		mSrc, err = rootOpts.rc.ManifestHead(ctx, src)
		if err != nil {
			rootOpts.log.Error("rate limit check failed",
				slog.String("source", src.CommonName()),
				slog.String("error", err.Error()))
			throttleDone()
			return err
		}
		// delay if rate limit exceeded
		rlSrc := manifest.GetRateLimit(mSrc)
		for rlSrc.Remain < s.RateLimit.Min {
			throttleDone()
			rootOpts.log.Info("Delaying for rate limit",
				slog.String("source", src.CommonName()),
				slog.Int("source-remain", rlSrc.Remain),
				slog.Int("source-limit", rlSrc.Limit),
				slog.Int("step-min", s.RateLimit.Min),
				slog.Duration("sleep", s.RateLimit.Retry))
			select {
			case <-ctx.Done():
				return ErrCanceled
			case <-time.After(s.RateLimit.Retry):
			}
			throttleDone, err = rootOpts.throttle.Acquire(ctx, throttle{})
			if err != nil {
				return fmt.Errorf("failed to reacquire throttle: %w", err)
			}
			mSrc, err = rootOpts.rc.ManifestHead(ctx, src)
			if err != nil {
				rootOpts.log.Error("rate limit check failed",
					slog.String("source", src.CommonName()),
					slog.String("error", err.Error()))
				throttleDone()
				return err
			}
			rlSrc = manifest.GetRateLimit(mSrc)
		}
		rootOpts.log.Debug("Rate limit passed",
			slog.String("source", src.CommonName()),
			slog.Int("source-remain", rlSrc.Remain),
			slog.Int("step-min", s.RateLimit.Min))
	}
	defer throttleDone()

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
			rootOpts.log.Error("Failed to expand backup template",
				slog.String("original", tgt.CommonName()),
				slog.String("backup-template", s.Backup),
				slog.String("error", err.Error()))
			return err
		}
		backupStr = strings.TrimSpace(backupStr)
		backupRef := tgt
		if strings.ContainsAny(backupStr, ":/") {
			// if the : or / are in the string, parse it as a full reference
			backupRef, err = ref.New(backupStr)
			if err != nil {
				rootOpts.log.Error("Failed to parse backup reference",
					slog.String("original", tgt.CommonName()),
					slog.String("template", s.Backup),
					slog.String("backup", backupStr),
					slog.String("error", err.Error()))
				return err
			}
		} else {
			// else parse backup string as just a tag
			backupRef = backupRef.SetTag(backupStr)
		}
		defer rootOpts.rc.Close(ctx, backupRef)
		// run copy from tgt ref to backup ref
		rootOpts.log.Info("Saving backup",
			slog.String("original", tgt.CommonName()),
			slog.String("backup", backupRef.CommonName()))
		err = rootOpts.rc.ImageCopy(ctx, tgt, backupRef)
		if err != nil {
			// Possible registry corruption with existing image, only warn and continue/overwrite
			rootOpts.log.Warn("Failed to backup existing image",
				slog.String("original", tgt.CommonName()),
				slog.String("template", s.Backup),
				slog.String("backup", backupRef.CommonName()),
				slog.String("error", err.Error()))
		}
	}

	opts := []regclient.ImageOpts{}
	if s.DigestTags != nil && *s.DigestTags {
		opts = append(opts, regclient.ImageWithDigestTags())
	}
	if s.Referrers != nil && *s.Referrers {
		if len(s.ReferrerFilters) == 0 {
			opts = append(opts, regclient.ImageWithReferrers())
		} else {
			for _, filter := range s.ReferrerFilters {
				rOpts := []scheme.ReferrerOpts{}
				if filter.ArtifactType != "" {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: filter.ArtifactType}))
				}
				if filter.Annotations != nil {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: filter.Annotations}))
				}
				opts = append(opts, regclient.ImageWithReferrers(rOpts...))
			}
		}
		if s.ReferrerSrc != "" {
			referrerSrc, err := ref.New(s.ReferrerSrc)
			if err != nil {
				rootOpts.log.Error("failed to parse referrer source reference",
					slog.String("referrerSource", s.ReferrerSrc),
					slog.String("error", err.Error()))
			}
			opts = append(opts, regclient.ImageWithReferrerSrc(referrerSrc))
		}
		if s.ReferrerTgt != "" {
			referrerTgt, err := ref.New(s.ReferrerTgt)
			if err != nil {
				rootOpts.log.Error("failed to parse referrer target reference",
					slog.String("referrerTarget", s.ReferrerTgt),
					slog.String("error", err.Error()))
			}
			opts = append(opts, regclient.ImageWithReferrerTgt(referrerTgt))
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
	rootOpts.log.Debug("Image sync running",
		slog.String("source", src.CommonName()),
		slog.String("target", tgt.CommonName()))
	err = rootOpts.rc.ImageCopy(ctx, src, tgt, opts...)
	if err != nil {
		rootOpts.log.Error("Failed to copy image",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()),
			slog.String("error", err.Error()))
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
func (rootOpts *rootCmd) getPlatformDigest(ctx context.Context, r ref.Ref, platStr string, origMan manifest.Manifest) (digest.Digest, error) {
	plat, err := platform.Parse(platStr)
	if err != nil {
		rootOpts.log.Warn("Could not parse platform",
			slog.String("platform", platStr),
			slog.String("err", err.Error()))
		return "", err
	}
	// cache manifestGet response
	manifestCache.mu.Lock()
	getMan, ok := manifestCache.manifests[manifest.GetDigest(origMan).String()]
	if !ok {
		getMan, err = rootOpts.rc.ManifestGet(ctx, r)
		if err != nil {
			rootOpts.log.Error("Failed to get source manifest",
				slog.String("source", r.CommonName()),
				slog.String("error", err.Error()))
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
		rootOpts.log.Warn("Platform could not be found in source manifest list",
			slog.String("platform", plat.String()),
			slog.String("err", err.Error()),
			slog.String("platforms", strings.Join(ps, ", ")))
		return "", ErrNotFound
	}
	return descPlat.Digest, nil
}
