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

type rootOpts struct {
	confFile   string
	verbosity  string
	logopts    []string
	log        *slog.Logger
	format     string // for Go template formatting of various commands
	abortOnErr bool
	missing    bool
	conf       *Config
	rc         *regclient.RegClient
	throttle   *pqueue.Queue[throttle]
}

func NewRootCmd() (*cobra.Command, *rootOpts) {
	opts := rootOpts{}
	var cmd = &cobra.Command{
		Use:   "regsync <cmd>",
		Short: "Utility for mirroring docker repositories",
		Long: `Utility for mirroring docker repositories
More details at <https://github.com/regclient/regclient>`,
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: opts.rootPreRun,
	}
	cmd.PersistentFlags().StringVarP(&opts.verbosity, "verbosity", "v", slog.LevelInfo.String(), "Log level (trace, debug, info, warn, error)")
	cmd.PersistentFlags().StringArrayVar(&opts.logopts, "logopt", []string{}, "Log options")

	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "run the regsync server",
		Long:  `Sync registries according to the configuration.`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  opts.runServer,
	}
	var checkCmd = &cobra.Command{
		Use:   "check",
		Short: "processes each sync command once but skip actual copy",
		Long: `Processes each sync command in the configuration file in order.
Manifests are checked to see if a copy is needed, but only log, skip copying.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: opts.runCheck,
	}
	var onceCmd = &cobra.Command{
		Use:   "once",
		Short: "processes each sync command once, ignoring cron schedule",
		Long: `Processes each sync command in the configuration file in order.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: opts.runOnce,
	}
	onceCmd.Flags().BoolVar(&opts.missing, "missing", false, "Only copy tags that are missing on target")
	var configCmd = &cobra.Command{
		Use:   "config",
		Short: "Show the config",
		Long:  `Show the config`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  opts.runConfig,
	}
	for _, curCmd := range []*cobra.Command{serverCmd, checkCmd, onceCmd, configCmd} {
		curCmd.Flags().StringVarP(&opts.confFile, "config", "c", "", "Config file")
		_ = curCmd.MarkFlagFilename("config")
		_ = curCmd.MarkFlagRequired("config")
	}
	for _, curCmd := range []*cobra.Command{serverCmd, checkCmd, onceCmd} {
		curCmd.Flags().BoolVar(&opts.abortOnErr, "abort-on-error", false, "Immediately abort on any errors")
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  `Show the version`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  opts.runVersion,
	}
	versionCmd.Flags().StringVar(&opts.format, "format", "{{printPretty .}}", "Format output with go template syntax")
	_ = versionCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	})

	opts.log = slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	cmd.AddCommand(
		serverCmd,
		checkCmd,
		onceCmd,
		configCmd,
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
		opts.log = slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: lvl}))
	} else {
		opts.log = slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: lvl}))
	}
	return nil
}

func (opts *rootOpts) runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()
	return template.Writer(os.Stdout, opts.format, info)
}

// runConfig processes the file in one pass, ignoring cron
func (opts *rootOpts) runConfig(cmd *cobra.Command, args []string) error {
	err := opts.loadConf()
	if err != nil {
		return err
	}

	return ConfigWrite(opts.conf, cmd.OutOrStdout())
}

// runOnce processes the file in one pass, ignoring cron
func (opts *rootOpts) runOnce(cmd *cobra.Command, args []string) error {
	err := opts.loadConf()
	if err != nil {
		return err
	}
	action := actionCopy
	if opts.missing {
		action = actionMissing
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := []error{}
	for _, s := range opts.conf.Sync {
		if opts.conf.Defaults.Parallel > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := opts.process(ctx, s, action)
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCanceled) {
					if opts.abortOnErr {
						cancel()
					}
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}()
		} else {
			err := opts.process(ctx, s, action)
			if err != nil {
				errs = append(errs, err)
				if opts.abortOnErr {
					break
				}
			}
		}
	}
	wg.Wait()
	return errors.Join(errs...)
}

// runServer stays running with cron scheduled tasks
func (opts *rootOpts) runServer(cmd *cobra.Command, args []string) error {
	err := opts.loadConf()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := []error{}
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	for _, s := range opts.conf.Sync {
		sched := s.Schedule
		if sched == "" && s.Interval != 0 {
			sched = "@every " + s.Interval.String()
		}
		if sched != "" {
			opts.log.Debug("Scheduled task",
				slog.String("source", s.Source),
				slog.String("target", s.Target),
				slog.String("type", s.Type),
				slog.String("sched", sched))
			_, err := c.AddFunc(sched, func() {
				opts.log.Debug("Running task",
					slog.String("source", s.Source),
					slog.String("target", s.Target),
					slog.String("type", s.Type))
				wg.Add(1)
				defer wg.Done()
				err := opts.process(ctx, s, actionCopy)
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCanceled) {
					if opts.abortOnErr {
						cancel()
					}
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			})
			if err != nil {
				opts.log.Error("Failed to schedule cron",
					slog.String("source", s.Source),
					slog.String("target", s.Target),
					slog.String("sched", sched),
					slog.String("err", err.Error()))
				errs = append(errs, err)
				if opts.abortOnErr {
					break
				}
			}
			// immediately copy any images that are missing from target
			if opts.conf.Defaults.Parallel > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := opts.process(ctx, s, actionMissing)
					if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCanceled) {
						if opts.abortOnErr {
							cancel()
						}
						mu.Lock()
						errs = append(errs, err)
						mu.Unlock()
					}
				}()
			} else {
				err := opts.process(ctx, s, actionMissing)
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCanceled) {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					if opts.abortOnErr {
						break
					}
				}
			}
		} else {
			opts.log.Error("No schedule or interval found, ignoring",
				slog.String("source", s.Source),
				slog.String("target", s.Target),
				slog.String("type", s.Type))
		}
	}
	// wait for any initial copies to finish
	wg.Wait()
	if ctx.Err() != nil {
		return errors.Join(errs...)
	}
	// start the server and wait until interrupted
	c.Start()
	done := ctx.Done()
	if done != nil {
		<-done
	}
	// perform a clean shutdown
	opts.log.Info("Stopping server")
	c.Stop()
	opts.log.Debug("Waiting on running tasks")
	wg.Wait()
	return errors.Join(errs...)
}

// run check is used for a dry-run
func (opts *rootOpts) runCheck(cmd *cobra.Command, args []string) error {
	err := opts.loadConf()
	if err != nil {
		return err
	}
	errs := []error{}
	ctx := cmd.Context()
	for _, s := range opts.conf.Sync {
		err := opts.process(ctx, s, actionCheck)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCanceled) {
			errs = append(errs, err)
			if opts.abortOnErr {
				break
			}
		}
	}
	return errors.Join(errs...)
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
	opts.throttle = pqueue.New(pqueue.Opts[throttle]{Max: concurrent})
	// set the regclient, loading docker creds unless disabled, and inject logins from config file
	rcOpts := []regclient.Opt{
		regclient.WithSlog(opts.log),
	}
	if opts.conf.Defaults.BlobLimit != 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithBlobLimit(opts.conf.Defaults.BlobLimit)))
	}
	if opts.conf.Defaults.CacheCount > 0 && opts.conf.Defaults.CacheTime > 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithCache(opts.conf.Defaults.CacheTime, opts.conf.Defaults.CacheCount)))
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
func (opts *rootOpts) process(ctx context.Context, s ConfigSync, action actionType) error {
	switch s.Type {
	case "registry":
		if err := opts.processRegistry(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	case "repository":
		if err := opts.processRepo(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	case "image":
		if err := opts.processImage(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	default:
		opts.log.Error("Type not recognized, must be one of: registry, repository, or image",
			slog.Any("step", s),
			slog.String("type", s.Type))
		return ErrInvalidInput
	}
	return nil
}

func (opts *rootOpts) processRegistry(ctx context.Context, s ConfigSync, src, tgt string, action actionType) error {
	last := ""
	errs := []error{}
	// loop through pages of the _catalog response
	for {
		repoOpts := []scheme.RepoOpts{}
		if last != "" {
			repoOpts = append(repoOpts, scheme.WithRepoLast(last))
		}
		sRepos, err := opts.rc.RepoList(ctx, src, repoOpts...)
		if err != nil {
			opts.log.Error("Failed to list source repositories",
				slog.String("source", src),
				slog.String("error", err.Error()))
			return err
		}
		sRepoList, err := sRepos.GetRepos()
		if err != nil {
			opts.log.Error("Failed to list source repositories",
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
			opts.log.Error("Failed processing repo filters",
				slog.String("source", src),
				slog.Any("allow", s.Repos.Allow),
				slog.Any("deny", s.Repos.Deny),
				slog.String("error", err.Error()))
			return err
		}
		for _, repo := range sRepoList {
			if err := opts.processRepo(ctx, s, fmt.Sprintf("%s/%s", src, repo), fmt.Sprintf("%s/%s", tgt, repo), action); err != nil {
				errs = append(errs, err)
				if opts.abortOnErr {
					break
				}
			}
		}
		if opts.abortOnErr && len(errs) > 0 {
			break
		}
	}
	return errors.Join(errs...)
}

func (opts *rootOpts) processRepo(ctx context.Context, s ConfigSync, src, tgt string, action actionType) error {
	sRepoRef, err := ref.New(src)
	if err != nil {
		opts.log.Error("Failed parsing source",
			slog.String("source", src),
			slog.String("error", err.Error()))
		return err
	}
	sTags, err := opts.rc.TagList(ctx, sRepoRef)
	if err != nil {
		opts.log.Error("Failed getting source tags",
			slog.String("source", sRepoRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	sTagsList, err := sTags.GetTags()
	if err != nil {
		opts.log.Error("Failed getting source tags",
			slog.String("source", sRepoRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	sTagList, err := filterList(s.Tags, sTagsList)
	if err != nil {
		opts.log.Error("Failed processing tag filters",
			slog.String("source", sRepoRef.CommonName()),
			slog.Any("allow", s.Tags.Allow),
			slog.Any("deny", s.Tags.Deny),
			slog.String("error", err.Error()))
		return err
	}
	if len(sTagList) == 0 {
		opts.log.Warn("No matching tags found",
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
			opts.log.Error("Failed parsing target",
				slog.String("target", tgt),
				slog.String("error", err.Error()))
			return err
		}
		tTags, err := opts.rc.TagList(ctx, tRepoRef)
		if err != nil {
			opts.log.Debug("Failed getting target tags",
				slog.String("target", tRepoRef.CommonName()),
				slog.String("error", err.Error()))
		}
		tTagList := []string{}
		if err == nil {
			tTagList, err = tTags.GetTags()
			if err != nil {
				opts.log.Debug("Failed getting target tags",
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
				opts.log.Warn("strings.Compare unexpected result",
					slog.Int("result", strings.Compare(sTagList[sI], tTagList[tI])),
					slog.String("left", sTagList[sI]),
					slog.String("right", tTagList[tI]))
				sI--
				tI--
			}
		}
	}
	errs := []error{}
	for _, tag := range sTagList {
		if err := opts.processImage(ctx, s, fmt.Sprintf("%s:%s", src, tag), fmt.Sprintf("%s:%s", tgt, tag), action); err != nil {
			errs = append(errs, err)
			if opts.abortOnErr {
				break
			}
		}
	}
	return errors.Join(errs...)
}

func (opts *rootOpts) processImage(ctx context.Context, s ConfigSync, src, tgt string, action actionType) error {
	sRef, err := ref.New(src)
	if err != nil {
		opts.log.Error("Failed parsing source",
			slog.String("source", src),
			slog.String("error", err.Error()))
		return err
	}
	tRef, err := ref.New(tgt)
	if err != nil {
		opts.log.Error("Failed parsing target",
			slog.String("target", tgt),
			slog.String("error", err.Error()))
		return err
	}
	err = opts.processRef(ctx, s, sRef, tRef, action)
	if err != nil {
		opts.log.Error("Failed to sync",
			slog.String("target", tRef.CommonName()),
			slog.String("source", sRef.CommonName()),
			slog.String("error", err.Error()))
	}
	if err := opts.rc.Close(ctx, tRef); err != nil {
		opts.log.Error("Error closing ref",
			slog.String("ref", tRef.CommonName()),
			slog.String("error", err.Error()))
	}
	return err
}

// process a sync step
func (opts *rootOpts) processRef(ctx context.Context, s ConfigSync, src, tgt ref.Ref, action actionType) error {
	mSrc, err := opts.rc.ManifestHead(ctx, src, regclient.WithManifestRequireDigest())
	if err != nil && errors.Is(err, errs.ErrUnsupportedAPI) {
		mSrc, err = opts.rc.ManifestGet(ctx, src)
	}
	if err != nil {
		opts.log.Error("Failed to lookup source manifest",
			slog.String("source", src.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	fastCheck := (s.FastCheck != nil && *s.FastCheck)
	forceRecursive := (s.ForceRecursive != nil && *s.ForceRecursive)
	referrers := (s.Referrers != nil && *s.Referrers)
	digestTags := (s.DigestTags != nil && *s.DigestTags)
	mTgt, err := opts.rc.ManifestHead(ctx, tgt, regclient.WithManifestRequireDigest())
	tgtExists := (err == nil)
	tgtMatches := false
	if err == nil && manifest.GetDigest(mSrc).String() == manifest.GetDigest(mTgt).String() {
		tgtMatches = true
	}
	if tgtMatches && (fastCheck || (!forceRecursive && !referrers && !digestTags)) {
		opts.log.Debug("Image matches",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
		return nil
	}
	if tgtExists && action == actionMissing {
		opts.log.Debug("target exists",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
		return nil
	}

	// skip when source manifest is an unsupported type
	smt := manifest.GetMediaType(mSrc)
	if !slices.Contains(s.MediaTypes, smt) {
		opts.log.Info("Skipping unsupported media type",
			slog.String("ref", src.CommonName()),
			slog.String("mediaType", manifest.GetMediaType(mSrc)),
			slog.Any("allowed", s.MediaTypes))
		return nil
	}

	// if platform is defined and source is a list, resolve the source platform
	if mSrc.IsList() && s.Platform != "" {
		platDigest, err := opts.getPlatformDigest(ctx, src, s.Platform, mSrc)
		if err != nil {
			return err
		}
		src.Digest = platDigest.String()
		if tgtExists && platDigest.String() == manifest.GetDigest(mTgt).String() {
			tgtMatches = true
		}
		if tgtMatches && (s.ForceRecursive == nil || !*s.ForceRecursive) {
			opts.log.Debug("Image matches for platform",
				slog.String("source", src.CommonName()),
				slog.String("platform", s.Platform),
				slog.String("target", tgt.CommonName()))
			return nil
		}
	}
	if tgtMatches {
		opts.log.Info("Image refreshing",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()),
			slog.Bool("forced", forceRecursive),
			slog.Bool("digestTags", digestTags),
			slog.Bool("referrers", referrers))
	} else {
		opts.log.Info("Image sync needed",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
	}
	if action == actionCheck {
		return nil
	}

	// wait for parallel tasks
	throttleDone, err := opts.throttle.Acquire(ctx, throttle{})
	if err != nil {
		return fmt.Errorf("failed to acquire throttle: %w", err)
	}
	// delay for rate limit on source
	if s.RateLimit.Min > 0 && manifest.GetRateLimit(mSrc).Set {
		// refresh current rate limit after acquiring throttle
		mSrc, err = opts.rc.ManifestHead(ctx, src)
		if err != nil {
			opts.log.Error("rate limit check failed",
				slog.String("source", src.CommonName()),
				slog.String("error", err.Error()))
			throttleDone()
			return err
		}
		// delay if rate limit exceeded
		rlSrc := manifest.GetRateLimit(mSrc)
		for rlSrc.Remain < s.RateLimit.Min {
			throttleDone()
			opts.log.Info("Delaying for rate limit",
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
			throttleDone, err = opts.throttle.Acquire(ctx, throttle{})
			if err != nil {
				return fmt.Errorf("failed to reacquire throttle: %w", err)
			}
			mSrc, err = opts.rc.ManifestHead(ctx, src)
			if err != nil {
				opts.log.Error("rate limit check failed",
					slog.String("source", src.CommonName()),
					slog.String("error", err.Error()))
				throttleDone()
				return err
			}
			rlSrc = manifest.GetRateLimit(mSrc)
		}
		opts.log.Debug("Rate limit passed",
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
			opts.log.Error("Failed to expand backup template",
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
				opts.log.Error("Failed to parse backup reference",
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
		defer opts.rc.Close(ctx, backupRef)
		// run copy from tgt ref to backup ref
		opts.log.Info("Saving backup",
			slog.String("original", tgt.CommonName()),
			slog.String("backup", backupRef.CommonName()))
		err = opts.rc.ImageCopy(ctx, tgt, backupRef)
		if err != nil {
			// Possible registry corruption with existing image, only warn and continue/overwrite
			opts.log.Warn("Failed to backup existing image",
				slog.String("original", tgt.CommonName()),
				slog.String("template", s.Backup),
				slog.String("backup", backupRef.CommonName()),
				slog.String("error", err.Error()))
		}
	}

	rcOpts := []regclient.ImageOpts{}
	if s.DigestTags != nil && *s.DigestTags {
		rcOpts = append(rcOpts, regclient.ImageWithDigestTags())
	}
	if s.Referrers != nil && *s.Referrers {
		if len(s.ReferrerFilters) == 0 {
			rcOpts = append(rcOpts, regclient.ImageWithReferrers())
		} else {
			for _, filter := range s.ReferrerFilters {
				rOpts := []scheme.ReferrerOpts{}
				if filter.ArtifactType != "" {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: filter.ArtifactType}))
				}
				if filter.Annotations != nil {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: filter.Annotations}))
				}
				rcOpts = append(rcOpts, regclient.ImageWithReferrers(rOpts...))
			}
		}
		if s.ReferrerSrc != "" {
			referrerSrc, err := ref.New(s.ReferrerSrc)
			if err != nil {
				opts.log.Error("failed to parse referrer source reference",
					slog.String("referrerSource", s.ReferrerSrc),
					slog.String("error", err.Error()))
			}
			rcOpts = append(rcOpts, regclient.ImageWithReferrerSrc(referrerSrc))
		}
		if s.ReferrerTgt != "" {
			referrerTgt, err := ref.New(s.ReferrerTgt)
			if err != nil {
				opts.log.Error("failed to parse referrer target reference",
					slog.String("referrerTarget", s.ReferrerTgt),
					slog.String("error", err.Error()))
			}
			rcOpts = append(rcOpts, regclient.ImageWithReferrerTgt(referrerTgt))
		}
	}
	if s.FastCheck != nil && *s.FastCheck {
		rcOpts = append(rcOpts, regclient.ImageWithFastCheck())
	}
	if s.ForceRecursive != nil && *s.ForceRecursive {
		rcOpts = append(rcOpts, regclient.ImageWithForceRecursive())
	}
	if s.IncludeExternal != nil && *s.IncludeExternal {
		rcOpts = append(rcOpts, regclient.ImageWithIncludeExternal())
	}
	if len(s.Platforms) > 0 {
		rcOpts = append(rcOpts, regclient.ImageWithPlatforms(s.Platforms))
	}

	// Copy the image
	opts.log.Debug("Image sync running",
		slog.String("source", src.CommonName()),
		slog.String("target", tgt.CommonName()))
	err = opts.rc.ImageCopy(ctx, src, tgt, rcOpts...)
	if err != nil {
		opts.log.Error("Failed to copy image",
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
func (opts *rootOpts) getPlatformDigest(ctx context.Context, r ref.Ref, platStr string, origMan manifest.Manifest) (digest.Digest, error) {
	plat, err := platform.Parse(platStr)
	if err != nil {
		opts.log.Warn("Could not parse platform",
			slog.String("platform", platStr),
			slog.String("err", err.Error()))
		return "", err
	}
	// cache manifestGet response
	manifestCache.mu.Lock()
	getMan, ok := manifestCache.manifests[manifest.GetDigest(origMan).String()]
	if !ok {
		getMan, err = opts.rc.ManifestGet(ctx, r)
		if err != nil {
			opts.log.Error("Failed to get source manifest",
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
		opts.log.Warn("Platform could not be found in source manifest list",
			slog.String("platform", plat.String()),
			slog.String("err", err.Error()),
			slog.String("platforms", strings.Join(ps, ", ")))
		return "", ErrNotFound
	}
	return descPlat.Digest, nil
}
