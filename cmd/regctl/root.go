package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/cobradoc"
	"github.com/regclient/regclient/internal/strparse"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
)

const (
	progressFreq = time.Millisecond * 250
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regctl"
)

type rootOpts struct {
	hosts     []string
	name      string
	logopts   []string
	log       *slog.Logger
	rcOpts    []regclient.Opt
	userAgent string
	verbosity string
}

type versionOpts struct {
	rootOpts *rootOpts
	format   string
}

func NewRootCmd() (*cobra.Command, *rootOpts) {
	rOpts := &rootOpts{}
	cmd := &cobra.Command{
		Use:   "regctl <cmd>",
		Short: "Utility for accessing docker registries",
		Long: `Utility for accessing docker registries
More details at <https://regclient.org>`,
		Example: `
# login to ghcr.io
regctl registry login ghcr.io

# configure a local registry for http
regctl registry set --tls disabled registry.example.org

# copy an image from ghcr.io to local registry
regctl image copy ghcr.io/regclient/regctl:latest registry.example.org/regctl:latest

# show debugging output from a command
regctl tag ls ghcr.io/regclient/regctl -v debug

# format log output in json
regctl image ratelimit --logopt json alpine

# override registry config for a single command
regctl image digest --host reg=localhost:5000,tls=disabled localhost:5000/repo:v1`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rOpts.name = cmd.Name()
	rOpts.log = slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))

	cmd.PersistentFlags().StringVarP(&rOpts.verbosity, "verbosity", "v", slog.LevelWarn.String(), "Log level (trace, debug, info, warn, error)")
	_ = cmd.RegisterFlagCompletionFunc("verbosity", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"trace", "debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.PersistentFlags().StringArrayVar(&rOpts.logopts, "logopt", []string{}, "Log options")
	_ = cmd.RegisterFlagCompletionFunc("logopt", completeArgNone)
	cmd.PersistentFlags().StringArrayVar(&rOpts.hosts, "host", []string{}, "Registry hosts to add (reg=registry,user=username,pass=password,tls=enabled)")
	_ = cmd.RegisterFlagCompletionFunc("host", completeArgNone)
	cmd.PersistentFlags().StringVarP(&rOpts.userAgent, "user-agent", "", "", "Override user agent")
	_ = cmd.RegisterFlagCompletionFunc("user-agent", completeArgNone)

	cmd.PersistentPreRunE = rOpts.rootPreRun
	cmd.AddCommand(cobradoc.NewCmd(rOpts.name, "cli-doc"))
	cmd.AddCommand(
		NewArtifactCmd(rOpts),
		NewBlobCmd(rOpts),
		NewConfigCmd(rOpts),
		NewDigestCmd(rOpts),
		NewImageCmd(rOpts),
		NewIndexCmd(rOpts),
		NewManifestCmd(rOpts),
		NewRefCmd(rOpts),
		NewRegistryCmd(rOpts),
		NewRepoCmd(rOpts),
		NewTagCmd(rOpts),
		newVersionCmd(rOpts),
	)
	return cmd, rOpts
}

func newVersionCmd(rOpts *rootOpts) *cobra.Command {
	opts := versionOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  fmt.Sprintf(`Show the version of %s. Note that docker image builds will always be marked "dirty".`, opts.rootOpts.name),
		Example: fmt.Sprintf(`
# display full version details
%[1]s version

# retrieve the version number
%[1]s version --format '{{.VCSTag}}'`, opts.rootOpts.name),
		Args: cobra.ExactArgs(0),
		RunE: opts.runVersion,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
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

func (opts *rootOpts) newRegClient() *regclient.RegClient {
	conf, err := ConfigLoadDefault()
	if err != nil {
		opts.log.Warn("Failed to load default config",
			slog.String("err", err.Error()))
		if conf == nil {
			conf = ConfigNew()
		}
	}

	rcOpts := []regclient.Opt{
		regclient.WithSlog(opts.log),
		regclient.WithRegOpts(reg.WithCache(time.Minute*5, 500)),
	}
	if len(opts.rcOpts) > 0 {
		rcOpts = append(rcOpts, opts.rcOpts...)
	}
	if opts.userAgent != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(opts.userAgent))
	} else {
		info := version.GetInfo()
		if info.VCSTag != "" {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSTag+")"))
		} else {
			rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+info.VCSRef+")"))
		}
	}
	if conf.BlobLimit != 0 {
		rcOpts = append(rcOpts, regclient.WithRegOpts(reg.WithBlobLimit(conf.BlobLimit)))
	}
	if conf.IncDockerCred == nil || *conf.IncDockerCred {
		rcOpts = append(rcOpts, regclient.WithDockerCreds())
	}
	if conf.IncDockerCert == nil || *conf.IncDockerCert {
		rcOpts = append(rcOpts, regclient.WithDockerCerts())
	}
	if conf.HostDefault != nil {
		rcOpts = append(rcOpts, regclient.WithConfigHostDefault(*conf.HostDefault))
	}

	rcHosts := []config.Host{}
	for name, host := range conf.Hosts {
		host.Name = name
		rcHosts = append(rcHosts, *host)
	}
	for _, h := range opts.hosts {
		hKV, err := strparse.SplitCSKV(h)
		if err != nil {
			opts.log.Warn("unable to parse host string",
				slog.String("host", h),
				slog.String("err", err.Error()))
		}
		host := config.Host{
			Name: hKV["reg"],
			User: hKV["user"],
			Pass: hKV["pass"],
		}
		if hKV["tls"] != "" {
			var hostTLS config.TLSConf
			err := hostTLS.UnmarshalText([]byte(hKV["tls"]))
			if err != nil {
				opts.log.Warn("unable to parse tls setting",
					slog.String("host", h),
					slog.String("tls", hKV["tls"]),
					slog.String("err", err.Error()))
			} else {
				host.TLS = hostTLS
			}
		}
		rcHosts = append(rcHosts, host)
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHost(rcHosts...))
	}

	return regclient.New(rcOpts...)
}

func (opts *versionOpts) runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()
	return template.Writer(cmd.OutOrStdout(), opts.format, info)
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	return flag.Changed
}
