package main

import (
	"fmt"
	"regexp"

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
	Long: `Delete a tag in a repository.
This avoids deleting the manifest when multiple tags reference the same image.
For registries that do not support the OCI tag delete API, this is implemented
by pushing a unique dummy manifest and deleting that by digest.
If the registry does not support the delete API, the dummy manifest will remain.
`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runTagDelete,
}
var tagLsCmd = &cobra.Command{
	Use:     "ls <repository>",
	Aliases: []string{"list"},
	Short:   "list tags in a repo",
	Long: `List tags in a repository.
Note: many registries ignore the pagination options.
For an OCI Layout, the index is available as Index (--format "{{.Index}}").
`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{},
	RunE:      runTagLs,
}

var tagOpts struct {
	limit   int
	last    string
	include []string
	exclude []string
	format  string
}

func init() {
	tagLsCmd.Flags().StringVarP(&tagOpts.last, "last", "", "", "Specify the last tag from a previous request for pagination (depends on registry support)")
	tagLsCmd.Flags().IntVarP(&tagOpts.limit, "limit", "", 0, "Specify the number of tags to retrieve (depends on registry support)")
	tagLsCmd.Flags().StringArrayVar(&tagOpts.include, "include", []string{}, "Regexp of tags to include (expression is bound to beginning and ending of tag)")
	tagLsCmd.Flags().StringArrayVar(&tagOpts.exclude, "exclude", []string{}, "Regexp of tags to exclude (expression is bound to beginning and ending of tag)")
	tagLsCmd.Flags().StringVarP(&tagOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = tagLsCmd.RegisterFlagCompletionFunc("last", completeArgNone)
	_ = tagLsCmd.RegisterFlagCompletionFunc("limit", completeArgNone)
	_ = tagLsCmd.RegisterFlagCompletionFunc("filter", completeArgNone)
	_ = tagLsCmd.RegisterFlagCompletionFunc("format", completeArgNone)

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
	reInclude := []*regexp.Regexp{}
	reExclude := []*regexp.Regexp{}
	for _, expr := range tagOpts.include {
		re, err := regexp.Compile("^" + expr + "$")
		if err != nil {
			return fmt.Errorf("failed to parse regexp \"%s\": %w", expr, err)
		}
		reInclude = append(reInclude, re)
	}
	for _, expr := range tagOpts.exclude {
		re, err := regexp.Compile("^" + expr + "$")
		if err != nil {
			return fmt.Errorf("failed to parse regexp \"%s\": %w", expr, err)
		}
		reExclude = append(reExclude, re)
	}
	rc := newRegClient()
	defer rc.Close(ctx, r)
	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
	}).Debug("Listing tags")
	opts := []scheme.TagOpts{}
	if tagOpts.limit != 0 {
		opts = append(opts, scheme.WithTagLimit(tagOpts.limit))
	}
	if tagOpts.last != "" {
		opts = append(opts, scheme.WithTagLast(tagOpts.last))
	}
	tl, err := rc.TagList(ctx, r, opts...)
	if err != nil {
		return err
	}
	if len(reInclude) > 0 || len(reExclude) > 0 {
		filtered := []string{}
		var included, excluded bool
		for _, tag := range tl.Tags {
			included = len(reInclude) == 0
			excluded = false
			for _, re := range reInclude {
				if re.MatchString(tag) {
					included = true
					break
				}
			}
			if included {
				for _, re := range reExclude {
					if re.MatchString(tag) {
						excluded = true
					}
				}
			}
			if included && !excluded {
				filtered = append(filtered, tag)
			}
		}
		tl.Tags = filtered
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
