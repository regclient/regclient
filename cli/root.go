package cli

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/sudo-bmitch/regcli/regclient"
)

const usageDesc = `Utility for accessing docker registries
More details at https://github.com/sudo-bmitch/regcli`

var rootCmd = &cobra.Command{
	Use:   "regcli",
	Short: "Utility for accessing docker registries",
	Long:  usageDesc,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}
var verbosity string

func init() {
	rootCmd.PersistentFlags().StringVarP(&verbosity, "verbosity", "v", log.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")
	rootCmd.PersistentPreRunE = rootPreRun
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func rootPreRun(cmd *cobra.Command, args []string) error {
	lvl, err := log.ParseLevel(verbosity)
	if err != nil {
		return err
	}
	log.SetLevel(lvl)
	return nil
}

func newRegClient() regclient.RegClient {
	return regclient.NewRegClient(regclient.WithConfigDefault(), regclient.WithDockerCreds())
}
