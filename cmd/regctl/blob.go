package main

import (
	"context"
	"io"
	"os"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var blobCmd = &cobra.Command{
	Use:     "blob <cmd>",
	Aliases: []string{"layer"},
	Short:   "manage image blobs/layers",
}
var blobGetCmd = &cobra.Command{
	Use:     "get <repository> <digest>",
	Aliases: []string{"pull"},
	Short:   "download a blob/layer",
	Long: `Download a blob from the registry. The output is the blob itself which may
be a compressed tar file, a json config, or any other blob supported by the
registry. The blob or layer digest can be found in the image manifest.`,
	Args: cobra.RangeArgs(2, 2),
	RunE: runBlobGet,
}

func init() {
	blobCmd.AddCommand(blobGetCmd)
	rootCmd.AddCommand(blobCmd)
}

func runBlobGet(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host":       ref.Registry,
		"repository": ref.Repository,
		"digest":     args[1],
	}).Debug("Pulling blob")
	blobIO, resp, err := rc.BlobGet(context.Background(), ref, args[1], []string{})
	if err != nil {
		return err
	}

	_ = resp
	_, err = io.Copy(os.Stdout, blobIO)
	if err != nil {
		return err
	}

	return nil
}
