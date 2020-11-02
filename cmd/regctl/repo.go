package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo <cmd>",
	Short: "manage repositories",
}
var repoLsCmd = &cobra.Command{
	Use:     "ls <registry>",
	Aliases: []string{"list"},
	Short:   "list repositories in a registry",
	Long: `List repositories in a registry.
Note: Docker Hub does not support this API request.`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runRepoLs,
}

var repoOpts regclient.RepoOpts

func init() {
	repoLsCmd.Flags().StringVarP(&repoOpts.Last, "last", "", "", "Specify the last repo from a previous request for pagination")
	repoLsCmd.Flags().IntVarP(&repoOpts.Limit, "limit", "", 0, "Specify the number of repos to retrieve")

	repoCmd.AddCommand(repoLsCmd)
	rootCmd.AddCommand(repoCmd)
}

func runRepoLs(cmd *cobra.Command, args []string) error {
	host := args[0]
	// TODO: use regex to validate hostname + port
	i := strings.IndexRune(host, '/')
	if i >= 0 {
		log.WithFields(logrus.Fields{
			"host": host,
		}).Error("Hostname invalid")
		return ErrInvalidInput
	}
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"host":  host,
		"last":  repoOpts.Last,
		"limit": repoOpts.Limit,
	}).Debug("Listing repositories")
	rl, err := rc.RepoListWithOpts(context.Background(), host, repoOpts)
	if err != nil {
		return err
	}
	for _, r := range rl.Repositories {
		fmt.Printf("%s\n", r)
	}
	return nil
}
