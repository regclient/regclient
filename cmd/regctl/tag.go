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

var tagOpts struct {
	Limit     int
	Last      string
	format    string
	raw       bool
	rawBody   bool
	rawHeader bool
}

func init() {
	tagLsCmd.Flags().StringVarP(&tagOpts.Last, "last", "", "", "Specify the last tag from a previous request for pagination")
	tagLsCmd.Flags().IntVarP(&tagOpts.Limit, "limit", "", 0, "Specify the number of tags to retrieve")
	tagLsCmd.Flags().StringVarP(&tagOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	tagLsCmd.Flags().BoolVarP(&tagOpts.raw, "raw", "", false, "Show raw response (overrides format)")
	tagLsCmd.Flags().BoolVarP(&tagOpts.rawBody, "raw-body", "", false, "Show raw body (overrides format)")
	tagLsCmd.Flags().BoolVarP(&tagOpts.rawHeader, "raw-header", "", false, "Show raw headers (overrides format)")

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
	tl, err := rc.TagListWithOpts(context.Background(), ref, regclient.TagOpts{Limit: tagOpts.Limit, Last: tagOpts.Last})
	if err != nil {
		return err
	}
	if tagOpts.raw {
		tagOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	} else if tagOpts.rawBody {
		tagOpts.format = "{{printf \"%s\" .RawBody}}"
	} else if tagOpts.rawHeader {
		tagOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(os.Stdout, tagOpts.format, tl, template.WithFuncs(regclient.TemplateFuncs))
}
