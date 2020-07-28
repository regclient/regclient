package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sudo-bmitch/regcli/regclient"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "manage repo",
}
var repoLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "list tags in a repo",
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
	rc := regclient.NewRegClient(regclient.WithDockerCreds())
	// fmt.Printf("Listing host: %s, repo: %s\n", ref.Registry, ref.Repository)
	tl, err := rc.TagsList(context.Background(), ref)
	if err != nil {
		return err
	}
	for _, tag := range tl.Tags {
		fmt.Printf("%s:%s\n", tl.Name, tag)
	}
	return nil
}
