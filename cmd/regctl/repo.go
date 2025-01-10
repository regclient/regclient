package main

import (
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
)

type repoCmd struct {
	rootOpts *rootCmd
	last     string
	limit    int
	format   string
}

func NewRepoCmd(rootOpts *rootCmd) *cobra.Command {
	repoOpts := repoCmd{
		rootOpts: rootOpts,
	}
	var repoTopCmd = &cobra.Command{
		Use:   "repo <cmd>",
		Short: "manage repositories",
	}
	var repoLsCmd = &cobra.Command{
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
		RunE:              repoOpts.runRepoLs,
	}

	repoLsCmd.Flags().StringVarP(&repoOpts.last, "last", "", "", "Specify the last repo from a previous request for pagination")
	_ = repoLsCmd.RegisterFlagCompletionFunc("last", completeArgNone)
	repoLsCmd.Flags().IntVarP(&repoOpts.limit, "limit", "", 0, "Specify the number of repos to retrieve")
	_ = repoLsCmd.RegisterFlagCompletionFunc("limit", completeArgNone)
	repoLsCmd.Flags().StringVarP(&repoOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = repoLsCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	repoTopCmd.AddCommand(repoLsCmd)
	return repoTopCmd
}

func (repoOpts *repoCmd) runRepoLs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	host := args[0]
	// TODO: use regex to validate hostname + port
	i := strings.IndexRune(host, '/')
	if i >= 0 {
		repoOpts.rootOpts.log.Error("Hostname invalid",
			slog.String("host", host))
		return ErrInvalidInput
	}
	rc := repoOpts.rootOpts.newRegClient()
	repoOpts.rootOpts.log.Debug("Listing repositories",
		slog.String("host", host),
		slog.String("last", repoOpts.last),
		slog.Int("limit", repoOpts.limit))
	opts := []scheme.RepoOpts{}
	if repoOpts.last != "" {
		opts = append(opts, scheme.WithRepoLast(repoOpts.last))
	}
	if repoOpts.limit != 0 {
		opts = append(opts, scheme.WithRepoLimit(repoOpts.limit))
	}
	rl, err := rc.RepoList(ctx, host, opts...)
	if err != nil {
		return err
	}
	switch repoOpts.format {
	case "raw":
		repoOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		repoOpts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		repoOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), repoOpts.format, rl)
}
