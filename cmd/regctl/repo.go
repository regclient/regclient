package main

import (
	"strings"

	"github.com/sirupsen/logrus"
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
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: registryArgListReg,
		RunE:              repoOpts.runRepoLs,
	}

	repoLsCmd.Flags().StringVarP(&repoOpts.last, "last", "", "", "Specify the last repo from a previous request for pagination")
	repoLsCmd.Flags().IntVarP(&repoOpts.limit, "limit", "", 0, "Specify the number of repos to retrieve")
	repoLsCmd.Flags().StringVarP(&repoOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = repoLsCmd.RegisterFlagCompletionFunc("last", completeArgNone)
	_ = repoLsCmd.RegisterFlagCompletionFunc("limit", completeArgNone)
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
		log.WithFields(logrus.Fields{
			"host": host,
		}).Error("Hostname invalid")
		return ErrInvalidInput
	}
	rc := repoOpts.rootOpts.newRegClient()
	log.WithFields(logrus.Fields{
		"host":  host,
		"last":  repoOpts.last,
		"limit": repoOpts.limit,
	}).Debug("Listing repositories")
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
