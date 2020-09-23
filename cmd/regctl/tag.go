package main

import (
	"context"
	"fmt"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag <cmd>",
	Short: "manage tags",
}
var tagDeleteCmd = &cobra.Command{
	Use:   "delete <image_ref>",
	Short: "delete a tag in a repo",
	Long: `Delete a tag in a repository without removing other tags pointing to the
same manifest`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runTagDelete,
}
var tagLsCmd = &cobra.Command{
	Use:   "ls <repository>",
	Short: "list tags in a repo",
	Long:  `List all tags for a repository`,
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runTagLs,
}

func init() {
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
	tl, err := rc.TagsList(context.Background(), ref)
	if err != nil {
		return err
	}
	for _, tag := range tl.Tags {
		fmt.Printf("%s:%s\n", tl.Name, tag)
	}
	return nil
}
