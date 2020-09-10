package main

import (
	"context"
	"fmt"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo <cmd>",
	Short: "manage a repository",
}
var repoLsCmd = &cobra.Command{
	Use:   "ls <repository>",
	Short: "list tags in a repo",
	Long:  `List all tags for a repository`,
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runRepoLs,
}

func init() {
	repoCmd.AddCommand(repoLsCmd)
	rootCmd.AddCommand(repoCmd)
}

func runRepoLs(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"host":       ref.Registry,
		"repository": ref.Repository,
	}).Debug("Listing tags")
	tl, err := rc.TagsList(context.Background(), ref)
	if err != nil {
		return err
	}
	for _, tag := range tl.Tags {
		fmt.Printf("%s:%s\n", tl.Name, tag)
	}
	return nil
}
