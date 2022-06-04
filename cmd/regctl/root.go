package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/pkg/template"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	usageDesc = `Utility for accessing docker registries
More details at https://github.com/regclient/regclient`
	// UserAgent sets the header on http requests
	UserAgent = "regclient/regctl"
)

//go:embed embed/*
var embedFS embed.FS

var (
	// VCSRef and VCSTag are populated from an embed at build time
	// These are used to version the UserAgent header
	VCSRef = ""
	VCSTag = ""
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
	userAgent string
}

func init() {
	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}
	setupVCSVars()

	rootCmd.PersistentFlags().StringVarP(&rootOpts.verbosity, "verbosity", "v", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().StringArrayVar(&rootOpts.logopts, "logopt", []string{}, "Log options")
	rootCmd.PersistentFlags().StringVarP(&rootOpts.userAgent, "user-agent", "", "", "Override user agent")

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
	return template.Writer(os.Stdout, rootOpts.format, ver)
}

func newRegClient() *regclient.RegClient {
	conf, err := ConfigLoadDefault()
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Failed to load default config")
	}

	rcOpts := []regclient.Opt{
		regclient.WithLog(log),
	}
	if rootOpts.userAgent != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(rootOpts.userAgent))
	} else if VCSTag != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+VCSTag+")"))
	} else if VCSRef != "" {
		rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" ("+VCSRef+")"))
	} else {
		rcOpts = append(rcOpts, regclient.WithUserAgent(UserAgent+" (unknown)"))
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
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHosts(rcHosts))
	}

	return regclient.New(rcOpts...)
}

func setupVCSVars() {
	verS := struct {
		VCSRef string
		VCSTag string
	}{}

	verB, err := embedFS.ReadFile("embed/version.json")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return
	}

	if len(verB) > 0 {
		err = json.Unmarshal(verB, &verS)
		if err != nil {
			return
		}
	}

	if verS.VCSRef != "" {
		VCSRef = verS.VCSRef
	}
	if verS.VCSTag != "" {
		VCSTag = verS.VCSTag
	}
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	return flag.Changed
}
