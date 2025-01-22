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

type rootCmd struct {
	name      string
	verbosity string
	logopts   []string
	log       *slog.Logger
	format    string // for Go template formatting of various commands
	hosts     []string
	userAgent string
}

func NewRootCmd() (*cobra.Command, *rootCmd) {
	rootOpts := rootCmd{}
	var rootTopCmd = &cobra.Command{
		Use:   "regctl <cmd>",
		Short: "Utility for accessing docker registries",
		Long: `Utility for accessing docker registries
More details at <https://github.com/regclient/regclient>`,
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
	rootOpts.name = rootTopCmd.Name()
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long:  fmt.Sprintf(`Show the version of %s`, rootOpts.name),
		Example: `
# display full version details
regctl version

# retrieve the version number
regctl version --format '{{.VCSTag}}'`,
		Args: cobra.ExactArgs(0),
		RunE: rootOpts.runVersion,
	}

	rootOpts.log = slog.New(slog.NewTextHandler(rootTopCmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))

	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", slog.LevelWarn.String(), "Log level (trace, debug, info, warn, error, fatal, panic)")
	_ = rootTopCmd.RegisterFlagCompletionFunc("verbosity", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}, cobra.ShellCompDirectiveNoFileComp
	})
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	_ = rootTopCmd.RegisterFlagCompletionFunc("logopt", completeArgNone)
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.hosts, "host", []string{}, "Registry hosts to add (reg=registry,user=username,pass=password,tls=enabled)")
	_ = rootTopCmd.RegisterFlagCompletionFunc("host", completeArgNone)
	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.userAgent, "user-agent", "", "", "Override user agent")
	_ = rootTopCmd.RegisterFlagCompletionFunc("user-agent", completeArgNone)

	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = versionCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	rootTopCmd.PersistentPreRunE = rootOpts.rootPreRun
	rootTopCmd.AddCommand(versionCmd)
	rootTopCmd.AddCommand(cobradoc.NewCmd(rootOpts.name, "cli-doc"))
	rootTopCmd.AddCommand(
		NewArtifactCmd(&rootOpts),
		NewBlobCmd(&rootOpts),
		NewConfigCmd(&rootOpts),
		NewDigestCmd(&rootOpts),
		NewImageCmd(&rootOpts),
		NewIndexCmd(&rootOpts),
		NewManifestCmd(&rootOpts),
		NewRefCmd(&rootOpts),
		NewRegistryCmd(&rootOpts),
		NewRepoCmd(&rootOpts),
		NewTagCmd(&rootOpts),
	)
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
	return template.Writer(cmd.OutOrStdout(), rootOpts.format, info)
}

func (rootOpts *rootCmd) newRegClient() *regclient.RegClient {
	conf, err := ConfigLoadDefault()
	if err != nil {
		rootOpts.log.Warn("Failed to load default config",
			slog.String("err", err.Error()))
		if conf == nil {
			conf = ConfigNew()
		}
	}

	rcOpts := []regclient.Opt{
		regclient.WithSlog(rootOpts.log),
		regclient.WithRegOpts(reg.WithCache(time.Minute*5, 500)),
	}
	if rootOpts.userAgent != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(rootOpts.userAgent))
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
	for _, h := range rootOpts.hosts {
		hKV, err := strparse.SplitCSKV(h)
		if err != nil {
			rootOpts.log.Warn("unable to parse host string",
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
				rootOpts.log.Warn("unable to parse tls setting",
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

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	return flag.Changed
}
