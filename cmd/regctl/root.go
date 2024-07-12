package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/strparse"
	"github.com/regclient/regclient/internal/version"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme/reg"
)

const (
	progressFreq = time.Millisecond * 250
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regctl"
)

// TODO: remove global, configure tests with t.Parallel
var (
	log *logrus.Logger
)

type rootCmd struct {
	name      string
	verbosity string
	logopts   []string
	format    string // for Go template formatting of various commands
	hosts     []string
	userAgent string
}

func NewRootCmd() *cobra.Command {
	rootOpts := rootCmd{}
	var rootTopCmd = &cobra.Command{
		Use:   "regctl <cmd>",
		Short: "Utility for accessing docker registries",
		Long: `Utility for accessing docker registries
More details at https://github.com/regclient/regclient`,
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

	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}

	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	rootTopCmd.PersistentFlags().StringArrayVar(&rootOpts.hosts, "host", []string{}, "Registry hosts to add (reg=registry,user=username,pass=password,tls=enabled)")
	rootTopCmd.PersistentFlags().StringVarP(&rootOpts.userAgent, "user-agent", "", "", "Override user agent")

	_ = rootTopCmd.RegisterFlagCompletionFunc("verbosity", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"debug", "info", "warn", "error", "fatal", "panic"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = rootTopCmd.RegisterFlagCompletionFunc("logopt", completeArgNone)
	_ = rootTopCmd.RegisterFlagCompletionFunc("host", completeArgNone)

	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = versionCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	rootTopCmd.PersistentPreRunE = rootOpts.rootPreRun
	rootTopCmd.AddCommand(versionCmd)
	rootTopCmd.AddCommand(
		NewArtifactCmd(&rootOpts),
		NewBlobCmd(&rootOpts),
		NewCompletionCmd(&rootOpts),
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
	return rootTopCmd
}

func (rootOpts *rootCmd) rootPreRun(cmd *cobra.Command, args []string) error {
	lvl, err := logrus.ParseLevel(rootOpts.verbosity)
	if err != nil {
		return err
	}
	log.SetLevel(lvl)
	for _, opt := range rootOpts.logopts {
		if opt == "json" {
			log.Formatter = new(logrus.JSONFormatter)
		}
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
		log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Failed to load default config")
		if conf == nil {
			conf = ConfigNew()
		}
	}

	rcOpts := []regclient.Opt{
		regclient.WithLog(log),
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

	rcHosts := []config.Host{}
	for name, host := range conf.Hosts {
		host.Name = name
		rcHosts = append(rcHosts, *host)
	}
	for _, h := range rootOpts.hosts {
		hKV, err := strparse.SplitCSKV(h)
		if err != nil {
			log.WithFields(logrus.Fields{
				"host": h,
				"err":  err,
			}).Warn("unable to parse host string")
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
				log.WithFields(logrus.Fields{
					"host": h,
					"tls":  hKV["tls"],
					"err":  err,
				}).Warn("unable to parse tls setting")
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
