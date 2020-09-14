package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/containerd/platforms"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
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
	Long:  `Shows the manifest or manifest list of the specified image.`,
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runImageManifest,
}

var imageOpts struct {
	list        bool
	platform    string
	requireList bool
}

func init() {
	imageInspectCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{jsonPretty .}}", "Format output with go template syntax")

	imageManifestCmd.Flags().BoolVarP(&imageOpts.list, "list", "", false, "Output manifest list if available")
	imageManifestCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageManifestCmd.Flags().BoolVarP(&imageOpts.requireList, "require-list", "", false, "Fail is manifest list is not received")
	imageManifestCmd.Flags().StringVarP(&rootOpts.format, "format", "", "{{jsonPretty .}}", "Format output with go template syntax")

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
	return templateRun(os.Stdout, rootOpts.format, img)
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

	// add warning if not list and list required or platform requested
	if !m.IsList() && imageOpts.requireList {
		log.Warn("Manifest list unavailable")
		return ErrNotFound
	}
	if !m.IsList() && imageOpts.platform != "" {
		log.Warn("Manifest list unavailable, ignoring platform flag")
	}

	// retrieve the specified platform from the manifest list
	if m.IsList() && !imageOpts.list && !imageOpts.requireList {
		var plat ociv1.Platform
		if imageOpts.platform != "" {
			plat, err = platforms.Parse(imageOpts.platform)
			if err != nil {
				log.WithFields(logrus.Fields{
					"platform": imageOpts.platform,
					"err":      err,
				}).Warn("Could not parse platform")
			}
		}
		if plat.OS == "" {
			plat = platforms.DefaultSpec()
		}
		desc, err := m.GetPlatformDesc(&plat)
		if err != nil {
			pl, _ := m.GetPlatformList()
			var ps []string
			for _, p := range pl {
				ps = append(ps, platforms.Format(*p))
			}
			log.WithFields(logrus.Fields{
				"platform":  platforms.Format(plat),
				"err":       err,
				"platforms": strings.Join(ps, ", "),
			}).Warn("Platform could not be found in manifest list")
			return ErrNotFound
		} else {
			log.WithFields(logrus.Fields{
				"platform": platforms.Format(plat),
				"digest":   desc.Digest.String(),
			}).Debug("Found platform specific digest in manifest list")
			ref.Digest = desc.Digest.String()
			m, err = rc.ManifestGet(context.Background(), ref)
			if err != nil {
				log.WithFields(logrus.Fields{
					"err":      err,
					"digest":   ref.Digest,
					"platform": platforms.Format(plat),
				}).Warn("Could not get platform specific manifest")
				return err
			}
		}
	}

	return templateRun(os.Stdout, rootOpts.format, m.GetOrigManifest())
}
