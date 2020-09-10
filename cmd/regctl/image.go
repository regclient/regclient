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
	Use:   "image <cmd>",
	Short: "manage images",
}
var imageCopyCmd = &cobra.Command{
	Use:   "copy <src_image_ref> <dst_image_ref>",
	Short: "copy or retag image",
	Long: `Copy or retag an image. This works between registries and only pulls layers
that do not exist at the target. In the same registry it attempts to mount
the layers between repositories. And within the same repository it only
sends the manifest with the new tag.`,
	Args: cobra.RangeArgs(2, 2),
	RunE: runImageCopy,
}
var imageDeleteCmd = &cobra.Command{
	Use:   "delete <image_ref>",
	Short: "delete image",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageDelete,
}
var imageDigestCmd = &cobra.Command{
	Use:   "digest <image_ref>",
	Short: "show digest for pinning",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageDigest,
}
var imageExportCmd = &cobra.Command{
	Use:   "export <image_ref>",
	Short: "export image",
	Long: `Exports an image into a tar file that can be later loaded into a docker
engine with "docker load". The tar file is output to stdout by default.
Example usage: regctl image export registry:5000/yourimg:v1 >yourimg-v1.tar`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runImageExport,
}
var imageImportCmd = &cobra.Command{
	Use:   "import <image_ref>",
	Short: "import image",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageImport,
}
var imageInspectCmd = &cobra.Command{
	Use:   "inspect <image_ref>",
	Short: "inspect image",
	Long: `Shows the config json for an image and is equivalent to pulling the image
in docker, and inspecting it, but without pulling any of the image layers.`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runImageInspect,
}
var imageManifestCmd = &cobra.Command{
	Use:   "manifest <image_ref>",
	Short: "show manifest or manifest list",
	Long: `Shows the manifest or manifest list of the specified image. A single manifest
from a manifest list can be displayed by using the digest. Examples:
regctl image manifest ubuntu:latest
regctl image manifest ubuntu@sha256:6f2fb2f9fb5582f8b587837afd6ea8f37d8d1d9e41168c90f410a6ef15fa8ce5`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runImageManifest,
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
