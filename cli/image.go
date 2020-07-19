package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sudo-bmitch/regcli/regclient"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "manage images",
}
var imageCopyCmd = &cobra.Command{
	Use:   "copy",
	Short: "copy images",
	Args:  cobra.RangeArgs(2, 2),
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
	return ErrNotImplemented
}

func runImageDelete(cmd *cobra.Command, args []string) error {
	return ErrNotImplemented
}

func runImageInspect(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := regclient.NewRegClient(regclient.WithDockerCreds())
	img, err := rc.ImageInspect(context.Background(), ref)
	if err != nil {
		return err
	}
	imgJSON, err := json.MarshalIndent(img, "", "  ")
	fmt.Println(string(imgJSON))
	return nil
}

func runImageRetag(cmd *cobra.Command, args []string) error {
	return ErrNotImplemented
}
