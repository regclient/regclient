package main

import (
	"fmt"
	"io"
	"os"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"
)

// TODO: identify a more appropriate location for this command, leave it hidden until then
var digestCmd = &cobra.Command{
	Hidden: true,
	Use:    "digest",
	Short:  "compute digest on stdin",
	Args:   cobra.RangeArgs(0, 0),
	RunE:   runDigest,
}

func init() {
	rootCmd.AddCommand(digestCmd)
}

func runDigest(cmd *cobra.Command, args []string) error {
	digester := digest.Canonical.Digester()

	_, err := io.Copy(digester.Hash(), os.Stdin)

	if err != nil {
		return err
	}

	fmt.Println(digester.Digest().String())
	return nil
}
