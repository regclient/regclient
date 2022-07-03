package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/mod"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
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
var imageModCmd = &cobra.Command{
	Hidden:            true, // TODO: remove when stable, and remove EXPERIMENTAL from description below
	Use:               "mod <image_ref>",
	Short:             "modify an image",
	Long:              `EXPERIMENTAL: Applies requested modifications to an image`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeArgTag,
	RunE:              runImageMod,
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
	create          string
	forceRecursive  bool
	format          string
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

func init() {
	imageOpts.modOpts = []mod.Opts{}

	imageCopyCmd.Flags().BoolVarP(&imageOpts.forceRecursive, "force-recursive", "", false, "Force recursive copy of image, repairs missing nested blobs and manifests")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.includeExternal, "include-external", "", false, "Include external layers")
	imageCopyCmd.Flags().StringArrayVarP(&imageOpts.platforms, "platforms", "", []string{}, "Copy only specific platforms, registry validation must be disabled")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.digestTags, "digest-tags", "", false, "Include digest tags (\"sha256-<digest>.*\") when copying manifests")
	imageCopyCmd.Flags().BoolVarP(&imageOpts.referrers, "referrers", "", false, "Experimental: Include referrers")
	// platforms should be treated as experimental since it will break many registries
	imageCopyCmd.Flags().MarkHidden("platforms")

	imageDeleteCmd.Flags().BoolVarP(&manifestOpts.forceTagDeref, "force-tag-dereference", "", false, "Dereference the a tag to a digest, this is unsafe")

	imageDigestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", true, "Do not resolve platform from manifest list (enabled by default)")
	imageDigestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageDigestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	imageDigestCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	imageDigestCmd.Flags().MarkHidden("list")

	imageInspectCmd.Flags().StringVarP(&imageOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageInspectCmd.Flags().StringVarP(&imageOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	imageInspectCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	imageInspectCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageManifestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", true, "Output manifest list if available (enabled by default)")
	imageManifestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	imageManifestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	imageManifestCmd.Flags().StringVarP(&manifestOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax (use \"raw-body\" for the original manifest)")
	imageManifestCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	imageManifestCmd.RegisterFlagCompletionFunc("format", completeArgNone)
	imageManifestCmd.Flags().MarkHidden("list")

	imageModCmd.Flags().StringVarP(&imageOpts.create, "create", "", "", "Create tag")
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
	}, "annotation", "", `set an annotation (name=value)`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "stringArray",
		f: func(val string) error {
			vs := strings.SplitN(val, ",", 2)
			if len(vs) < 1 {
				return fmt.Errorf("arg requires an image name and digest")
			}
			r, err := ref.New(vs[0])
			if err != nil {
				return fmt.Errorf("invalid image reference: %v", err)
			}
			d := digest.Digest("")
			if len(vs) == 1 {
				// parse ref with digest
				if r.Tag == "" || r.Digest == "" {
					return fmt.Errorf("arg requires an image name and digest")
				}
				d, err = digest.Parse(r.Digest)
				if err != nil {
					return fmt.Errorf("invalid digest: %v", err)
				}
				r.Digest = ""
			} else {
				// parse separate ref and digest
				d, err = digest.Parse(vs[1])
				if err != nil {
					return fmt.Errorf("invalid digest: %v", err)
				}
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithAnnotationOCIBase(r, d))
			return nil
		},
	}, "annotation-base", "", `set base image annotations (image/name:tag,sha256:digest)`)
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
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithConfigTimestampMax(t))
			return nil
		},
	}, "config-time-max", "", `max timestamp for a config`)
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
			vs := strings.SplitN(val, ",", 2)
			if len(vs) != 2 {
				return fmt.Errorf("filename and timestamp both required, comma separated")
			}
			t, err := time.Parse(time.RFC3339, vs[1])
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithFileTarTimeMax(vs[0], t))
			return nil
		},
	}, "file-tar-time-max", "", `max timestamp for contents of a tar file within a layer`)
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
	}, "label", "", `set an label (name=value)`)
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
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts, mod.WithLayerTimestampMax(t))
			return nil
		},
	}, "layer-time-max", "", `max timestamp for a layer`)
	imageModCmd.Flags().VarP(&modFlagFunc{
		t: "string",
		f: func(val string) error {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return fmt.Errorf("time must be formatted %s: %w", time.RFC3339, err)
			}
			imageOpts.modOpts = append(imageOpts.modOpts,
				mod.WithConfigTimestampMax(t),
				mod.WithLayerTimestampMax(t))
			return nil
		},
	}, "time-max", "", `max timestamp for both the config and layers`)
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
	imageRateLimitCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	imageCmd.AddCommand(imageCopyCmd)
	imageCmd.AddCommand(imageDeleteCmd)
	imageCmd.AddCommand(imageDigestCmd)
	imageCmd.AddCommand(imageExportCmd)
	imageCmd.AddCommand(imageImportCmd)
	imageCmd.AddCommand(imageInspectCmd)
	imageCmd.AddCommand(imageManifestCmd)
	imageCmd.AddCommand(imageModCmd)
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
		"source":      rSrc.CommonName(),
		"target":      rTgt.CommonName(),
		"recursive":   imageOpts.forceRecursive,
		"digest-tags": imageOpts.digestTags,
	}).Debug("Image copy")
	opts := []regclient.ImageOpts{}
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
	if !flagChanged(cmd, "list") {
		manifestOpts.list = false
	}

	m, err := getManifest(ctx, rc, r)
	if err != nil {
		return err
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		return fmt.Errorf("manifest does not support image methods%.0w", types.ErrUnsupportedMediaType)
	}
	cd, err := mi.GetConfig()
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

func runImageMod(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	var rNew ref.Ref
	if imageOpts.create != "" {
		if strings.ContainsAny(imageOpts.create, "/:") {
			rNew, err = ref.New((imageOpts.create))
			if err != nil {
				return fmt.Errorf("failed to parse new image name %s: %w", imageOpts.create, err)
			}
		} else {
			rNew = r
			rNew.Digest = ""
			rNew.Tag = imageOpts.create
		}
	} else if imageOpts.replace {
		if r.Tag == "" {
			return fmt.Errorf("cannot replace an image digest, must include a tag")
		}
		rNew = r
		rNew.Digest = ""
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"ref": r.CommonName(),
	}).Debug("Modifying image")

	defer rc.Close(ctx, r)
	rOut, err := mod.Apply(ctx, rc, r, imageOpts.modOpts...)
	if err != nil {
		return err
	}
	if rNew.Tag != "" {
		defer rc.Close(ctx, rNew)
		err = rc.ImageCopy(ctx, rOut, rNew)
		if err != nil {
			return fmt.Errorf("failed copying image to new name: %w", err)
		}
		fmt.Printf("%s\n", rNew.CommonName())
	} else {
		fmt.Printf("%s\n", rOut.CommonName())
	}
	return nil
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

	return template.Writer(os.Stdout, imageOpts.format, manifest.GetRateLimit(m))
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
