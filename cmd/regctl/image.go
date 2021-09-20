package main

import (
	"context"
	"io"
	"os"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:   "image <cmd>",
	Short: "manage images",
}
var imageCopyCmd = &cobra.Command{
	Use:     "copy <src_image_ref> <dst_image_ref>",
	Aliases: []string{"cp"},
	Short:   "copy or retag image",
	Long: `Copy or retag an image. This works between registries and only pulls layers
that do not exist at the target. In the same registry it attempts to mount
the layers between repositories. And within the same repository it only
sends the manifest with the new tag.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeArgTag,
	RunE:              runImageCopy,
}
var imageDeleteCmd = &cobra.Command{
	Use:     "delete <image_ref>",
	Aliases: []string{"del", "rm", "remove"},
	Short:   "delete image, same as \"manifest delete\"",
	Long: `Delete a manifest. This will delete the manifest, and all tags pointing to that
manifest. You must specify a digest, not a tag on this command (e.g. 
image_name@sha256:1234abc...). It is up to the registry whether the delete
API is supported. Additionally, registries may garbage collect the filesystem
layers (blobs) separately or not at all. See also the "tag delete" command.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete digests
	RunE:      runManifestDelete,
}
var imageDigestCmd = &cobra.Command{
	Use:               "digest <image_ref>",
	Short:             "show digest for pinning, same as \"manifest digest\"",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runManifestDigest,
}
var imageExportCmd = &cobra.Command{
	Use:   "export <image_ref> [filename]",
	Short: "export image",
	Long: `Exports an image into a tar file that can be later loaded into a docker
engine with "docker load". The tar file is output to stdout by default.
Example usage: regctl image export registry:5000/yourimg:v1 >yourimg-v1.tar`,
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: completeArgTag,
	RunE:              runImageExport,
}
var imageImportCmd = &cobra.Command{
	Use:   "import <image_ref> <filename>",
	Short: "import image",
	Long: `Imports an image from a tar file. This must be either a docker formatted tar
from "docker save" or an OCI Layout compatible tar. The output from
"regctl image export" can be used. Stdin is not permitted for the tar file.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeArgList([]completeFunc{completeArgTag, completeArgDefault}),
	RunE:              runImageImport,
}
var imageInspectCmd = &cobra.Command{
	Use:     "inspect <image_ref>",
	Aliases: []string{"config"},
	Short:   "inspect image",
	Long: `Shows the config json for an image and is equivalent to pulling the image
in docker, and inspecting it, but without pulling any of the image layers.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runImageInspect,
}
var imageManifestCmd = &cobra.Command{
	Use:               "manifest <image_ref>",
	Short:             "show manifest or manifest list, same as \"manifest get\"",
	Long:              `Shows the manifest or manifest list of the specified image.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runManifestGet,
}
var imageRateLimitCmd = &cobra.Command{
	Use:   "ratelimit <image_ref>",
	Short: "show the current rate limit",
	Long: `Shows the rate limit using an http head request against the image manifest.
If Set is false, the Remain value was not provided.
The other values may be 0 if not provided by the registry.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runImageRateLimit,
}

var imageOpts struct {
	list        bool
	platform    string
	requireList bool
	format      string
}

func init() {
	imageDigestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", false, "Do not resolve platform from manifest list (recommended)")
	imageDigestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageDigestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	imageDigestCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)

	imageInspectCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageInspectCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	imageInspectCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	imageInspectCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageManifestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", false, "Output manifest list if available")
	imageManifestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageManifestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	imageManifestCmd.Flags().StringVarP(&manifestOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	imageManifestCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	imageManifestCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageRateLimitCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	imageRateLimitCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageCmd.AddCommand(imageCopyCmd)
	imageCmd.AddCommand(imageDeleteCmd)
	imageCmd.AddCommand(imageDigestCmd)
	imageCmd.AddCommand(imageExportCmd)
	imageCmd.AddCommand(imageImportCmd)
	imageCmd.AddCommand(imageInspectCmd)
	imageCmd.AddCommand(imageManifestCmd)
	imageCmd.AddCommand(imageRateLimitCmd)
	rootCmd.AddCommand(imageCmd)
}

func runImageCopy(cmd *cobra.Command, args []string) error {
	refSrc, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	refTgt, err := types.NewRef(args[1])
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

func runImageExport(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	var w io.Writer
	if len(args) == 2 {
		w, err = os.Create(args[1])
		if err != nil {
			return err
		}
	} else {
		w = os.Stdout
	}
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"ref": ref.CommonName(),
	}).Debug("Image export")
	return rc.ImageExport(context.Background(), ref, w)
}

func runImageImport(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rs, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer rs.Close()
	rc := newRegClient()
	log.WithFields(logrus.Fields{
		"ref":  ref.CommonName(),
		"file": args[1],
	}).Debug("Image import")

	return rc.ImageImport(context.Background(), ref, rs)
}

func runImageInspect(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host":     ref.Registry,
		"repo":     ref.Repository,
		"tag":      ref.Tag,
		"platform": imageOpts.platform,
	}).Debug("Image inspect")

	m, err := getManifest(rc, ref)
	if err != nil {
		return err
	}
	cd, err := m.GetConfigDigest()
	if err != nil {
		return err
	}

	blobConfig, err := rc.BlobGetOCIConfig(context.Background(), ref, cd)
	if err != nil {
		return err
	}
	switch imageOpts.format {
	case "raw":
		imageOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		imageOpts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		imageOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(os.Stdout, imageOpts.format, blobConfig, template.WithFuncs(regclient.TemplateFuncs))
}

func runImageRateLimit(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image rate limit")

	// request only the headers, avoids adding to Docker Hub rate limits
	m, err := rc.ManifestHead(context.Background(), ref)
	if err != nil {
		return err
	}

	return template.Writer(os.Stdout, imageOpts.format, m.GetRateLimit(), template.WithFuncs(regclient.TemplateFuncs))
}
