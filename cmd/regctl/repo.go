package main

import (
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
)

type repoOpts struct {
	rootOpts *rootOpts
	last     string
	limit    int
	format   string
}

func NewRepoCmd(rOpts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <cmd>",
		Short: "manage repositories",
	}
	cmd.AddCommand(newRepoLsCmd(rOpts))
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
