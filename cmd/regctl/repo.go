package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

type repoOpts struct {
	rootOpts   *rootOpts
	concurrent int
	exclude    []string
	format     string
	include    []string
	last       string
	limit      int
	newTags    bool
	referrers  bool
}

func NewRepoCmd(rOpts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <cmd>",
		Short: "manage repositories",
	}
	cmd.AddCommand(newRepoCopyCmd(rOpts))
	cmd.AddCommand(newRepoLsCmd(rOpts))
	return cmd
}

func newRepoCopyCmd(rOpts *rootOpts) *cobra.Command {
	opts := repoOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "copy <source_repo> <dest_repo>",
		Aliases: []string{"cp"},
		Short:   "copy a repository",
		Long: `Copy images from the source to destination repository.
Existing images in the destination are not deleted, but tags may be overwritten.
If include/exclude options are provided, only entries that match one include
option and are not excluded by any exclude option are copied.`,
		Example: `
# copy all tags from the a to b
regctl repo copy registry-a.example.org/repo registry-b.example.org/repo

# copy all tags beginning with v1.2
regctl repo copy --include 'v1\\.2.*' registry-a.example.org/repo registry-b.example.org/repo
		`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeArgNone,
		RunE:              opts.runRepoCopy,
	}
	cmd.Flags().IntVar(&opts.concurrent, "concurrent", 2, "Number of concurrent images to copy")
	cmd.Flags().StringArrayVar(&opts.exclude, "exclude", []string{}, "Exclude tags by regexp")
	_ = cmd.RegisterFlagCompletionFunc("exclude", completeArgNone)
	cmd.Flags().StringArrayVar(&opts.include, "include", []string{}, "Include tags by regexp")
	_ = cmd.RegisterFlagCompletionFunc("include", completeArgNone)
	cmd.Flags().BoolVar(&opts.newTags, "new-tags", false, "Only copy tags that do not exist in destination repo")
	cmd.Flags().BoolVar(&opts.referrers, "referrers", false, "Include referrers")
	return cmd
}

func newRepoLsCmd(rOpts *rootOpts) *cobra.Command {
	opts := repoOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "ls <registry>",
		Aliases: []string{"list"},
		Short:   "list repositories in a registry",
		Long: `List repositories in a registry.
Note: Docker Hub does not support this API request.`,
		Example: `
# list all repositories
regctl repo ls registry.example.org

# list the next 5 repositories after repo1
regctl repo ls --last repo1 --limit 5 registry.example.org`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: registryArgListReg,
		RunE:              opts.runRepoLs,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().StringVarP(&opts.last, "last", "", "", "Specify the last repo from a previous request for pagination")
	_ = cmd.RegisterFlagCompletionFunc("last", completeArgNone)
	cmd.Flags().IntVarP(&opts.limit, "limit", "", 0, "Specify the number of repos to retrieve")
	_ = cmd.RegisterFlagCompletionFunc("limit", completeArgNone)
	return cmd
}

func (opts *repoOpts) runRepoCopy(cmd *cobra.Command, args []string) error {
	var err error
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	// compile regexp include/exclude list
	incRe := make([]*regexp.Regexp, len(opts.include))
	for i, str := range opts.include {
		incRe[i], err = regexp.Compile("^" + str + "$")
		if err != nil {
			return fmt.Errorf("failed to parse include regexp: %q, %w", str, err)
		}
	}
	excRe := make([]*regexp.Regexp, len(opts.exclude))
	for i, str := range opts.exclude {
		excRe[i], err = regexp.Compile("^" + str + "$")
		if err != nil {
			return fmt.Errorf("failed to parse exclude regexp: %q, %w", str, err)
		}
	}
	// list all tags in source repo
	srcRef, err := ref.New(args[0])
	if err != nil {
		return err
	}
	tgtRef, err := ref.New(args[1])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	tagList, err := rc.TagList(ctx, srcRef)
	if err != nil {
		return fmt.Errorf("failed to list tags in %s: %w", args[0], err)
	}
	// filter include/exclude regexp
	tags := []string{}
	for _, tag := range tagList.Tags {
		if len(incRe) > 0 {
			included := false
			for _, re := range incRe {
				if re.MatchString(tag) {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}
		excluded := false
		for _, re := range excRe {
			if re.MatchString(tag) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		tags = append(tags, tag)
	}
	// if "new-tags", list all tags in target repo and filter out all existing tags
	if opts.newTags {
		tgtTags, err := rc.TagList(ctx, tgtRef)
		if err != nil && !errors.Is(err, errs.ErrNotFound) {
			return fmt.Errorf("failed to list tags in %s: %w", args[1], err)
		}
		if tgtTags != nil && len(tgtTags.Tags) > 0 {
			newTags := []string{}
			for _, tag := range tags {
				if !slices.Contains(tgtTags.Tags, tag) {
					newTags = append(newTags, tag)
				}
			}
			tags = newTags
		}
	}
	// use a channel to throttle requests
	if opts.concurrent <= 0 {
		// no throttle
		opts.concurrent = len(tags)
	}
	throttle := make(chan struct{}, opts.concurrent)
	errList := []error{}
	mu := sync.Mutex{}
	// iterate over each tag, running image copy in goroutine
	rcOpts := []regclient.ImageOpts{}
	if opts.referrers {
		rcOpts = append(rcOpts, regclient.ImageWithReferrers())
	}
	for _, tag := range tags {
		mu.Lock()
		foundErr := len(errList) > 0
		mu.Unlock()
		if foundErr {
			break
		}
		srcTag := srcRef.SetTag(tag)
		tgtTag := tgtRef.SetTag(tag)
		throttle <- struct{}{}
		go func() {
			err := rc.ImageCopy(ctx, srcTag, tgtTag, rcOpts...)
			if err != nil {
				mu.Lock()
				if !errors.Is(err, context.Canceled) || len(errList) == 0 {
					errList = append(errList, err)
					// cancel other copies on any errors
					cancel()
				}
				mu.Unlock()
			}
			<-throttle
		}()
	}
	// wait for all copies to finish
	for range opts.concurrent {
		throttle <- struct{}{}
	}
	if len(errList) == 1 {
		return errList[0]
	}
	return errors.Join(errList...)
	// TODO: include tty progress
}

func (opts *repoOpts) runRepoLs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	host := args[0]
	// TODO: use regex to validate hostname + port
	i := strings.IndexRune(host, '/')
	if i >= 0 {
		opts.rootOpts.log.Error("Hostname invalid",
			slog.String("host", host))
		return ErrInvalidInput
	}
	rc := opts.rootOpts.newRegClient()
	opts.rootOpts.log.Debug("Listing repositories",
		slog.String("host", host),
		slog.String("last", opts.last),
		slog.Int("limit", opts.limit))
	sOpts := []scheme.RepoOpts{}
	if opts.last != "" {
		sOpts = append(sOpts, scheme.WithRepoLast(opts.last))
	}
	if opts.limit != 0 {
		sOpts = append(sOpts, scheme.WithRepoLimit(opts.limit))
	}
	rl, err := rc.RepoList(ctx, host, sOpts...)
	if err != nil {
		return err
	}
	switch opts.format {
	case "raw":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		opts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, rl)
}
