package main

import (
	"fmt"
	"io"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/spf13/cobra"
)

type digestCmd struct {
	rootOpts   *rootCmd
	decompress bool
}

func NewDigestCmd(rootOpts *rootCmd) *cobra.Command {
	digestOpts := digestCmd{
		rootOpts: rootOpts,
	}
	// TODO: identify a more appropriate location for this command, leave it hidden until then
	var digestCmd = &cobra.Command{
		Hidden: true,
		Use:    "digest",
		Short:  "compute digest on stdin",
		Args:   cobra.RangeArgs(0, 0),
		RunE:   digestOpts.runDigest,
	}
	digestCmd.Flags().BoolVarP(&digestOpts.decompress, "decompress", "", false, "Decompress the input if compressed")

	return digestCmd
}

func (digestOpts *digestCmd) runDigest(cmd *cobra.Command, args []string) error {
	var reader io.Reader = cmd.InOrStdin()

	if digestOpts.decompress {
		nreader, err := archive.Decompress(reader)
		if err != nil {
			return err
		}
		reader = nreader
	}

	digester := digest.Canonical.Digester()

	_, err := io.Copy(digester.Hash(), reader)

	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), digester.Digest().String())
	return nil
}
