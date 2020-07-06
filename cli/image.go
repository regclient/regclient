package cli

import (
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "manage images",
}
var imageCopyCmd = &cobra.Command{
	Use:   "copy",
	Short: "copy images",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageCopy,
}
var imageDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "delete images",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageDelete,
}
var imageInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "inspect images",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageInspect,
}
var imageRetagCmd = &cobra.Command{
	Use:   "retag",
	Short: "retag images",
	Args:  cobra.RangeArgs(2, 2),
	RunE:  runImageRetag,
}

func init() {
	imageCmd.AddCommand(imageCopyCmd)
	imageCmd.AddCommand(imageDeleteCmd)
	imageCmd.AddCommand(imageInspectCmd)
	imageCmd.AddCommand(imageRetagCmd)
	rootCmd.AddCommand(imageCmd)
}

func runImageCopy(cmd *cobra.Command, args []string) error {
	return nil
}

func runImageDelete(cmd *cobra.Command, args []string) error {
	return nil
}

func runImageInspect(cmd *cobra.Command, args []string) error {
	return nil
}

func runImageRetag(cmd *cobra.Command, args []string) error {
	return nil
}
