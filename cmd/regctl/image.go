package main

import (
	"io"
	"os"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/ref"
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
	forceRecursive bool
	format         string
	digestTags     bool
	list           bool
	platform       string
	platforms      []string
	requireList    bool
}

func init() {
	imageCopyCmd.Flags().BoolVarP(&imageOpts.forceRecursive, "force-recursive", "", false, "Force recursive copy of image, repairs missing nested blobs and manifests")
	imageCopyCmd.Flags().StringArrayVarP(&imageOpts.platforms, "platforms", "", []string{}, "Copy only specific platforms, registry validation must be disabled")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.digestTags, "digest-tags", "", false, "Include digest tags (\"sha256-<digest>.*\") when copying manifests")
	// platforms should be treated as experimental since it will break many registries
	imageCopyCmd.Flags().MarkHidden("platforms")

	imageDeleteCmd.Flags().BoolVarP(&manifestOpts.forceTagDeref, "force-tag-dereference", "", false, "Dereference the a tag to a digest, this is unsafe")

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
	ctx := cmd.Context()
	rSrc, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rTgt, err := ref.New(args[1])
	if err != nil {
		return err
	}
	rc := newRegClient()
	defer rc.Close(ctx, rSrc)
	defer rc.Close(ctx, rTgt)

	log.WithFields(logrus.Fields{
		"source host": rSrc.Registry,
		"source repo": rSrc.Repository,
		"source tag":  rSrc.Tag,
		"target host": rTgt.Registry,
		"target repo": rTgt.Repository,
		"target tag":  rTgt.Tag,
		"recursive":   imageOpts.forceRecursive,
		"digest-tags": imageOpts.digestTags,
	}).Debug("Image copy")
	opts := []regclient.ImageOpts{}
	if imageOpts.forceRecursive {
		opts = append(opts, regclient.ImageWithForceRecursive())
	}
	if imageOpts.digestTags {
		opts = append(opts, regclient.ImageWithDigestTags())
	}
	if len(imageOpts.platforms) > 0 {
		opts = append(opts, regclient.ImageWithPlatforms(imageOpts.platforms))
	}
	return rc.ImageCopy(ctx, rSrc, rTgt, opts...)
}

func runImageExport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
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
	defer rc.Close(ctx, r)
	log.WithFields(logrus.Fields{
		"ref": r.CommonName(),
	}).Debug("Image export")
	return rc.ImageExport(ctx, r, w)
}

func runImageImport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rs, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer rs.Close()
	rc := newRegClient()
	defer rc.Close(ctx, r)
	log.WithFields(logrus.Fields{
		"ref":  r.CommonName(),
		"file": args[1],
	}).Debug("Image import")

	return rc.ImageImport(ctx, r, rs)
}

func runImageInspect(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"host":     r.Registry,
		"repo":     r.Repository,
		"tag":      r.Tag,
		"platform": imageOpts.platform,
	}).Debug("Image inspect")

	manifestOpts.platform = imageOpts.platform
	m, err := getManifest(rc, r)
	if err != nil {
		return err
	}
	cd, err := m.GetConfigDigest()
	if err != nil {
		return err
	}

	blobConfig, err := rc.BlobGetOCIConfig(ctx, r, cd)
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
	return template.Writer(os.Stdout, imageOpts.format, blobConfig)
}

func runImageRateLimit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host": r.Registry,
		"repo": r.Repository,
		"tag":  r.Tag,
	}).Debug("Image rate limit")

	// request only the headers, avoids adding to Docker Hub rate limits
	m, err := rc.ManifestHead(ctx, r)
	if err != nil {
		return err
	}

	return template.Writer(os.Stdout, imageOpts.format, m.GetRateLimit())
}
