package cli

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/sudo-bmitch/regcli/regclient"
)

var layerCmd = &cobra.Command{
	Use:   "layer",
	Short: "manage image layers/blobs",
}
var layerPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "download a layer/blob",
	Args:  cobra.RangeArgs(2, 2),
	RunE:  runLayerPull,
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
	rc := regclient.NewRegClient(regclient.WithDockerCreds())

	// try retrieving a manifest list
	blobIO, resp, err := rc.BlobGet(context.Background(), ref, args[1], []string{})

	_ = resp
	_, err = io.Copy(os.Stdout, blobIO)
	if err != nil {
		return err
	}

	return nil
}
