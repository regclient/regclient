package cli

import (
	"github.com/spf13/cobra"
)

var layerCmd = &cobra.Command{
	Use:   "layer",
	Short: "manage image layers/blobs",
}
var layerPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "download a layer/blob",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runLayerPull,
}

func init() {
	layerCmd.AddCommand(layerPullCmd)
	rootCmd.AddCommand(layerCmd)
}

func runLayerPull(cmd *cobra.Command, args []string) error {
	return nil
}
