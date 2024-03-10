package main

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/internal/ascii"
	"github.com/regclient/regclient/internal/units"
	"github.com/regclient/regclient/mod"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

type imageCmd struct {
	rootOpts        *rootCmd
	checkBaseRef    string
	checkBaseDigest string
	checkSkipConfig bool
	create          string
	exportCompress  bool
	exportRef       string
	fastCheck       bool
	forceRecursive  bool
	format          string
	formatFile      string
	importName      string
	includeExternal bool
	digestTags      bool
	list            bool
	modOpts         []mod.Opts
	platform        string
	platforms       []string
	referrers       bool
	replace         bool
	requireList     bool
}

func NewImageCmd(rootOpts *rootCmd) *cobra.Command {
	imageOpts := imageCmd{
		rootOpts: rootOpts,
	}
	// TODO: is there a better way to define an alias across parent commands?
	manifestOpts := manifestCmd{
		rootOpts: rootOpts,
	}
	var imageTopCmd = &cobra.Command{
		Use:   "image <cmd>",
		Short: "manage images",
	}
	var imageCheckBaseCmd = &cobra.Command{
		Use:     "check-base <image_ref>",
		Aliases: []string{},
		Short:   "check if the base image has changed",
		Long: `Check the base image (found using annotations or an option).
If the base name is not provided, annotations will be checked in the image.
If the digest is available, this checks if that matches the base name.
If the digest is not available, layers of each manifest are compared.
If the layers match, the config (history and roots) are optionally compared.	
If the base image does not match, the command exits with a non-zero status.
Use "-v info" to see more details.`,
		Example: `
# report if base image has changed using annotations
regctl image check-base ghcr.io/regclient/regctl:alpine -v info`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              imageOpts.runImageCheckBase,
	}
	var imageCopyCmd = &cobra.Command{
		Use:     "copy <src_image_ref> <dst_image_ref>",
		Aliases: []string{"cp"},
		Short:   "copy or retag image",
		Long: `Copy or retag an image. This works between registries and only pulls layers
that do not exist at the target. In the same registry it attempts to mount
the layers between repositories. And within the same repository it only
sends the manifest with the new tag.`,
		Example: `
# copy an image
regctl image copy \
  ghcr.io/regclient/regctl:edge registry.example.org/regclient/regctl:edge

# copy an image with signatures
regctl image copy --digest-tags \
  ghcr.io/regclient/regctl:edge registry.example.org/regclient/regctl:edge

# copy only the local platform image
regctl image copy --platform local \
  ghcr.io/regclient/regctl:edge registry.example.org/regclient/regctl:edge

# retag an image
regctl image copy registry.example.org/repo:v1.2.3 registry.example.org/repo:v1

# copy an image to an OCI Layout including referrers
regctl image copy --referrers \
  ghcr.io/regclient/regctl:edge ocidir://regctl:edge

# copy a windows image, including foreign layers
regctl image copy --platform windows/amd64,osver=10.0.17763.4974 --include-external \
  golang:latest registry.example.org/library/golang:windows`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              imageOpts.runImageCopy,
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
		Example: `
# delete a specific image
regctl image delete registry.example.org/repo@sha256:fab3c890d0480549d05d2ff3d746f42e360b7f0e3fe64bdf39fc572eab94911b

# delete a specific image by tag (including all other tags to the same image)
regctl image delete --force-tag-dereference registry.example.org/repo:v123`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete digests
		RunE:      manifestOpts.runManifestDelete,
	}
	var imageDigestCmd = &cobra.Command{
		Use:   "digest <image_ref>",
		Short: "show digest for pinning, same as \"manifest digest\"",
		Long:  `show digest for pinning, same as "manifest digest"`,
		Example: `
# get the digest for the latest regctl image
regctl image digest ghcr.io/regclient/regctl`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              manifestOpts.runManifestHead,
	}
	var imageExportCmd = &cobra.Command{
		Use:   "export <image_ref> [filename]",
		Short: "export image",
		Long: `Exports an image into a tar file that can be later loaded into a docker
engine with "docker load". The tar file is output to stdout by default.
Compression is typically not useful since layers are already compressed.`,
		Example: `
# export an image
regctl image export registry.example.org/repo:v1 >image-v1.tar`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              imageOpts.runImageExport,
	}
	var imageGetFileCmd = &cobra.Command{
		Use:     "get-file <image_ref> <filename> [out-file]",
		Aliases: []string{"cat"},
		Short:   "get a file from an image",
		Long:    `Go through each of the image layers searching for the requested file.`,
		Example: `
# get the alpine-release file from the latest alpine image
regctl image get-file --platform local alpine /etc/alpine-release`,
		Args:              cobra.RangeArgs(2, 3),
		ValidArgsFunction: completeArgList([]completeFunc{rootOpts.completeArgTag, completeArgNone, completeArgNone}),
		RunE:              imageOpts.runImageGetFile,
	}
	var imageImportCmd = &cobra.Command{
		Use:   "import <image_ref> <filename>",
		Short: "import image",
		Long: `Imports an image from a tar file. This must be either a docker formatted tar
from "docker save" or an OCI Layout compatible tar. The output from
"regctl image export" can be used. Stdin is not permitted for the tar file.`,
		Example: `
# import an image saved from docker
regctl image import registry.example.org/repo:v1 image-v1.tar`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeArgList([]completeFunc{rootOpts.completeArgTag, completeArgDefault}),
		RunE:              imageOpts.runImageImport,
	}
	var imageInspectCmd = &cobra.Command{
		Use:     "inspect <image_ref>",
		Aliases: []string{"config"},
		Short:   "inspect image",
		Long: `Shows the config json for an image and is equivalent to pulling the image
in docker, and inspecting it, but without pulling any of the image layers.`,
		Example: `
# return the image config for the nginx image
regctl image inspect --platform local nginx`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              imageOpts.runImageInspect,
	}
	var imageManifestCmd = &cobra.Command{
		Use:   "manifest <image_ref>",
		Short: "show manifest or manifest list, same as \"manifest get\"",
		Long:  `Shows the manifest or manifest list of the specified image.`,
		Example: `
# return the manifest of the golang image
regctl image manifest golang`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              manifestOpts.runManifestGet,
	}
	var imageModCmd = &cobra.Command{
		Use:   "mod <image_ref>",
		Short: "modify an image",
		// TODO: remove EXPERIMENTAL when stable
		Long: `EXPERIMENTAL: Applies requested modifications to an image
For time options, the value is a comma separated list of key/value pairs:
  set=${time}: time to set in rfc3339 format, e.g. 2006-01-02T15:04:05Z
  from-label=${label}: label used to extract time in rfc3339 format
  after=${time_in_rfc3339}: adjust any time after this
  base-ref=${image}: image to lookup base layers, which are skipped
  base-layers=${count}: number of layers to skip changing (from the base image)
  Note: set or from-label is required in the time options`,
		Example: `
# add an annotation to all images, replacing the v1 tag with the new image
regctl image mod registry.example.org/repo:v1 \
  --replace --annotation '[*]org.opencontainers.image.created=2021-02-03T05:06:07Z

# convert an image to the OCI media types, copying to local registry
regctl image mod alpine:3.5 --to-oci --create registry.example.org/alpine:3.5

# set the timestamp on the config and layers, ignoring the alpine base image layers
regctl image mod registry.example.org/repo:v1 --create v1-mod \
  --time "set=2021-02-03T04:05:06Z,base-ref=alpine:3"

# Rebase an older regctl image, copying to the local registry.
# This uses annotations that were included in the original image build.
regctl image mod registry.example.org/regctl:v0.5.1-alpine \
  --rebase --create v0.5.1-alpine-rebase`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              imageOpts.runImageMod,
	}
	var imageRateLimitCmd = &cobra.Command{
		Use:     "ratelimit <image_ref>",
		Aliases: []string{"rate-limit"},
		Short:   "show the current rate limit",
		Long: `Shows the rate limit using an http head request against the image manifest.
If Set is false, the Remain value was not provided.
The other values may be 0 if not provided by the registry.`,
		Example: `
# return the current rate limit for pulling the alpine image
regctl image ratelimit alpine

# return the number of pulls remaining
regctl image ratelimit alpine --format '{{.Remain}}'`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              imageOpts.runImageRateLimit,
	}

	imageOpts.modOpts = []mod.Opts{}

	imageCheckBaseCmd.Flags().StringVarP(&imageOpts.checkBaseRef, "base", "", "", "Base image reference (including tag)")
	imageCheckBaseCmd.Flags().StringVarP(&imageOpts.checkBaseDigest, "digest", "", "", "Base image digest (checks if digest matches base)")
	imageCheckBaseCmd.Flags().BoolVarP(&imageOpts.checkSkipConfig, "no-config", "", false, "Skip check of config history")
	imageCheckBaseCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")

	imageCopyCmd.Flags().BoolVarP(&imageOpts.fastCheck, "fast", "", false, "Fast check, skip referrers and digest tag checks when image exists, overrides force-recursive")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.forceRecursive, "force-recursive", "", false, "Force recursive copy of image, repairs missing nested blobs and manifests")
	imageCopyCmd.Flags().StringVarP(&imageOpts.format, "format", "", "", "Format output with go template syntax")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.includeExternal, "include-external", "", false, "Include external layers")
	imageCopyCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageCopyCmd.Flags().StringArrayVarP(&imageOpts.platforms, "platforms", "", []string{}, "Copy only specific platforms, registry validation must be disabled")
	// platforms should be treated as experimental since it will break many registries
	_ = imageCopyCmd.Flags().MarkHidden("platforms")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.digestTags, "digest-tags", "", false, "Include digest tags (\"sha256-<digest>.*\") when copying manifests")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.referrers, "referrers", "", false, "Include referrers")

	imageDeleteCmd.Flags().BoolVarP(&manifestOpts.forceTagDeref, "force-tag-dereference", "", false, "Dereference the a tag to a digest, this is unsafe")

	imageDigestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", true, "Do not resolve platform from manifest list (enabled by default)")
	imageDigestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageDigestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	_ = imageDigestCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	_ = imageDigestCmd.Flags().MarkHidden("list")

	imageGetFileCmd.Flags().StringVarP(&imageOpts.formatFile, "format", "", "", "Format output with go template syntax")
	imageGetFileCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")

	imageExportCmd.Flags().BoolVar(&imageOpts.exportCompress, "compress", false, "Compress output with gzip")
	imageExportCmd.Flags().StringVar(&imageOpts.exportRef, "name", "", "Name of image to embed for docker load")
	imageExportCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")

	imageImportCmd.Flags().StringVar(&imageOpts.importName, "name", "", "Name of image or tag to import when multiple images are packaged in the tar")

	imageInspectCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageInspectCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = imageInspectCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	_ = imageInspectCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageManifestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", true, "Output manifest list if available (enabled by default)")
	imageManifestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageManifestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	imageManifestCmd.Flags().StringVarP(&manifestOpts.formatGet, "format", "", "{{printPretty .}}", "Format output with go template syntax (use \"raw-body\" for the original manifest)")
	_ = imageManifestCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	_ = imageManifestCmd.RegisterFlagCompletionFunc("format", completeArgNone)
	_ = imageManifestCmd.Flags().MarkHidden("list")

	imageModCmd.Flags().StringVarP(&imageOpts.create, "create", "", "", "Create image or tag")
	imageModCmd.Flags().BoolVarP(&imageOpts.replace, "replace", "", false, "Replace tag (ignored when \"create\" is used)")
	// most image mod flags are order dependent, so they are added using VarP/VarPF to append to modOpts
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			vs := strings.SplitN(val, "=", 2)
			if len(vs) == 2 {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithAnnotation(vs[0], vs[1]))
			} else if len(vs) == 1 {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithAnnotation(vs[0], ""))
			} else {
				return fmt.Errorf("invalid annotation")
			}
			return nil
		},
	}, "annotation", "", `set an annotation (name=value, omit value to delete, prefix with platform list [p1,p2] or [*] for all images)`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			vs := strings.SplitN(val, ",", 2)
			if len(vs) < 1 {
				return fmt.Errorf("arg requires an image name and digest")
			}
			r, err := ref.New(vs[0])
			if err != nil {
				return fmt.Errorf("invalid image reference: %w", err)
			}
			d := digest.Digest("")
			if len(vs) == 1 {
				// parse ref with digest
				if r.Tag == "" || r.Digest == "" {
					return fmt.Errorf("arg requires an image name and digest")
				}
				d, err = digest.Parse(r.Digest)
				if err != nil {
					return fmt.Errorf("invalid digest: %w", err)
				}
				r.Digest = ""
			} else {
				// parse separate ref and digest
				d, err = digest.Parse(vs[1])
				if err != nil {
					return fmt.Errorf("invalid digest: %w", err)
				}
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithAnnotationOCIBase(r, d))
			return nil
		},
	}, "annotation-base", "", `set base image annotations (image/name:tag,sha256:digest)`)
	flagAnnotationPromote := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithAnnotationPromoteCommon())
			}
			return nil
		},
	}, "annotation-promote", "", `promote common annotations from child images to index`)
	flagAnnotationPromote.NoOptDefVal = "true"
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			vs := strings.SplitN(val, "=", 2)
			if len(vs) != 2 {
				return fmt.Errorf("arg must be in the format \"name=value\"")
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithBuildArgRm(vs[0], regexp.MustCompile(regexp.QuoteMeta(vs[1]))))
			return nil
		},
	}, "buildarg-rm", "", `delete a build arg`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			vs := strings.SplitN(val, "=", 2)
			if len(vs) != 2 {
				return fmt.Errorf("arg must be in the format \"name=regex\"")
			}
			value, err := regexp.Compile(vs[1])
			if err != nil {
				return fmt.Errorf("regexp value is invalid: %w", err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithBuildArgRm(vs[0], value))
			return nil
		},
	}, "buildarg-rm-regex", "", `delete a build arg with a regex value`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			p, err := platform.Parse(val)
			if err != nil {
				return err
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithConfigPlatform(p),
			)
			return nil
		},
	}, "config-platform", "", `set platform on the config (not recommended for an index of multiple images)`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			ot, otherFields, err := imageParseOptTime(val)
			if err != nil {
				return err
			}
			if len(otherFields) > 0 {
				keys := []string{}
				for k := range otherFields {
					keys = append(keys, k)
				}
				return fmt.Errorf("unknown time option: %s", strings.Join(keys, ", "))
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithConfigTimestamp(ot),
			)
			return nil
		},
	}, "config-time", "", `set timestamp for the config`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithConfigTimestamp(mod.OptTime{
					Set:   t,
					After: t,
				}))
			return nil
		},
	}, "config-time-max", "", `max timestamp for a config`)
	_ = imageModCmd.Flags().MarkHidden("config-time-max") // TODO: deprecate config-time-max in favor of config-time
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			size, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("unable to parse layer size %s: %w", val, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithData(size))
			return nil
		},
	}, "data-max", "", `sets or removes descriptor data field (size in bytes)`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithExposeAdd(val))
			return nil
		},
	}, "expose-add", "", `add an exposed port`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithExposeRm(val))
			return nil
		},
	}, "expose-rm", "", `delete an exposed port`)
	flagExtURLsRm := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithExternalURLsRm())
			}
			return nil
		},
	}, "external-urls-rm", "", `remove external url references from layers (first copy image with "--include-external")`)
	flagExtURLsRm.NoOptDefVal = "true"
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			ot, otherFields, err := imageParseOptTime(val)
			if err != nil {
				return err
			}
			if otherFields["filename"] == "" {
				return fmt.Errorf("filename must be included")
			}
			if len(otherFields) > 1 {
				keys := []string{}
				for k := range otherFields {
					if k != "filename" {
						keys = append(keys, k)
					}
				}
				return fmt.Errorf("unknown time option: %s", strings.Join(keys, ", "))
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithFileTarTime(otherFields["filename"], ot))
			return nil
		},
	}, "file-tar-time", "", `timestamp for contents of a tar file within a layer, set filename=${name} with time options`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			vs := strings.SplitN(val, ",", 2)
			if len(vs) != 2 {
				return fmt.Errorf("filename and timestamp both required, comma separated")
			}
			t, err := time.Parse(time.RFC3339, vs[1])
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithFileTarTime(vs[0], mod.OptTime{
				Set:   t,
				After: t,
			}))
			return nil
		},
	}, "file-tar-time-max", "", `max timestamp for contents of a tar file within a layer`)
	_ = imageModCmd.Flags().MarkHidden("file-tar-time-max") // TODO: deprecate in favor of file-tar-time
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			vs := strings.SplitN(val, "=", 2)
			if len(vs) == 2 {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLabel(vs[0], vs[1]))
			} else if len(vs) == 1 {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLabel(vs[0], ""))
			} else {
				return fmt.Errorf("invalid label")
			}
			return nil
		},
	}, "label", "", `set an label (name=value, omit value to delete, prefix with platform list [p1,p2] for subset of images)`)
	flagLabelAnnot := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLabelToAnnotation())
			}
			return nil
		},
	}, "label-to-annotation", "", `set annotations from labels`)
	flagLabelAnnot.NoOptDefVal = "true"
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			re, err := regexp.Compile(val)
			if err != nil {
				return fmt.Errorf("value must be a valid regex: %w", err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithLayerRmCreatedBy(*re))
			return nil
		},
	}, "layer-rm-created-by", "", `delete a layer based on history (created by string is a regex)`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "uint",
		f: func(val string) error {
			i, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("index invalid: %w", err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLayerRmIndex(i))
			return nil
		},
	}, "layer-rm-index", "", `delete a layer from an image (index begins at 0)`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLayerStripFile(val))
			return nil
		},
	}, "layer-strip-file", "", `delete a file or directory from all layers`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			ot, otherFields, err := imageParseOptTime(val)
			if err != nil {
				return err
			}
			if len(otherFields) > 0 {
				keys := []string{}
				for k := range otherFields {
					keys = append(keys, k)
				}
				return fmt.Errorf("unknown time option: %s", strings.Join(keys, ", "))
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithLayerTimestamp(ot),
			)
			return nil
		},
	}, "layer-time", "", `set timestamp for the layer contents`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLayerTimestamp(
				mod.OptTime{
					Set:   t,
					After: t,
				}))
			return nil
		},
	}, "layer-time-max", "", `max timestamp for a layer`)
	_ = imageModCmd.Flags().MarkHidden("layer-time-max") // TODO: deprecate in favor of layer-time
	flagRebase := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if !b {
				return nil
			}
			// pull the manifest, get the base image annotations
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithRebase())
			return nil
		},
	}, "rebase", "", `rebase an image using OCI annotations`)
	flagRebase.NoOptDefVal = "true"
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			vs := strings.SplitN(val, ",", 2)
			if len(vs) != 2 {
				return fmt.Errorf("rebase-ref requires two base images (old,new), comma separated")
			}
			// parse both refs
			rOld, err := ref.New(vs[0])
			if err != nil {
				return fmt.Errorf("failed parsing old base image ref: %w", err)
			}
			rNew, err := ref.New(vs[1])
			if err != nil {
				return fmt.Errorf("failed parsing new base image ref: %w", err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithRebaseRefs(rOld, rNew))
			return nil
		},
	}, "rebase-ref", "", `rebase an image with base references (base:old,base:new)`)
	flagReproducible := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLayerReproducible())
			}
			return nil
		},
	}, "reproducible", "", `fix tar headers for reproducibility`)
	flagReproducible.NoOptDefVal = "true"
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			ot, otherFields, err := imageParseOptTime(val)
			if err != nil {
				return err
			}
			if len(otherFields) > 0 {
				keys := []string{}
				for k := range otherFields {
					keys = append(keys, k)
				}
				return fmt.Errorf("unknown time option: %s", strings.Join(keys, ", "))
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithConfigTimestamp(ot),
				mod.WithLayerTimestamp(ot),
			)
			return nil
		},
	}, "time", "", `set timestamp for both the config and layers`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithConfigTimestamp(mod.OptTime{
					Set:   t,
					After: t,
				}),
				mod.WithLayerTimestamp(mod.OptTime{
					Set:   t,
					After: t,
				}))
			return nil
		},
	}, "time-max", "", `max timestamp for both the config and layers`)
	_ = imageModCmd.Flags().MarkHidden("time-max") // TODO: deprecate
	flagDocker := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithManifestToDocker())
			}
			return nil
		},
	}, "to-docker", "", `convert to Docker schema2 media types`)
	flagDocker.NoOptDefVal = "true"
	flagOCI := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithManifestToOCI())
			}
			return nil
		},
	}, "to-oci", "", `convert to OCI media types`)
	flagOCI.NoOptDefVal = "true"
	flagOCIReferrers := imageModCmd.Flags().VarPF(&modFlagFunc{
		t: "bool",
		f: func(val string) error {
			b, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("unable to parse value %s: %w", val, err)
			}
			if b {
				imageOpts.modOpts = append(imageOpts.modOpts, mod.WithManifestToOCIReferrers())
			}
			return nil
		},
	}, "to-oci-referrers", "", `convert to OCI referrers`)
	flagOCIReferrers.NoOptDefVal = "true"
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithVolumeAdd(val))
			return nil
		},
	}, "volume-add", "", `add a volume definition`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithVolumeRm(val))
			return nil
		},
	}, "volume-rm", "", `delete a volume definition`)

	imageRateLimitCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = imageRateLimitCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageTopCmd.AddCommand(imageCheckBaseCmd)
	imageTopCmd.AddCommand(imageCopyCmd)
	imageTopCmd.AddCommand(imageDeleteCmd)
	imageTopCmd.AddCommand(imageDigestCmd)
	imageTopCmd.AddCommand(imageExportCmd)
	imageTopCmd.AddCommand(imageGetFileCmd)
	imageTopCmd.AddCommand(imageImportCmd)
	imageTopCmd.AddCommand(imageInspectCmd)
	imageTopCmd.AddCommand(imageManifestCmd)
	imageTopCmd.AddCommand(imageModCmd)
	imageTopCmd.AddCommand(imageRateLimitCmd)
	return imageTopCmd
}

func imageParseOptTime(s string) (mod.OptTime, map[string]string, error) {
	ot := mod.OptTime{}
	otherFields := map[string]string{}
	for _, ss := range strings.Split(s, ",") {
		kv := strings.SplitN(ss, "=", 2)
		if len(kv) != 2 {
			return ot, otherFields, fmt.Errorf("parameter without a value: %s", ss)
		}
		switch kv[0] {
		case "set":
			t, err := time.Parse(time.RFC3339, kv[1])
			if err != nil {
				return ot, otherFields, fmt.Errorf("set time must be formatted %s: %w", time.RFC3339, err)
			}
			ot.Set = t
		case "after":
			t, err := time.Parse(time.RFC3339, kv[1])
			if err != nil {
				return ot, otherFields, fmt.Errorf("after time must be formatted %s: %w", time.RFC3339, err)
			}
			ot.After = t
		case "from-label":
			ot.FromLabel = kv[1]
		case "base-ref":
			r, err := ref.New(kv[1])
			if err != nil {
				return ot, otherFields, fmt.Errorf("failed to parse base ref: %w", err)
			}
			ot.BaseRef = r
		case "base-layers":
			i, err := strconv.Atoi(kv[1])
			if err != nil {
				return ot, otherFields, fmt.Errorf("unable to parse base layer count: %w", err)
			}
			ot.BaseLayers = i
		default:
			otherFields[kv[0]] = kv[1]
		}
	}
	return ot, otherFields, nil
}

func (imageOpts *imageCmd) runImageCheckBase(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := imageOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	opts := []regclient.ImageOpts{}
	if imageOpts.checkBaseDigest != "" {
		opts = append(opts, regclient.ImageWithCheckBaseDigest(imageOpts.checkBaseDigest))
	}
	if imageOpts.checkBaseRef != "" {
		opts = append(opts, regclient.ImageWithCheckBaseRef(imageOpts.checkBaseRef))
	}
	if imageOpts.checkSkipConfig {
		opts = append(opts, regclient.ImageWithCheckSkipConfig())
	}
	if imageOpts.platform != "" {
		opts = append(opts, regclient.ImageWithPlatform(imageOpts.platform))
	}

	err = rc.ImageCheckBase(ctx, r, opts...)
	if err == nil {
		log.Info("base image matches")
		return nil
	} else if errors.Is(err, errs.ErrMismatch) {
		log.WithFields(logrus.Fields{
			"err": err,
		}).Info("base image mismatch")
		// return empty error message
		return fmt.Errorf("%.0w", err)
	} else {
		return err
	}
}

func (imageOpts *imageCmd) runImageCopy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	rSrc, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rTgt, err := ref.New(args[1])
	if err != nil {
		return err
	}
	rc := imageOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, rSrc)
	defer rc.Close(ctx, rTgt)
	if imageOpts.platform != "" {
		p, err := platform.Parse(imageOpts.platform)
		if err != nil {
			return err
		}
		m, err := rc.ManifestGet(ctx, rSrc)
		if err != nil {
			return err
		}
		if m.IsList() {
			d, err := manifest.GetPlatformDesc(m, &p)
			if err != nil {
				return err
			}
			rSrc.Digest = d.Digest.String()
		}
	}
	log.WithFields(logrus.Fields{
		"source":      rSrc.CommonName(),
		"target":      rTgt.CommonName(),
		"recursive":   imageOpts.forceRecursive,
		"digest-tags": imageOpts.digestTags,
	}).Debug("Image copy")
	opts := []regclient.ImageOpts{}
	if imageOpts.fastCheck {
		opts = append(opts, regclient.ImageWithFastCheck())
	}
	if imageOpts.forceRecursive {
		opts = append(opts, regclient.ImageWithForceRecursive())
	}
	if imageOpts.includeExternal {
		opts = append(opts, regclient.ImageWithIncludeExternal())
	}
	if imageOpts.digestTags {
		opts = append(opts, regclient.ImageWithDigestTags())
	}
	if imageOpts.referrers {
		opts = append(opts, regclient.ImageWithReferrers())
	}
	if len(imageOpts.platforms) > 0 {
		opts = append(opts, regclient.ImageWithPlatforms(imageOpts.platforms))
	}
	// check for a tty and attach progress reporter
	done := make(chan bool)
	var progress *imageProgress
	if !flagChanged(cmd, "verbosity") && ascii.IsWriterTerminal(cmd.ErrOrStderr()) {
		progress = &imageProgress{
			start:   time.Now(),
			entries: map[string]*imageProgressEntry{},
			asciOut: ascii.NewLines(cmd.ErrOrStderr()),
			bar:     ascii.NewProgressBar(cmd.ErrOrStderr()),
		}
		ticker := time.NewTicker(progressFreq)
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-done:
					ticker.Stop()
					return
				case <-ticker.C:
					progress.display(cmd.ErrOrStderr(), false)
				}
			}
		}()
		opts = append(opts, regclient.ImageWithCallback(progress.callback))
	}
	err = rc.ImageCopy(ctx, rSrc, rTgt, opts...)
	if progress != nil {
		close(done)
		progress.display(cmd.ErrOrStderr(), true)
	}
	if err != nil {
		return err
	}
	if !flagChanged(cmd, "format") {
		imageOpts.format = "{{ .CommonName }}\n"
	}
	return template.Writer(cmd.OutOrStdout(), imageOpts.format, rTgt)
}

type imageProgress struct {
	mu      sync.Mutex
	start   time.Time
	entries map[string]*imageProgressEntry
	asciOut *ascii.Lines
	bar     *ascii.ProgressBar
	changed bool
}

type imageProgressEntry struct {
	kind        types.CallbackKind
	instance    string
	state       types.CallbackState
	start, last time.Time
	cur, total  int64
	bps         []float64
}

func (ip *imageProgress) callback(kind types.CallbackKind, instance string, state types.CallbackState, cur, total int64) {
	// track kind/instance
	ip.mu.Lock()
	defer ip.mu.Unlock()
	ip.changed = true
	now := time.Now()
	if e, ok := ip.entries[kind.String()+":"+instance]; ok {
		e.state = state
		diff := now.Sub(e.last)
		bps := float64(cur-e.cur) / diff.Seconds()
		e.state = state
		e.last = now
		e.cur = cur
		e.total = total
		if len(e.bps) >= 10 {
			e.bps = append(e.bps[1:], bps)
		} else {
			e.bps = append(e.bps, bps)
		}
	} else {
		ip.entries[kind.String()+":"+instance] = &imageProgressEntry{
			kind:     kind,
			instance: instance,
			state:    state,
			start:    now,
			last:     now,
			cur:      cur,
			total:    total,
			bps:      []float64{},
		}
	}
}

func (ip *imageProgress) display(w io.Writer, final bool) {
	ip.mu.Lock()
	defer ip.mu.Unlock()
	if !ip.changed && !final {
		return // skip since no changes since last display and not the final display
	}
	var manifestTotal, manifestFinished, sum, skipped, queued int64
	// sort entry keys by start time
	keys := make([]string, 0, len(ip.entries))
	for k := range ip.entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if ip.entries[keys[a]].state != ip.entries[keys[b]].state {
			return ip.entries[keys[a]].state > ip.entries[keys[b]].state
		} else if ip.entries[keys[a]].state != types.CallbackActive {
			return ip.entries[keys[a]].last.Before(ip.entries[keys[b]].last)
		} else {
			return ip.entries[keys[a]].cur > ip.entries[keys[b]].cur
		}
	})
	startCount, startLimit := 0, 2
	finishedCount, finishedLimit := 0, 2
	// hide old finished entries
	for i := len(keys) - 1; i >= 0; i-- {
		e := ip.entries[keys[i]]
		if e.kind != types.CallbackManifest && e.state == types.CallbackFinished {
			finishedCount++
			if finishedCount > finishedLimit {
				e.state = types.CallbackArchived
			}
		}
	}
	for _, k := range keys {
		e := ip.entries[k]
		switch e.kind {
		case types.CallbackManifest:
			manifestTotal++
			if e.state == types.CallbackFinished || e.state == types.CallbackSkipped {
				manifestFinished++
			}
		default:
			// show progress bars
			if !final && (e.state == types.CallbackActive || (e.state == types.CallbackStarted && startCount < startLimit) || e.state == types.CallbackFinished) {
				if e.state == types.CallbackStarted {
					startCount++
				}
				pre := e.instance + " "
				if len(pre) > 15 {
					pre = pre[:14] + " "
				}
				pct := float64(e.cur) / float64(e.total)
				post := fmt.Sprintf(" %4.2f%% %s/%s", pct*100, units.HumanSize(float64(e.cur)), units.HumanSize(float64(e.total)))
				ip.asciOut.Add(ip.bar.Generate(pct, pre, post))
			}
			// track stats
			if e.state == types.CallbackSkipped {
				skipped += e.total
			} else if e.total > 0 {
				sum += e.cur
				queued += e.total - e.cur
			}
		}
	}
	// show stats summary
	ip.asciOut.Add([]byte(fmt.Sprintf("Manifests: %d/%d | Blobs: %s copied, %s skipped",
		manifestFinished, manifestTotal,
		units.HumanSize(float64(sum)),
		units.HumanSize(float64(skipped)))))
	if queued > 0 {
		ip.asciOut.Add([]byte(fmt.Sprintf(", %s queued",
			units.HumanSize(float64(queued)))))
	}
	ip.asciOut.Add([]byte(fmt.Sprintf(" | Elapsed: %ds\n", int64(time.Since(ip.start).Seconds()))))
	ip.asciOut.Flush()
	if !final {
		ip.asciOut.Return()
	}
}

func (imageOpts *imageCmd) runImageExport(cmd *cobra.Command, args []string) error {
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
		w = cmd.OutOrStdout()
	}
	rc := imageOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)
	opts := []regclient.ImageOpts{}
	if imageOpts.platform != "" {
		p, err := platform.Parse(imageOpts.platform)
		if err != nil {
			return err
		}
		m, err := rc.ManifestGet(ctx, r)
		if err != nil {
			return err
		}
		if m.IsList() {
			d, err := manifest.GetPlatformDesc(m, &p)
			if err != nil {
				return err
			}
			r.Digest = d.Digest.String()
		}
	}
	if imageOpts.exportCompress {
		opts = append(opts, regclient.ImageWithExportCompress())
	}
	if imageOpts.exportRef != "" {
		eRef, err := ref.New(imageOpts.exportRef)
		if err != nil {
			return fmt.Errorf("cannot parse %s: %w", imageOpts.exportRef, err)
		}
		opts = append(opts, regclient.ImageWithExportRef(eRef))
	}
	log.WithFields(logrus.Fields{
		"ref": r.CommonName(),
	}).Debug("Image export")
	return rc.ImageExport(ctx, r, w, opts...)
}

func (imageOpts *imageCmd) runImageGetFile(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	filename := args[1]
	filename = strings.TrimPrefix(filename, "/")
	rc := imageOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"ref":      r.CommonName(),
		"filename": filename,
	}).Debug("Get file")

	// make it recursive for index of index scenarios
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}
	if m.IsList() {
		if imageOpts.platform == "" {
			imageOpts.platform = "local"
		}
		plat, err := platform.Parse(imageOpts.platform)
		if err != nil {
			log.WithFields(logrus.Fields{
				"platform": imageOpts.platform,
				"err":      err,
			}).Warn("Could not parse platform")
		}
		desc, err := manifest.GetPlatformDesc(m, &plat)
		if err != nil {
			pl, _ := manifest.GetPlatformList(m)
			var ps []string
			for _, p := range pl {
				ps = append(ps, p.String())
			}
			log.WithFields(logrus.Fields{
				"platform":  plat,
				"err":       err,
				"platforms": strings.Join(ps, ", "),
			}).Warn("Platform could not be found in manifest list")
			return err
		}
		m, err = rc.ManifestGet(ctx, r, regclient.WithManifestDesc(*desc))
		if err != nil {
			return fmt.Errorf("failed to pull platform specific digest: %w", err)
		}
	}
	// go through layers in reverse
	mi, ok := m.(manifest.Imager)
	if !ok {
		return fmt.Errorf("reference is not a known image media type")
	}
	layers, err := mi.GetLayers()
	if err != nil {
		return err
	}
	for i := len(layers) - 1; i >= 0; i-- {
		blob, err := rc.BlobGet(ctx, r, layers[i])
		if err != nil {
			return fmt.Errorf("failed pulling layer %d: %w", i, err)
		}
		btr, err := blob.ToTarReader()
		if err != nil {
			return fmt.Errorf("could not convert layer %d to tar reader: %w", i, err)
		}
		th, rdr, err := btr.ReadFile(filename)
		if err != nil {
			if errors.Is(err, errs.ErrFileNotFound) {
				if err := btr.Close(); err != nil {
					return err
				}
				if err := blob.Close(); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("failed pulling from layer %d: %w", i, err)
		}
		// file found, output
		if imageOpts.formatFile != "" {
			data := struct {
				Header *tar.Header
				Reader io.Reader
			}{
				Header: th,
				Reader: rdr,
			}
			return template.Writer(cmd.OutOrStdout(), imageOpts.formatFile, data)
		}
		var w io.Writer
		if len(args) < 3 {
			w = cmd.OutOrStdout()
		} else {
			w, err = os.Create(args[2])
			if err != nil {
				return err
			}
		}
		_, err = io.Copy(w, rdr)
		if err != nil {
			return err
		}
		if err := btr.Close(); err != nil {
			return err
		}
		if err := blob.Close(); err != nil {
			return err
		}
		return nil
	}
	// all layers exhausted, not found or deleted
	return errs.ErrNotFound
}

func (imageOpts *imageCmd) runImageImport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	opts := []regclient.ImageOpts{}
	if imageOpts.importName != "" {
		opts = append(opts, regclient.ImageWithImportName(imageOpts.importName))
	}
	rs, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer rs.Close()
	rc := imageOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)
	log.WithFields(logrus.Fields{
		"ref":  r.CommonName(),
		"file": args[1],
	}).Debug("Image import")

	return rc.ImageImport(ctx, r, rs, opts...)
}

func (imageOpts *imageCmd) runImageInspect(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := imageOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"host":     r.Registry,
		"repo":     r.Repository,
		"tag":      r.Tag,
		"platform": imageOpts.platform,
	}).Debug("Image inspect")

	m, err := getManifest(ctx, rc, r, imageOpts.platform, imageOpts.list, imageOpts.requireList)
	if err != nil {
		return err
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		return fmt.Errorf("manifest does not support image methods%.0w", errs.ErrUnsupportedMediaType)
	}
	cd, err := mi.GetConfig()
	if err != nil {
		return err
	}

	blobConfig, err := rc.BlobGetOCIConfig(ctx, r, cd)
	if err != nil {
		return err
	}
	result := struct {
		*blob.BOCIConfig
		v1.Image
	}{
		BOCIConfig: blobConfig,
		Image:      blobConfig.GetConfig(),
	}
	switch imageOpts.format {
	case "raw":
		imageOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		imageOpts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		imageOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), imageOpts.format, result)
}

func (imageOpts *imageCmd) runImageMod(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	rSrc, err := ref.New(args[0])
	if err != nil {
		return err
	}
	var rTgt ref.Ref
	if imageOpts.create != "" {
		if strings.ContainsAny(imageOpts.create, "/:") {
			rTgt, err = ref.New((imageOpts.create))
			if err != nil {
				return fmt.Errorf("failed to parse new image name %s: %w", imageOpts.create, err)
			}
		} else {
			rTgt = rSrc.SetTag(imageOpts.create)
		}
	} else if imageOpts.replace {
		rTgt = rSrc
	} else {
		rTgt = rSrc
		rTgt.Tag = ""
	}
	imageOpts.modOpts = append(imageOpts.modOpts, mod.WithRefTgt(rTgt))
	rc := imageOpts.rootOpts.newRegClient()

	log.WithFields(logrus.Fields{
		"ref": rSrc.CommonName(),
	}).Debug("Modifying image")

	defer rc.Close(ctx, rSrc)
	rOut, err := mod.Apply(ctx, rc, rSrc, imageOpts.modOpts...)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", rOut.CommonName())
	err = rc.Close(ctx, rOut)
	if err != nil {
		return fmt.Errorf("failed to close ref: %w", err)
	}
	return nil
}

func (imageOpts *imageCmd) runImageRateLimit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := imageOpts.rootOpts.newRegClient()

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

	return template.Writer(cmd.OutOrStdout(), imageOpts.format, manifest.GetRateLimit(m))
}

type modFlagFunc struct {
	f func(string) error
	t string
}

func (m *modFlagFunc) IsBoolFlag() bool {
	return m.t == "bool"
}

func (m *modFlagFunc) String() string {
	return ""
}

func (m *modFlagFunc) Set(val string) error {
	return m.f(val)
}

func (m *modFlagFunc) Type() string {
	return m.t
}
