package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
var imageDigestCmd = &cobra.Command{
	Use:   "digest",
	Short: "show digest for pinning",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageDigest,
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

func init() {
	imageCmd.AddCommand(imageCopyCmd)
	imageCmd.AddCommand(imageDeleteCmd)
	imageCmd.AddCommand(imageDigestCmd)
	imageCmd.AddCommand(imageExportCmd)
	imageCmd.AddCommand(imageImportCmd)
	imageCmd.AddCommand(imageInspectCmd)
	imageCmd.AddCommand(imageManifestCmd)
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
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"source host": refSrc.Registry,
		"source repo": refSrc.Repository,
		"source tag":  refSrc.Tag,
		"target host": refTgt.Registry,
		"target repo": refTgt.Repository,
		"target tag":  refTgt.Tag,
	}).Debug("Image copy")
	return rc.ImageCopy(context.Background(), refSrc, refTgt)
}

func runImageDelete(cmd *cobra.Command, args []string) error {
	return ErrNotImplemented
}

func runImageDigest(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image digest")
	d, err := rc.ManifestDigest(context.Background(), ref)
	if err != nil {
		return err
	}
	fmt.Println(d.String())
	return nil
}

func runImageExport(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image export")
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
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image inspect")
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
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image manifest")
	m, err := rc.ManifestGet(context.Background(), ref)
	if err != nil {
		return err
	}
	mj, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(mj))
	return nil
}
