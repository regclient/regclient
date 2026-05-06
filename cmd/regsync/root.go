package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/regclient/regclient/pkg/regsync"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/cobradoc"
	"github.com/regclient/regclient/internal/pqueue"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
)

const (
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regsync"
)

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
	throttle   *pqueue.Queue[regsync.Throttle]
	rs         *regsync.Regsync
}

func NewRootCmd() (*cobra.Command, *rootOpts) {
	opts := rootOpts{}
	cmd := &cobra.Command{
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

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "run the regsync server",
		Long:  `Sync registries according to the configuration.`,
		Args:  cobra.RangeArgs(0, 0),
		RunE:  opts.runServer,
	}
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "processes each sync command once but skip actual copy",
		Long: `Processes each sync command in the configuration file in order.
Manifests are checked to see if a copy is needed, but only log, skip copying.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: opts.runCheck,
	}
	onceCmd := &cobra.Command{
		Use:   "once",
		Short: "processes each sync command once, ignoring cron schedule",
		Long: `Processes each sync command in the configuration file in order.
No jobs are run in parallel, and the command returns after any error or last
sync step is finished.`,
		Args: cobra.RangeArgs(0, 0),
		RunE: opts.runOnce,
	}
	onceCmd.Flags().BoolVar(&opts.missing, "missing", false, "Only copy tags that are missing on target")
	configCmd := &cobra.Command{
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

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  fmt.Sprintf(`Show the version of %s. Note that docker image builds will always be marked "dirty".`, cmd.Name()),
		Example: fmt.Sprintf(`
# display full version details
%[1]s version

# retrieve the version number
%[1]s version --format '{{.VCSTag}}'`, cmd.Name()),
		Args: cobra.ExactArgs(0),
		RunE: opts.runVersion,
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
	action := regsync.ActionCopy
	if opts.missing {
		action = regsync.ActionMissing
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := []error{}
	for _, s := range opts.conf.Sync {
		if opts.conf.Defaults.Parallel > 0 {
			wg.Go(func() {
				err := opts.rs.Process(ctx, s, action)
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, regsync.ErrCanceled) {
					if opts.abortOnErr {
						cancel()
					}
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			})
		} else {
			err := opts.rs.Process(ctx, s, action)
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
				err := opts.rs.Process(ctx, s, regsync.ActionCopy)
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, regsync.ErrCanceled) {
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
				wg.Go(func() {
					err := opts.rs.Process(ctx, s, regsync.ActionMissing)
					if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, regsync.ErrCanceled) {
						if opts.abortOnErr {
							cancel()
						}
						mu.Lock()
						errs = append(errs, err)
						mu.Unlock()
					}
				})
			} else {
				err := opts.rs.Process(ctx, s, regsync.ActionMissing)
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, regsync.ErrCanceled) {
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
		err := opts.rs.Process(ctx, s, regsync.ActionCheck)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, regsync.ErrCanceled) {
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
		return regsync.ErrMissingInput
	}
	// use a throttle to control parallelism
	concurrent := opts.conf.Defaults.Parallel
	if concurrent <= 0 {
		concurrent = 1
	}
	opts.log.Debug("Configuring parallel settings",
		slog.Int("concurrent", concurrent))
	opts.throttle = pqueue.New(pqueue.Opts[regsync.Throttle]{Max: concurrent})
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

	rsOpts := []regsync.Opt{
		regsync.WithAbortOnErr(opts.abortOnErr),
		regsync.WithThrottle(opts.throttle),
	}
	opts.rs = regsync.New(opts.rc, rsOpts...)

	return nil
}
