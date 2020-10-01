package main

import (
	"fmt"
	"os"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const usageDesc = `Utility for accessing docker registries
More details at https://github.com/regclient/regclient`

var log *logrus.Logger

var rootCmd = &cobra.Command{
	Use:   "regctl <cmd>",
	Short: "Utility for accessing docker registries",
	Long:  usageDesc,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
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

// Execute runs the CLI using cobra
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
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
	return regclient.NewRegClient(regclient.WithLog(log), regclient.WithConfigDefault(), regclient.WithDockerCreds(), regclient.WithDockerCerts())
}
