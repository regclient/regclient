package main

import (
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/ref"
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
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runTagDelete,
}
var tagLsCmd = &cobra.Command{
	Use:     "ls <repository>",
	Aliases: []string{"list"},
	Short:   "list tags in a repo",
	Long: `List tags in a repository.
Note: most registries ignore the pagination options.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{},
	RunE:      runTagLs,
}

var tagOpts struct {
	Limit  int
	Last   string
	format string
}

func init() {
	tagLsCmd.Flags().StringVarP(&tagOpts.Last, "last", "", "", "Specify the last tag from a previous request for pagination")
	tagLsCmd.Flags().IntVarP(&tagOpts.Limit, "limit", "", 0, "Specify the number of tags to retrieve")
	tagLsCmd.Flags().StringVarP(&tagOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	tagLsCmd.RegisterFlagCompletionFunc("last", completeArgNone)
	tagLsCmd.RegisterFlagCompletionFunc("limit", completeArgNone)
	tagLsCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	tagCmd.AddCommand(tagDeleteCmd)
	tagCmd.AddCommand(tagLsCmd)
	rootCmd.AddCommand(tagCmd)
}

func runTagDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	defer rc.Close(ctx, r)
	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
		"tag":        r.Tag,
	}).Debug("Delete tag")
	err = rc.TagDelete(ctx, r)
	if err != nil {
		return err
	}
	return nil
}

func runTagLs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	defer rc.Close(ctx, r)
	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
	}).Debug("Listing tags")
	opts := []scheme.TagOpts{}
	if tagOpts.Limit != 0 {
		opts = append(opts, scheme.WithTagLimit(tagOpts.Limit))
	}
	if tagOpts.Last != "" {
		opts = append(opts, scheme.WithTagLast(tagOpts.Last))
	}
	tl, err := rc.TagList(ctx, r, opts...)
	if err != nil {
		return err
	}
	switch tagOpts.format {
	case "raw":
		tagOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		tagOpts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		tagOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), tagOpts.format, tl)
}
