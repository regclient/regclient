package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
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
var imageExportCmd = &cobra.Command{
	Use:   "export",
	Short: "export images",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageExport,
}
var imageImportCmd = &cobra.Command{
	Use:   "import",
	Short: "import images",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageImport,
}
var imageInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "inspect images",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageInspect,
}
var imageManifestCmd = &cobra.Command{
	Use:   "manifest",
	Short: "show manifest",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageManifest,
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
	imageCmd.AddCommand(imageExportCmd)
	imageCmd.AddCommand(imageImportCmd)
	imageCmd.AddCommand(imageInspectCmd)
	imageCmd.AddCommand(imageManifestCmd)
	imageCmd.AddCommand(imageRetagCmd)
	rootCmd.AddCommand(imageCmd)
}

func runImageCopy(cmd *cobra.Command, args []string) error {
	refSrc, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	refTgt, err := regclient.NewRef(args[1])
	if err != nil {
		return err
	}
	rc := regclient.NewRegClient(regclient.WithDockerCreds())
	return rc.ImageCopy(context.Background(), refSrc, refTgt)
}

func runImageDelete(cmd *cobra.Command, args []string) error {
	return ErrNotImplemented
}

func runImageExport(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := regclient.NewRegClient(regclient.WithDockerCreds())
	return rc.ImageExport(context.Background(), ref, os.Stdout)
}

func runImageImport(cmd *cobra.Command, args []string) error {
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

func runImageManifest(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := regclient.NewRegClient(regclient.WithDockerCreds())

	m, err := rc.ManifestGet(context.Background(), ref)
	if err != nil {
		return err
	}
	mt := m.GetMediaType()
	var mj []byte
	switch mt {
	case regclient.MediaTypeDocker2Manifest:
		mj, err = json.MarshalIndent(m.GetDocker(), "", "  ")
	case ociv1.MediaTypeImageManifest:
		mj, err = json.MarshalIndent(m.GetOCI(), "", "  ")
	case regclient.MediaTypeDocker2ManifestList:
		// TODO
		return fmt.Errorf("Unsupported manifest media type %s", mt)
	case ociv1.MediaTypeImageIndex:
		mj, err = json.MarshalIndent(m.GetOCIIndex(), "", "  ")
	default:
		return fmt.Errorf("Unknown manifest media type %s", mt)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(mj))
	return nil
}

func runImageRetag(cmd *cobra.Command, args []string) error {
	return ErrNotImplemented
}
