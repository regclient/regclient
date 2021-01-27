package main

import (
	"context"
	"os"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag <cmd>",
	Short: "manage tags",
}
var tagDeleteCmd = &cobra.Command{
	Use:     "delete <image_ref>",
	Aliases: []string{"del", "rm", "remove"},
	Short:   "delete a tag in a repo",
	Long: `Delete a tag in a repository without removing other tags pointing to the
same manifest`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runTagDelete,
}
var tagLsCmd = &cobra.Command{
	Use:     "ls <repository>",
	Aliases: []string{"list"},
	Short:   "list tags in a repo",
	Long: `List tags in a repository.
Note: most registries ignore the pagination options.`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runTagLs,
}

var tagOpts regclient.TagOpts

func init() {
	tagLsCmd.Flags().StringVarP(&tagOpts.Last, "last", "", "", "Specify the last tag from a previous request for pagination")
	tagLsCmd.Flags().IntVarP(&tagOpts.Limit, "limit", "", 0, "Specify the number of tags to retrieve")
	tagLsCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{jsonPretty .}}", "Format output with go template syntax")

	tagCmd.AddCommand(tagDeleteCmd)
	tagCmd.AddCommand(tagLsCmd)
	rootCmd.AddCommand(tagCmd)
}

func runTagDelete(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"host":       ref.Registry,
		"repository": ref.Repository,
		"tag":        ref.Tag,
	}).Debug("Delete tag")
	err = rc.TagDelete(context.Background(), ref)
	if err != nil {
		return err
	}
	return nil
}

func runTagLs(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"host":       ref.Registry,
		"repository": ref.Repository,
	}).Debug("Listing tags")
	tl, err := rc.TagListWithOpts(context.Background(), ref, tagOpts)
	if err != nil {
		return err
	}
	return template.Writer(os.Stdout, rootOpts.format, tl)
}
