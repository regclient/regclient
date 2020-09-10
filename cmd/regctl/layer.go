package main

import (
	"context"
	"io"
	"os"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var layerCmd = &cobra.Command{
	Use:   "layer <cmd>",
	Short: "manage image layers/blobs",
}
var layerPullCmd = &cobra.Command{
	Use:   "pull <repository> <digest>",
	Short: "download a layer/blob",
	Long: `Download a blob from the registry. The output is the blob itself which may
be a compressed tar file, a json config, or any other blob supported by the
registry. The layer or blob digest can be found in the image manifest.`,
	Args: cobra.RangeArgs(2, 2),
	RunE: runLayerPull,
}

func init() {
	layerCmd.AddCommand(layerPullCmd)
	rootCmd.AddCommand(layerCmd)
}

func runLayerPull(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host":       ref.Registry,
		"repository": ref.Repository,
		"digest":     args[1],
	}).Debug("Pulling layer")
	blobIO, resp, err := rc.BlobGet(context.Background(), ref, args[1], []string{})

	_ = resp
	_, err = io.Copy(os.Stdout, blobIO)
	if err != nil {
		return err
	}

	return nil
}
