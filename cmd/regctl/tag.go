package main

import (
	"fmt"
	"log/slog"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/ref"
)

type tagCmd struct {
	rootOpts *rootCmd
	limit    int
	last     string
	include  []string
	exclude  []string
	format   string
}

func NewTagCmd(rootOpts *rootCmd) *cobra.Command {
	tagOpts := tagCmd{
		rootOpts: rootOpts,
	}
	var tagTopCmd = &cobra.Command{
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
If the registry does not support the delete API, the dummy manifest will remain.`,
		Example: `
# delete a tag
regctl tag delete registry.example.org/repo:v42`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              tagOpts.runTagDelete,
	}
	var tagLsCmd = &cobra.Command{
		Use:     "ls <repository>",
		Aliases: []string{"list"},
		Short:   "list tags in a repo",
		Long: `List tags in a repository.
Note: many registries ignore the pagination options.
For an OCI Layout, the index is available as Index (--format "{{.Index}}").`,
		Example: `
# list all tags in a repository
regctl tag ls registry.example.org/repo

# exclude tags starting with sha256- from the listing
regctl tag ls registry.example.org/repo --exclude 'sha256-.*'`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{},
		RunE:      tagOpts.runTagLs,
	}

	tagLsCmd.Flags().StringVarP(&tagOpts.last, "last", "", "", "Specify the last tag from a previous request for pagination (depends on registry support)")
	_ = tagLsCmd.RegisterFlagCompletionFunc("last", completeArgNone)
	tagLsCmd.Flags().IntVarP(&tagOpts.limit, "limit", "", 0, "Specify the number of tags to retrieve (depends on registry support)")
	_ = tagLsCmd.RegisterFlagCompletionFunc("limit", completeArgNone)
	tagLsCmd.Flags().StringArrayVar(&tagOpts.include, "include", []string{}, "Regexp of tags to include (expression is bound to beginning and ending of tag)")
	_ = tagLsCmd.RegisterFlagCompletionFunc("include", completeArgNone)
	tagLsCmd.Flags().StringArrayVar(&tagOpts.exclude, "exclude", []string{}, "Regexp of tags to exclude (expression is bound to beginning and ending of tag)")
	_ = tagLsCmd.RegisterFlagCompletionFunc("exclude", completeArgNone)
	tagLsCmd.Flags().StringVarP(&tagOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = tagLsCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	tagTopCmd.AddCommand(tagDeleteCmd)
	tagTopCmd.AddCommand(tagLsCmd)
	return tagTopCmd
}

func (tagOpts *tagCmd) runTagDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := tagOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)
	tagOpts.rootOpts.log.Debug("Delete tag",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository),
		slog.String("tag", r.Tag))
	err = rc.TagDelete(ctx, r)
	if err != nil {
		return err
	}
	return nil
}

func (tagOpts *tagCmd) runTagLs(cmd *cobra.Command, args []string) error {
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
	rc := tagOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)
	tagOpts.rootOpts.log.Debug("Listing tags",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository))
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
