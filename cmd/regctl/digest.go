package main

import (
	"io"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
)

type digestCmd struct {
	rootOpts *rootCmd
	format   string
}

func NewDigestCmd(rootOpts *rootCmd) *cobra.Command {
	digestOpts := digestCmd{
		rootOpts: rootOpts,
	}
	// TODO(bmitch): consider if this should be moved out of hidden/experimental
	var digestCmd = &cobra.Command{
		Hidden: true,
		Use:    "digest",
		Short:  "compute digest on stdin",
		Long: `Output the digest from content provided on stdin.
This command is EXPERIMENTAL and could be removed in the future.`,
		Example: `
# compute the digest of hello world
echo hello world | regctl digest`,
		Args: cobra.RangeArgs(0, 0),
		RunE: digestOpts.runDigest,
	}

	digestCmd.Flags().StringVar(&digestOpts.format, "format", "{{.String}}", "Go template to output the digest result")

	return digestCmd
}

func (digestOpts *digestCmd) runDigest(cmd *cobra.Command, args []string) error {
	digester := digest.Canonical.Digester()

	_, err := io.Copy(digester.Hash(), cmd.InOrStdin())
	if err != nil {
		return err
	}

	return template.Writer(cmd.OutOrStdout(), digestOpts.format, digester.Digest())
}
