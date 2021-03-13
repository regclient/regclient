package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/containerd/platforms"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
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
	Args: cobra.RangeArgs(2, 2),
	RunE: runImageCopy,
}
var imageDeleteCmd = &cobra.Command{
	Use:     "delete <image_ref>",
	Aliases: []string{"del", "rm", "remove"},
	Short:   "delete image",
	Args:    cobra.RangeArgs(1, 1),
	RunE:    runImageDelete,
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
var imageRateLimitCmd = &cobra.Command{
	Use:   "ratelimit <image_ref>",
	Short: "show the current rate limit",
	Long: `Shows the rate limit using an http head request against the image manifest.
If Set is false, the Remain value was not provided.
The other values may be 0 if not provided by the registry.`,
	Args: cobra.RangeArgs(1, 1),
	RunE: runImageRateLimit,
}

var imageOpts struct {
	list        bool
	platform    string
	requireList bool
	format      string
	raw         bool
	rawBody     bool
	rawHeader   bool
}

func init() {
	imageDigestCmd.Flags().BoolVarP(&imageOpts.list, "list", "", false, "Do not resolve platform from manifest list (recommended)")
	imageDigestCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageDigestCmd.Flags().BoolVarP(&imageOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")

	imageInspectCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageInspectCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	imageManifestCmd.Flags().BoolVarP(&imageOpts.list, "list", "", false, "Output manifest list if available")
	imageManifestCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	imageManifestCmd.Flags().BoolVarP(&imageOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	imageManifestCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	imageManifestCmd.Flags().BoolVarP(&imageOpts.raw, "raw", "", false, "Show raw response (overrides format)")
	imageManifestCmd.Flags().BoolVarP(&imageOpts.rawBody, "raw-body", "", false, "Show raw body (overrides format)")
	imageManifestCmd.Flags().BoolVarP(&imageOpts.rawHeader, "raw-header", "", false, "Show raw headers (overrides format)")

	imageRateLimitCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

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

func getManifest(rc regclient.RegClient, ref regclient.Ref) (regclient.Manifest, error) {
	m, err := rc.ManifestGet(context.Background(), ref)
	if err != nil {
		return m, err
	}

	// add warning if not list and list required or platform requested
	if !m.IsList() && imageOpts.requireList {
		log.Warn("Manifest list unavailable")
		return m, ErrNotFound
	}
	if !m.IsList() && imageOpts.platform != "" {
		log.Info("Manifest list unavailable, ignoring platform flag")
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
			return m, ErrNotFound
		}
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
			return m, err
		}
	}
	return m, nil
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

	err = rc.ManifestDelete(context.Background(), ref)
	if err != nil {
		return err
	}
	return nil
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

	// attempt to request only the headers, avoids Docker Hub rate limits
	m, err := rc.ManifestHead(context.Background(), ref)
	if err != nil {
		return err
	}

	// add warning if not list and list required or platform requested
	if !m.IsList() && imageOpts.requireList {
		log.Warn("Manifest list unavailable")
		return ErrNotFound
	}
	if !m.IsList() && imageOpts.platform != "" {
		log.Info("Manifest list unavailable, ignoring platform flag")
	}

	// if a manifest list was received and we need the platform specific
	// manifest, run the http GET calls
	if m.IsList() && !imageOpts.list && !imageOpts.requireList {
		m, err = getManifest(rc, ref)
		if err != nil {
			return err
		}
	}

	fmt.Println(m.GetDigest().String())
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

	img, err := rc.ImageGetConfig(context.Background(), ref, cd.String())
	if err != nil {
		return err
	}
	return template.Writer(os.Stdout, imageOpts.format, img, template.WithFuncs(regclient.TemplateFuncs))
}

func runImageManifest(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	m, err := getManifest(rc, ref)
	if err != nil {
		return err
	}

	if imageOpts.raw {
		imageOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	} else if imageOpts.rawBody {
		imageOpts.format = "{{printf \"%s\" .RawBody}}"
	} else if imageOpts.rawHeader {
		imageOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(os.Stdout, imageOpts.format, m, template.WithFuncs(regclient.TemplateFuncs))
}

func runImageRateLimit(cmd *cobra.Command, args []string) error {
	ref, err := regclient.NewRef(args[0])
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
