package main

import (
	"os"

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
	log    *logrus.Logger
)

var rootCmd = &cobra.Command{
	Use:           "regctl <cmd>",
	Short:         "Utility for accessing docker registries",
	Long:          usageDesc,
	SilenceUsage:  true,
	SilenceErrors: true,
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
	rootCmd.PersistentPreRunE = rootPreRun
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
			PathPrefix: host.PathPrefix,
			Mirrors:    host.Mirrors,
			Priority:   host.Priority,
			API:        host.API,
		})
	}
	if len(rcHosts) > 0 {
		rcOpts = append(rcOpts, regclient.WithConfigHosts(rcHosts))
	}

	return regclient.NewRegClient(rcOpts...)
}
