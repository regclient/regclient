package main

import (
	"fmt"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/ref"
)

type refOpts struct {
	rootOpts *rootOpts
	format   string
}

func NewRefCmd(rOpts *rootOpts) *cobra.Command {
	opts := refOpts{
		rootOpts: rOpts,
	}
	// TODO(bmitch): consider if this should be moved out of hidden/experimental
	cmd := &cobra.Command{
		Hidden: true,
		Use:    "ref",
		Short:  "parse an image ref",
		Long: `Parse an image reference so that it may be output with formatting.
This command is EXPERIMENTAL and could be removed in the future.`,
		Example: `
# extract the registry (docker.io)
regctl ref nginx --format '{{ .Registry }}'
`,
		Args: cobra.ExactArgs(1),
		RunE: opts.runRef,
	}
	cmd.Flags().StringVar(&opts.format, "format", "{{.CommonName}}", "Format the output using a Go template")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)

	return cmd
}

func (opts *refOpts) runRef(cmd *cobra.Command, args []string) error {
	r, err := ref.New(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", args[0], err)
	}

	return template.Writer(cmd.OutOrStdout(), opts.format, r)
}
