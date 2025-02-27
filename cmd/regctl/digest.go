package main

import (
	"fmt"
	"io"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
)

type digestOpts struct {
	rootOpts *rootOpts
	algo     string
	format   string
}

func NewDigestCmd(rOpts *rootOpts) *cobra.Command {
	opts := digestOpts{
		rootOpts: rOpts,
	}
	// TODO(bmitch): consider if this should be moved out of hidden/experimental
	cmd := &cobra.Command{
		Hidden: true,
		Use:    "digest",
		Short:  "compute digest on stdin",
		Long: `Output the digest from content provided on stdin.
This command is EXPERIMENTAL and could be removed in the future.`,
		Example: `
# compute the digest of hello world
echo hello world | regctl digest`,
		Args: cobra.RangeArgs(0, 0),
		RunE: opts.runDigest,
	}
	cmd.Flags().StringVar(&opts.algo, "algorithm", "sha256", "Digest algorithm")
	_ = cmd.RegisterFlagCompletionFunc("algorithm", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"sha256", "sha512"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringVar(&opts.format, "format", "{{.String}}", "Go template to output the digest result")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)

	return cmd
}

func (opts *digestOpts) runDigest(cmd *cobra.Command, args []string) error {
	algo := digest.Algorithm(opts.algo)
	if !algo.Available() {
		return fmt.Errorf("digest algorithm %s is not available", opts.algo)
	}
	digester := algo.Digester()

	_, err := io.Copy(digester.Hash(), cmd.InOrStdin())
	if err != nil {
		return err
	}

	return template.Writer(cmd.OutOrStdout(), opts.format, digester.Digest())
}
