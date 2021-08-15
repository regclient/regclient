package main

import (
	"os"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	usageDesc = `Utility for accessing docker registries
More details at https://github.com/regclient/regclient`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regctl"
)

var (
	// VCSRef is injected from a build flag, used to version the UserAgent header
	VCSRef = "unknown"
	// VCSTag is injected from a build flag
	VCSTag = "unknown"
	log    *logrus.Logger
)

var rootCmd = &cobra.Command{
	Use:           "regctl <cmd>",
	Short:         "Utility for accessing docker registries",
	Long:          usageDesc,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the version",
	Long:  `Show the version`,
	Args:  cobra.ExactArgs(0),
	RunE:  runVersion,
}

var rootOpts struct {
	verbosity string
	logopts   []string
	format    string // for Go template formatting of various commands
}

func init() {
	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}

	rootCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	rootCmd.RegisterFlagCompletionFunc("verbosity", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"debug", "info", "warn", "error", "fatal", "panic"}, cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.RegisterFlagCompletionFunc("logopt", completeArgNone)

	versionCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{jsonPretty .}}", "Format output with go template syntax")
	versionCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	rootCmd.PersistentPreRunE = rootPreRun
	rootCmd.AddCommand(versionCmd)
}

func rootPreRun(cmd *cobra.Command, args []string) error {
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

func runVersion(cmd *cobra.Command, args []string) error {
	ver := struct {
		VCSRef string
		VCSTag string
	}{
		VCSRef: VCSRef,
		VCSTag: VCSTag,
	}
	return template.Writer(os.Stdout, rootOpts.format, ver, template.WithFuncs(regclient.TemplateFuncs))
}

func newRegClient() regclient.RegClient {
	config, err := ConfigLoadDefault()
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Failed to load default config")
	}

	rcOpts := []regclient.Opt{
		regclient.WithLog(log),
		regclient.WithUserAgent(UserAgent + " (" + VCSRef + ")"),
	}
	if config.IncDockerCred == nil || *config.IncDockerCred {
		rcOpts = append(rcOpts, regclient.WithDockerCreds())
	}
	if config.IncDockerCert == nil || *config.IncDockerCert {
		rcOpts = append(rcOpts, regclient.WithDockerCerts())
	}

	rcHosts := []regclient.ConfigHost{}
	for name, host := range config.Hosts {
		rcHosts = append(rcHosts, regclient.ConfigHost{
			Name:       name,
			TLS:        host.TLS,
			RegCert:    host.RegCert,
			Hostname:   host.Hostname,
			User:       host.User,
			Pass:       host.Pass,
			Token:      host.Token,
			PathPrefix: host.PathPrefix,
			Mirrors:    host.Mirrors,
			Priority:   host.Priority,
			API:        host.API,
			BlobChunk:  host.BlobChunk,
			BlobMax:    host.BlobMax,
		})
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHosts(rcHosts))
	}

	return regclient.NewRegClient(rcOpts...)
}
