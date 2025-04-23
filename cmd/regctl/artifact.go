package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
	"github.com/regclient/regclient/types/warning"
)

const (
	ociAnnotTitle     = "org.opencontainers.image.title"
	defaultMTArtifact = "application/vnd.unknown.config+json"
	defaultMTLayer    = "application/octet-stream"
)

var manifestKnownTypes = []string{
	mediatype.OCI1Manifest,
}
var artifactFileKnownTypes = []string{
	"application/octet-stream",
	"application/tar+gzip",
	"application/vnd.oci.image.layer.v1.tar",
	"application/vnd.oci.image.layer.v1.tar+zstd",
	"application/vnd.oci.image.layer.v1.tar+gzip",
}
var configKnownTypes = []string{
	"application/vnd.oci.image.config.v1+json",
	"application/vnd.cncf.helm.chart.config.v1+json",
	"application/vnd.sylabs.sif.config.v1+json",
}

type artifactOpts struct {
	rootOpts         *rootOpts
	annotations      []string
	artifactMT       string
	artifactType     string
	artifactConfig   string
	artifactConfigMT string
	artifactFile     []string
	artifactFileMT   []string
	artifactTitle    bool
	byDigest         bool
	digestTags       bool
	externalRepo     string
	filterAT         string
	filterAnnot      []string
	format           string
	getConfig        bool
	index            bool
	latest           bool
	outputDir        string
	platform         string
	refers           string
	sortAnnot        string
	sortDesc         bool
	stripDirs        bool
	subject          string
}

func NewArtifactCmd(rOpts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact <cmd>",
		Short: "manage artifacts",
	}
	cmd.AddCommand(newArtifactGetCmd(rOpts))
	cmd.AddCommand(newArtifactListCmd(rOpts))
	cmd.AddCommand(newArtifactPutCmd(rOpts))
	cmd.AddCommand(newArtifactTreeCmd(rOpts))
	return cmd
}

func newArtifactGetCmd(rOpts *rootOpts) *cobra.Command {
	opts := artifactOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "get <reference>",
		Aliases: []string{"pull"},
		Short:   "download artifacts",
		Long:    `Download artifacts from the registry.`,
		Example: `
# download a helm chart
regctl artifact get registry.example.org/helm-charts/chart:0.0.1 > chart.tgz

# retrieve the SPDX SBOM for the latest regsync image for this platform
regctl artifact get \
  --subject ghcr.io/regclient/regsync:latest \
  --filter-artifact-type application/spdx+json \
  --platform local | jq .
  
# retrieve the artifact config rather than the artifact itself
regctl artifact get registry.example.org/artifact:0.0.1 --config`,
		Args:      cobra.RangeArgs(0, 1),
		ValidArgs: []string{}, // do not auto complete repository/tag
		RunE:      opts.runArtifactGet,
	}
	cmd.Flags().BoolVar(&opts.getConfig, "config", false, "Show the config, overrides file options")
	cmd.Flags().StringVar(&opts.artifactConfig, "config-file", "", "Output config to a file")
	cmd.Flags().StringVar(&opts.externalRepo, "external", "", "Query referrers from a separate source")
	cmd.Flags().StringArrayVarP(&opts.artifactFile, "file", "f", []string{}, "Filter by artifact filename")
	cmd.Flags().StringArrayVarP(&opts.artifactFileMT, "file-media-type", "m", []string{}, "Filter by artifact media-type")
	_ = cmd.RegisterFlagCompletionFunc("file-media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return artifactFileKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringArrayVar(&opts.filterAnnot, "filter-annotation", []string{}, "Filter referrers by annotation (key=value)")
	cmd.Flags().StringVar(&opts.filterAT, "filter-artifact-type", "", "Filter referrers by artifactType")
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "Get the most recent referrer using the OCI created annotation")
	cmd.Flags().StringVarP(&opts.outputDir, "output", "o", "", "Output directory for multiple artifacts")
	cmd.Flags().StringVarP(&opts.platform, "platform", "p", "", "Specify platform of a subject (e.g. linux/amd64 or local)")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	cmd.Flags().StringVar(&opts.refers, "refers", "", "Deprecated: Get a referrer to the reference")
	_ = cmd.Flags().MarkHidden("refers")
	cmd.Flags().StringVar(&opts.sortAnnot, "sort-annotation", "", "Annotation used for sorting results")
	cmd.Flags().BoolVar(&opts.sortDesc, "sort-desc", false, "Sort in descending order")
	cmd.Flags().StringVar(&opts.subject, "subject", "", "Get a referrer to the subject reference")
	cmd.Flags().BoolVar(&opts.stripDirs, "strip-dirs", false, "Strip directories from filenames in output dir")
	return cmd
}

func newArtifactListCmd(rOpts *rootOpts) *cobra.Command {
	opts := artifactOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "list <reference>",
		Aliases: []string{"ls"},
		Short:   "list artifacts that have a subject to the given reference",
		Long:    `List artifacts that have a subject to the given reference.`,
		Example: `
# list all referrers of the regsync package for the local platform
regctl artifact list ghcr.io/regclient/regctl --platform local

# return the original referrers response
regctl artifact list registry.example.com/repo:v1 --format body

# pretty print the referrers response
regctl artifact list registry.example.com/repo:v1 --format '{{jsonPretty .Manifest}}'`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete repository/tag
		RunE:      opts.runArtifactList,
	}
	cmd.Flags().BoolVar(&opts.digestTags, "digest-tags", false, "Include digest tags")
	cmd.Flags().StringVar(&opts.externalRepo, "external", "", "Query referrers from a separate source")
	cmd.Flags().StringVar(&opts.filterAT, "filter-artifact-type", "", "Filter descriptors by artifactType")
	cmd.Flags().StringArrayVar(&opts.filterAnnot, "filter-annotation", []string{}, "Filter descriptors by annotation (key=value)")
	cmd.Flags().StringVar(&opts.format, "format", "{{printPretty .}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "Sort using the OCI created annotation")
	cmd.Flags().StringVarP(&opts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	cmd.Flags().StringVar(&opts.sortAnnot, "sort-annotation", "", "Annotation used for sorting results")
	cmd.Flags().BoolVar(&opts.sortDesc, "sort-desc", false, "Sort in descending order")
	return cmd
}

func newArtifactPutCmd(rOpts *rootOpts) *cobra.Command {
	opts := artifactOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "put <reference>",
		Aliases: []string{"create", "push"},
		Short:   "upload artifacts",
		Long:    `Upload artifacts to the registry.`,
		Example: `
# push a simple artifact by name
regctl artifact put \
  --artifact-type application/example.test \
  registry.example.com/repo:artifact <text.txt

# push an artifact with a config
regctl artifact put \
  --config-type application/vnd.example.config.v1+json \
  --config-file config.json \
  --file-media-type application/vnd.example.data.v1.tar+gzip \
  --file data.tgz \
  registry.example.com/repo:artifact

# push an SBOM that is a referrer to an existing image
regctl artifact put \
  --artifact-type application/spdx+json \
  --subject registry.example.com/repo:v1 \
  < spdx.json`,
		Args:      cobra.RangeArgs(0, 1),
		ValidArgs: []string{}, // do not auto complete repository/tag
		RunE:      opts.runArtifactPut,
	}
	cmd.Flags().StringArrayVar(&opts.annotations, "annotation", []string{}, "Annotation to include on manifest")
	cmd.Flags().StringVar(&opts.artifactType, "artifact-type", "", "Artifact type (recommended)")
	_ = cmd.RegisterFlagCompletionFunc("artifact-type", completeArgNone)
	cmd.Flags().BoolVar(&opts.byDigest, "by-digest", false, "Push manifest by digest instead of tag")
	cmd.Flags().StringVar(&opts.artifactConfig, "config-file", "", "Filename for config content")
	cmd.Flags().StringVar(&opts.artifactConfigMT, "config-type", "", "Config mediaType")
	_ = cmd.RegisterFlagCompletionFunc("config-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return configKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringVar(&opts.externalRepo, "external", "", "Push referrers to a separate repository")
	cmd.Flags().StringArrayVarP(&opts.artifactFile, "file", "f", []string{}, "Artifact filename")
	cmd.Flags().StringArrayVarP(&opts.artifactFileMT, "file-media-type", "m", []string{}, "Set the mediaType for the individual files")
	_ = cmd.RegisterFlagCompletionFunc("file-media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return artifactFileKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().BoolVar(&opts.artifactTitle, "file-title", false, "Include a title annotation with the filename")
	cmd.Flags().StringVarP(&opts.artifactMT, "media-type", "", mediatype.OCI1Manifest, "EXPERIMENTAL: Manifest media-type")
	_ = cmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return manifestKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.Flags().MarkHidden("media-type")
	cmd.Flags().StringVar(&opts.format, "format", "", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().BoolVar(&opts.index, "index", false, "Create/append artifact to an index")
	cmd.Flags().StringVarP(&opts.platform, "platform", "p", "", "Specify platform of a subject (e.g. linux/amd64 or local)")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	cmd.Flags().StringVar(&opts.refers, "refers", "", "EXPERIMENTAL: Set a referrer to the reference")
	_ = cmd.Flags().MarkHidden("refers")
	cmd.Flags().BoolVar(&opts.stripDirs, "strip-dirs", false, "Strip directories from filenames in file-title")
	cmd.Flags().StringVar(&opts.subject, "subject", "", "Set the subject to a reference (used for referrer queries)")
	return cmd
}

func newArtifactTreeCmd(rOpts *rootOpts) *cobra.Command {
	opts := artifactOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "tree <reference>",
		Aliases: []string{},
		Short:   "tree listing of artifacts",
		Long: `Return a graph of manifests and referrers to those manifests.
This command will recursively query referrers to all child images.
For a single image, it is better to run "regctl artifact list".`,
		Example: `
# list all referrers to the latest regsync image
regctl artifact tree ghcr.io/regclient/regsync:latest

# include digest tags (used by sigstore)
regctl artifact tree --digest-tags ghcr.io/regclient/regsync:latest`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete repository/tag
		RunE:      opts.runArtifactTree,
	}
	cmd.Flags().BoolVar(&opts.digestTags, "digest-tags", false, "Include digest tags")
	cmd.Flags().StringVar(&opts.externalRepo, "external", "", "Query referrers from a separate source")
	cmd.Flags().StringVar(&opts.filterAT, "filter-artifact-type", "", "Filter descriptors by artifactType")
	cmd.Flags().StringArrayVar(&opts.filterAnnot, "filter-annotation", []string{}, "Filter descriptors by annotation (key=value)")
	cmd.Flags().StringVar(&opts.format, "format", "{{printPretty .}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
}

func (opts *artifactOpts) runArtifactGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	rc := opts.rootOpts.newRegClient()

	// validate inputs
	if opts.refers != "" {
		opts.rootOpts.log.Warn("--refers is deprecated, use --subject instead")
		if opts.subject == "" {
			opts.subject = opts.refers
		}
	}
	if opts.externalRepo != "" && opts.subject == "" {
		opts.rootOpts.log.Warn("--external option depends on --subject")
	}
	if opts.latest && opts.sortAnnot != "" {
		return fmt.Errorf("--latest cannot be used with --sort-annotation")
	}
	// if output dir defined, ensure it exists
	if opts.outputDir != "" {
		fi, err := os.Stat(opts.outputDir)
		if err != nil {
			return fmt.Errorf("output directory unavailable: %w", err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("output must be a directory: \"%s\"", opts.outputDir)
		}
	}
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	r := ref.Ref{}
	matchOpts := descriptor.MatchOpt{
		ArtifactType:   opts.filterAT,
		SortAnnotation: opts.sortAnnot,
		SortDesc:       opts.sortDesc,
	}
	if opts.filterAnnot != nil {
		matchOpts.Annotations = map[string]string{}
		for _, kv := range opts.filterAnnot {
			kvSplit := strings.SplitN(kv, "=", 2)
			if len(kvSplit) == 2 {
				matchOpts.Annotations[kvSplit[0]] = kvSplit[1]
			} else {
				matchOpts.Annotations[kv] = ""
			}
		}
	}
	if opts.latest {
		matchOpts.SortAnnotation = types.AnnotationCreated
		matchOpts.SortDesc = true
	}
	if opts.platform != "" {
		p, err := platform.Parse(opts.platform)
		if err != nil {
			return fmt.Errorf("platform could not be parsed: %w", err)
		}
		matchOpts.Platform = &p
	}

	// lookup referrers to the subject
	if len(args) == 0 && opts.subject != "" {
		rSubject, err := ref.New(opts.subject)
		if err != nil {
			return err
		}
		referrerMatchOpts := matchOpts
		referrerMatchOpts.Platform = nil
		referrerOpts := []scheme.ReferrerOpts{
			scheme.WithReferrerMatchOpt(referrerMatchOpts),
		}
		if opts.platform != "" {
			referrerOpts = append(referrerOpts, scheme.WithReferrerPlatform(opts.platform))
		}
		if opts.externalRepo != "" {
			rExt, err := ref.New(opts.externalRepo)
			if err != nil {
				return fmt.Errorf("failed to parse external ref: %w", err)
			}
			referrerOpts = append(referrerOpts, scheme.WithReferrerSource(rExt))
			r = rExt
		} else {
			r = rSubject
		}
		rl, err := rc.ReferrerList(ctx, rSubject, referrerOpts...)
		if err != nil {
			return err
		}
		if len(rl.Descriptors) == 0 {
			return fmt.Errorf("no matching referrers to %s", opts.subject)
		} else if len(rl.Descriptors) > 1 && opts.sortAnnot == "" && !opts.latest {
			opts.rootOpts.log.Warn("multiple referrers match, using first match",
				slog.Int("match count", len(rl.Descriptors)),
				slog.String("subject", opts.subject))
		}
		r = r.SetDigest(rl.Descriptors[0].Digest.String())
	} else if len(args) > 0 {
		var err error
		r, err = ref.New(args[0])
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("either a reference or subject must be provided")
	}
	defer rc.Close(ctx, r)

	// pull the manifest
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}
	// lookup descriptor if index / manifest list is returned
	if m.IsList() {
		mi, ok := m.(manifest.Indexer)
		if !ok {
			return fmt.Errorf("manifest list does not support index methods%.0w", errs.ErrUnsupportedMediaType)
		}
		dl, err := mi.GetManifestList()
		if err != nil {
			return fmt.Errorf("failed to get descriptor list: %w", err)
		}
		d, err := descriptor.DescriptorListSearch(dl, matchOpts)
		if err != nil {
			return fmt.Errorf("no matching artifacts found in index: %w", err)
		}
		m, err = rc.ManifestGet(ctx, r, regclient.WithManifestDesc(d))
		if err != nil {
			return err
		}
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		return fmt.Errorf("manifest does not support image methods%.0w", errs.ErrUnsupportedMediaType)
	}

	// if config-file defined, create file as writer, perform a blob get
	if opts.artifactConfig != "" || opts.getConfig {
		d, err := mi.GetConfig()
		if err != nil {
			return err
		}
		rdr, err := rc.BlobGet(ctx, r, d)
		if err != nil {
			return err
		}
		defer rdr.Close()
		if opts.artifactConfig != "" {
			fh, err := os.Create(opts.artifactConfig)
			if err != nil {
				return err
			}
			defer fh.Close()
			_, err = io.Copy(fh, rdr)
			if err != nil {
				return err
			}
		} else {
			_, err = io.Copy(cmd.OutOrStdout(), rdr)
			if err != nil {
				return err
			}
		}
		if opts.getConfig {
			// do not return layer contents if request is only for a config
			return nil
		}
	}

	// get list of layers
	layers, err := mi.GetLayers()
	if err != nil {
		return err
	}
	// filter by media-type if defined
	if len(opts.artifactFileMT) > 0 {
		for i := len(layers) - 1; i >= 0; i-- {
			if !slices.Contains(opts.artifactFileMT, layers[i].MediaType) {
				layers = slices.Delete(layers, i, i+1)
			}
		}
	}
	// filter by filename if defined
	if len(opts.artifactFile) > 0 {
		for i := len(layers) - 1; i >= 0; i-- {
			af, ok := layers[i].Annotations[ociAnnotTitle]
			if !ok || !slices.Contains(opts.artifactFile, af) {
				layers = slices.Delete(layers, i, i+1)
			}
		}
	}

	if len(layers) == 0 {
		return fmt.Errorf("no matching layers found in the artifact, verify media-type and filename%.0w", errs.ErrNotFound)
	}

	if opts.outputDir != "" {
		// loop through each matching layer
		for _, l := range layers {
			if err = l.Digest.Validate(); err != nil {
				return fmt.Errorf("layer contains invalid digest: %s: %w", string(l.Digest), err)
			}
			// wrap in a closure to trigger defer on each step, avoiding open file handles
			err = func() error {
				// perform blob get
				rdr, err := rc.BlobGet(ctx, r, l)
				if err != nil {
					return err
				}
				defer rdr.Close()
				// clean each filename, strip any preceding ..
				f := l.Annotations[ociAnnotTitle]
				if f == "" {
					f = l.Digest.Encoded()
				}
				f = path.Clean("/" + f)
				if strings.HasSuffix(l.Annotations[ociAnnotTitle], "/") || l.Annotations["io.deis.oras.content.unpack"] == "true" {
					f = f + "/"
				}
				if opts.stripDirs {
					f = f[strings.LastIndex(f, "/"):]
				}
				dirs := strings.Split(f, "/")
				// create nested folders if needed
				if len(dirs) > 2 {
					// strip the leading empty dir and trailing filename
					dirs = dirs[1 : len(dirs)-1]
					dest := filepath.Join(opts.outputDir, filepath.Join(dirs...))
					fi, err := os.Stat(dest)
					if os.IsNotExist(err) {
						//#nosec G301 defer to user umask setting, simplifies container scenarios, registry content is often public
						err = os.MkdirAll(dest, 0777)
						if err != nil {
							return err
						}
					} else if err != nil {
						return err
					} else if !fi.IsDir() {
						return fmt.Errorf("destination exists and is not a directory: \"%s\"", dest)
					}
				}
				// if there's a trailing slash, expand the compressed blob into the folder
				if strings.HasSuffix(f, "/") {
					err = archive.Extract(ctx, filepath.Join(opts.outputDir, f), rdr)
					if err != nil {
						return err
					}
				} else {
					// create file as writer
					out := filepath.Join(opts.outputDir, f)
					//#nosec G304 command is run by a user accessing their own files
					fh, err := os.Create(out)
					if err != nil {
						return err
					}
					defer fh.Close()
					_, err = io.Copy(fh, rdr)
					if err != nil {
						return err
					}
				}
				return nil
			}()
			if err != nil {
				return err
			}
		}
	} else {
		// else output dir not defined
		// if more than one matching layer, error
		if len(layers) > 1 {
			return fmt.Errorf("more than one matching layer found, add filters or specify output dir")
		}
		// pull blob, write to stdout
		rdr, err := rc.BlobGet(ctx, r, layers[0])
		if err != nil {
			return err
		}
		defer rdr.Close()
		_, err = io.Copy(cmd.OutOrStdout(), rdr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (opts *artifactOpts) runArtifactList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate inputs
	rSubject, err := ref.New(args[0])
	if err != nil {
		return err
	}
	if opts.latest && opts.sortAnnot != "" {
		return fmt.Errorf("--latest cannot be used with --sort-annotation")
	}
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, rSubject)

	matchOpts := descriptor.MatchOpt{
		ArtifactType:   opts.filterAT,
		SortAnnotation: opts.sortAnnot,
		SortDesc:       opts.sortDesc,
	}
	if opts.filterAnnot != nil {
		matchOpts.Annotations = map[string]string{}
		for _, kv := range opts.filterAnnot {
			kvSplit := strings.SplitN(kv, "=", 2)
			if len(kvSplit) == 2 {
				matchOpts.Annotations[kvSplit[0]] = kvSplit[1]
			} else {
				matchOpts.Annotations[kv] = ""
			}
		}
	}
	if opts.latest {
		matchOpts.SortAnnotation = types.AnnotationCreated
		matchOpts.SortDesc = true
	}
	referrerOpts := []scheme.ReferrerOpts{
		scheme.WithReferrerMatchOpt(matchOpts),
	}
	if opts.platform != "" {
		referrerOpts = append(referrerOpts, scheme.WithReferrerPlatform(opts.platform))
	}
	if opts.externalRepo != "" {
		rExternal, err := ref.New(opts.externalRepo)
		if err != nil {
			return fmt.Errorf("failed to parse external ref: %w", err)
		}
		referrerOpts = append(referrerOpts, scheme.WithReferrerSource(rExternal))
	}

	rl, err := rc.ReferrerList(ctx, rSubject, referrerOpts...)
	if err != nil {
		return err
	}

	// include digest tags if requested
	if opts.digestTags {
		tl, err := rc.TagList(ctx, rSubject)
		if err != nil {
			return fmt.Errorf("failed to list tags: %w", err)
		}
		if rl.Subject.Digest == "" {
			mh, err := rc.ManifestHead(ctx, rl.Subject, regclient.WithManifestRequireDigest())
			if err != nil {
				return fmt.Errorf("failed to get manifest digest: %w", err)
			}
			rl.Subject.Digest = mh.GetDescriptor().Digest.String()
		}
		prefix, err := referrer.FallbackTag(rl.Subject)
		if err != nil {
			return fmt.Errorf("failed to compute fallback tag: %w", err)
		}
		for _, t := range tl.Tags {
			if strings.HasPrefix(t, prefix.Tag) && !slices.Contains(rl.Tags, t) {
				rTag := rl.Subject.SetTag(t)
				mh, err := rc.ManifestHead(ctx, rTag, regclient.WithManifestRequireDigest())
				if err != nil {
					return fmt.Errorf("failed to query digest tag: %w", err)
				}
				desc := mh.GetDescriptor()
				if desc.Annotations == nil {
					desc.Annotations = map[string]string{}
				}
				desc.Annotations[types.AnnotationRefName] = t
				rl.Descriptors = append(rl.Descriptors, desc)
				rl.Tags = append(rl.Tags, t)
			}
		}
	}

	switch opts.format {
	case "raw":
		opts.format = "{{ range $key,$vals := .Manifest.RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .Manifest.RawBody}}"
	case "rawBody", "raw-body", "body":
		opts.format = "{{printf \"%s\" .Manifest.RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		opts.format = "{{ range $key,$vals := .Manifest.RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, rl)
}

func (opts *artifactOpts) runArtifactPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	hasConfig := false
	var r, rArt, rSubject ref.Ref
	var err error

	switch opts.artifactMT {
	case mediatype.OCI1Artifact:
		opts.rootOpts.log.Warn("changing media-type is experimental and non-portable")
		hasConfig = false
	case "", mediatype.OCI1Manifest:
		hasConfig = true
	default:
		return fmt.Errorf("unsupported manifest media type: %s%.0w", opts.artifactMT, errs.ErrUnsupportedMediaType)
	}

	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	// validate inputs
	if opts.refers != "" {
		opts.rootOpts.log.Warn("--refers is deprecated, use --subject instead")
		if opts.subject == "" {
			opts.subject = opts.refers
		}
	}
	if len(args) == 0 && opts.subject == "" {
		return fmt.Errorf("either a reference or subject must be provided")
	}
	if opts.subject != "" {
		rSubject, err = ref.New(opts.subject)
		if err != nil {
			return err
		}
		r = rSubject
	}
	if len(args) > 0 {
		rArt, err = ref.New(args[0])
		if err != nil {
			return err
		}
		r = rArt
	}
	if opts.externalRepo != "" {
		if rSubject.IsZero() {
			return fmt.Errorf("pushing a referrer to an external repository requires a subject%.0w", errs.ErrUnsupported)
		}
		rExt, err := ref.New(opts.externalRepo)
		if err != nil {
			return err
		}
		if rArt.IsSet() && !ref.EqualRepository(rExt, rArt) {
			return fmt.Errorf("push by reference and external to separate repositories is not supported%.0w", errs.ErrUnsupported)
		}
		if !rArt.IsSet() {
			r = rExt
		}
	}
	if !rArt.IsSet() && !rSubject.IsSet() {
		return fmt.Errorf("either a reference or subject must be provided")
	}

	// validate/set artifactType and config.mediaType
	if opts.artifactConfigMT != "" && !mediatype.Valid(opts.artifactConfigMT) {
		return fmt.Errorf("invalid media type: %s%.0w", opts.artifactConfigMT, errs.ErrUnsupportedMediaType)
	}
	if opts.artifactType != "" && !mediatype.Valid(opts.artifactType) {
		return fmt.Errorf("invalid media type: %s%.0w", opts.artifactType, errs.ErrUnsupportedMediaType)
	}
	for _, mt := range opts.artifactFileMT {
		if !mediatype.Valid(mt) {
			return fmt.Errorf("invalid media type: %s%.0w", mt, errs.ErrUnsupportedMediaType)
		}
	}
	if hasConfig && opts.artifactConfigMT == "" {
		if opts.artifactConfig == "" {
			opts.artifactConfigMT = mediatype.OCI1Empty
		} else {
			if opts.artifactType != "" {
				opts.artifactConfigMT = opts.artifactType
				opts.rootOpts.log.Warn("setting config-type using artifact-type")
			} else {
				return fmt.Errorf("config-type is required for config-file")
			}
		}
	}
	if !hasConfig && (opts.artifactConfig != "" || opts.artifactConfigMT != "") {
		return fmt.Errorf("cannot set config-type or config-file on %s%.0w", opts.artifactMT, errs.ErrUnsupportedMediaType)
	}
	if opts.artifactType == "" {
		if !hasConfig || opts.artifactConfigMT == mediatype.OCI1Empty {
			opts.rootOpts.log.Warn("using default value for artifact-type is not recommended")
			opts.artifactType = defaultMTArtifact
		}
	}

	// set and validate artifact files with media types
	if len(opts.artifactFile) <= 1 && len(opts.artifactFileMT) == 0 && opts.artifactType != "" && opts.artifactType != defaultMTArtifact {
		// special case for single file and artifact-type
		opts.artifactFileMT = []string{opts.artifactType}
	} else if len(opts.artifactFile) == 1 && len(opts.artifactFileMT) == 0 {
		// default media-type for a single file, same is used for stdin
		opts.artifactFileMT = []string{defaultMTLayer}
	} else if len(opts.artifactFile) == 0 && len(opts.artifactFileMT) == 1 {
		// no-op, special case for stdin with a media type
	} else if len(opts.artifactFile) != len(opts.artifactFileMT) {
		// all other mis-matches are invalid
		return fmt.Errorf("one artifact media-type must be set for each artifact file")
	}

	// include annotations
	annotations := map[string]string{}
	for _, a := range opts.annotations {
		aSplit := strings.SplitN(a, "=", 2)
		if len(aSplit) == 1 {
			annotations[aSplit[0]] = ""
		} else {
			annotations[aSplit[0]] = aSplit[1]
		}
	}

	// setup regclient
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	var subjectDesc *descriptor.Descriptor
	if rSubject.IsSet() {
		mOpts := []regclient.ManifestOpts{regclient.WithManifestRequireDigest()}
		if opts.platform != "" {
			p, err := platform.Parse(opts.platform)
			if err != nil {
				return fmt.Errorf("failed to parse platform %s: %w", opts.platform, err)
			}
			mOpts = append(mOpts, regclient.WithManifestPlatform(p))
		}
		smh, err := rc.ManifestHead(ctx, rSubject, mOpts...)
		if err != nil {
			return fmt.Errorf("unable to find subject manifest: %w", err)
		}
		d := smh.GetDescriptor()
		subjectDesc = &descriptor.Descriptor{MediaType: d.MediaType, Digest: d.Digest, Size: d.Size}
	}

	// read config, or initialize to an empty json config
	confDesc := descriptor.Descriptor{}
	if hasConfig {
		var configBytes []byte
		var configDigest digest.Digest
		if opts.artifactConfig == "" {
			configBytes = descriptor.EmptyData
			configDigest = descriptor.EmptyDigest
		} else {
			var err error
			configBytes, err = os.ReadFile(opts.artifactConfig)
			if err != nil {
				return err
			}
			configDigest = digest.Canonical.FromBytes(configBytes)
		}
		// push config to registry
		_, err = rc.BlobPut(ctx, r, descriptor.Descriptor{Digest: configDigest, Size: int64(len(configBytes))}, bytes.NewReader(configBytes))
		if err != nil {
			return err
		}
		// save config descriptor to manifest
		confDesc = descriptor.Descriptor{
			MediaType: opts.artifactConfigMT,
			Digest:    configDigest,
			Size:      int64(len(configBytes)),
		}
	}

	blobs := []descriptor.Descriptor{}
	if len(opts.artifactFile) > 0 {
		// if files were passed
		for i, f := range opts.artifactFile {
			// wrap in a closure to trigger defer on each step, avoiding open file handles
			err = func() error {
				mt := opts.artifactFileMT[i]
				openF := f
				// if file is a directory, compress it into a tgz first
				// this unfortunately needs a temp file for the digest
				fi, err := os.Stat(f)
				if err != nil {
					return err
				}
				if fi.IsDir() {
					tf, err := os.CreateTemp("", "regctl-artifact-*.tgz")
					if err != nil {
						return err
					}
					defer tf.Close()
					// change the file being opened to the temp file
					openF = tf.Name()
					defer os.Remove(openF)
					err = archive.Tar(ctx, f, tf, archive.TarCompressGzip)
					if err != nil {
						return err
					}
					if !strings.HasSuffix(f, "/") {
						f = f + "/"
					}
				}
				//#nosec G304 command is run by a user accessing their own files
				rdr, err := os.Open(openF)
				if err != nil {
					return err
				}
				defer rdr.Close()
				// compute digest on file
				desc := descriptor.Descriptor{
					MediaType: mt,
				}
				digester := desc.DigestAlgo().Digester()
				l, err := io.Copy(digester.Hash(), rdr)
				if err != nil {
					return err
				}
				desc.Size = l
				desc.Digest = digester.Digest()
				// add layer to manifest
				if opts.artifactTitle {
					af := f
					if opts.stripDirs {
						fSplit := strings.Split(f, "/")
						if fSplit[len(fSplit)-1] != "" {
							af = fSplit[len(fSplit)-1]
						} else if len(fSplit) > 1 {
							af = fSplit[len(fSplit)-2] + "/"
						}
					}
					desc.Annotations = map[string]string{
						ociAnnotTitle: af,
					}
				}
				blobs = append(blobs, desc)
				// if blob already exists, skip Put
				bRdr, err := rc.BlobHead(ctx, r, desc)
				if err == nil {
					_ = bRdr.Close()
					return nil
				}
				// need to put blob
				_, err = rdr.Seek(0, 0)
				if err != nil {
					return err
				}
				_, err = rc.BlobPut(ctx, r, desc, rdr)
				if err != nil {
					return err
				}
				return nil
			}()
			if err != nil {
				return err
			}
		}
	} else {
		// no files passed, push from stdin
		mt := defaultMTLayer
		if len(opts.artifactFileMT) > 0 {
			mt = opts.artifactFileMT[0]
		}
		d, err := rc.BlobPut(ctx, r, descriptor.Descriptor{}, cmd.InOrStdin())
		if err != nil {
			return err
		}
		d.MediaType = mt
		blobs = append(blobs, d)
	}

	mOpts := []manifest.Opts{}
	switch opts.artifactMT {
	case mediatype.OCI1Artifact:
		m := v1.ArtifactManifest{
			MediaType:    mediatype.OCI1Artifact,
			ArtifactType: opts.artifactType,
			Blobs:        blobs,
			Annotations:  annotations,
			Subject:      subjectDesc,
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	case "", mediatype.OCI1Manifest:
		m := v1.Manifest{
			Versioned:    v1.ManifestSchemaVersion,
			MediaType:    mediatype.OCI1Manifest,
			ArtifactType: opts.artifactType,
			Config:       confDesc,
			Layers:       blobs,
			Annotations:  annotations,
			Subject:      subjectDesc,
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	default:
		return fmt.Errorf("unsupported manifest media type: %s", opts.artifactMT)
	}

	// generate manifest
	mm, err := manifest.New(mOpts...)
	if err != nil {
		return err
	}

	if opts.byDigest || opts.index || rArt.IsZero() {
		r = r.SetDigest(mm.GetDescriptor().Digest.String())
	}

	// push manifest
	putOpts := []regclient.ManifestOpts{}
	if rArt.IsZero() || opts.index {
		putOpts = append(putOpts, regclient.WithManifestChild())
	}
	err = rc.ManifestPut(ctx, r, mm, putOpts...)
	if err != nil {
		return err
	}

	// create/append to index
	if opts.index && rArt.IsSet() {
		// create a descriptor to add
		d := mm.GetDescriptor()
		d.ArtifactType = opts.artifactType
		d.Annotations = annotations
		if opts.platform != "" {
			p, err := platform.Parse(opts.platform)
			if err != nil {
				return fmt.Errorf("failed to parse platform: %w", err)
			}
			d.Platform = &p
		}
		mi, err := rc.ManifestGet(ctx, rArt)
		if err == nil && mi.IsList() {
			// append to existing index
			mii, ok := mi.(manifest.Indexer)
			if !ok {
				return fmt.Errorf("index to append to is a list but not an Indexer?")
			}
			dl, err := mii.GetManifestList()
			if err != nil {
				return err
			}
			dl = append(dl, d)
			err = mii.SetManifestList(dl)
			if err != nil {
				return err
			}
			err = rc.ManifestPut(ctx, rArt, mi)
			if err != nil {
				return err
			}
		} else {
			// create a new index
			mii := v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: mediatype.OCI1ManifestList,
				Manifests: []descriptor.Descriptor{d},
			}
			mi, err := manifest.New(manifest.WithOrig(mii))
			if err != nil {
				return err
			}
			err = rc.ManifestPut(ctx, rArt, mi)
			if err != nil {
				return err
			}
		}
	}

	result := struct {
		Manifest manifest.Manifest
	}{
		Manifest: mm,
	}
	if opts.byDigest && opts.format == "" {
		opts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, result)
}

func (opts *artifactOpts) runArtifactTree(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate inputs
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}
	referrerOpts := []scheme.ReferrerOpts{}
	if opts.filterAT != "" {
		referrerOpts = append(referrerOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: opts.filterAT}))
	}
	if opts.filterAnnot != nil {
		af := map[string]string{}
		for _, kv := range opts.filterAnnot {
			kvSplit := strings.SplitN(kv, "=", 2)
			if len(kvSplit) == 2 {
				af[kvSplit[0]] = kvSplit[1]
			} else {
				af[kv] = ""
			}
		}
		referrerOpts = append(referrerOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: af}))
	}
	rRefSrc := r
	if opts.externalRepo != "" {
		rRefSrc, err = ref.New(opts.externalRepo)
		if err != nil {
			return fmt.Errorf("failed to parse external ref: %w", err)
		}
		referrerOpts = append(referrerOpts, scheme.WithReferrerSource(rRefSrc))
	}

	// include digest tags if requested
	tags := []string{}
	if opts.digestTags {
		tl, err := rc.TagList(ctx, r)
		if err != nil {
			return fmt.Errorf("failed to list tags: %w", err)
		}
		tags = tl.Tags
	}

	seen := []string{}
	tr, err := opts.treeAddResult(ctx, rc, r, seen, referrerOpts, tags)
	var twErr error
	if tr != nil {
		twErr = template.Writer(cmd.OutOrStdout(), opts.format, tr)
	}
	if err != nil {
		return err
	}
	return twErr
}

func (opts *artifactOpts) treeAddResult(ctx context.Context, rc *regclient.RegClient, r ref.Ref, seen []string, rOpts []scheme.ReferrerOpts, tags []string) (*treeResult, error) {
	tr := treeResult{
		Ref: r,
	}

	// get manifest
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return nil, err
	}
	tr.Manifest = m
	r = r.AddDigest(m.GetDescriptor().Digest.String())

	// track already seen manifests
	dig := m.GetDescriptor().Digest.String()
	if slices.Contains(seen, dig) {
		return &tr, fmt.Errorf("%w, already processed %s", ErrLoopEncountered, dig)
	}
	seen = append(seen, dig)

	// get child nodes
	if m.IsList() {
		tr.Child = []*treeResult{}
		mi, ok := m.(manifest.Indexer)
		if !ok {
			return &tr, fmt.Errorf("failed to convert a manifest list to indexer for %s", r.CommonName())
		}
		dl, err := mi.GetManifestList()
		if err != nil {
			return &tr, fmt.Errorf("failed to get platforms for %s: %w", r.CommonName(), err)
		}
		for _, d := range dl {
			rChild := r.SetDigest(d.Digest.String())
			tChild, err := opts.treeAddResult(ctx, rc, rChild, seen, rOpts, tags)
			if tChild != nil {
				tChild.ArtifactType = d.ArtifactType
				if d.Platform != nil {
					pCopy := *d.Platform
					tChild.Platform = &pCopy
				}
				tr.Child = append(tr.Child, tChild)
			}
			if err != nil {
				return &tr, err
			}
		}
	}

	// get referrers
	rl, err := rc.ReferrerList(ctx, r, rOpts...)
	if err != nil {
		return &tr, fmt.Errorf("failed to check referrers for %s: %w", r.CommonName(), err)
	}
	if len(rl.Descriptors) > 0 {
		var rReferrer ref.Ref
		if rl.Source.IsSet() {
			rReferrer = rl.Source
		} else {
			rReferrer = rl.Subject
		}
		tr.Referrer = []*treeResult{}
		for _, d := range rl.Descriptors {
			rReferrer = rReferrer.SetDigest(d.Digest.String())
			tReferrer, err := opts.treeAddResult(ctx, rc, rReferrer, seen, rOpts, tags)
			if tReferrer != nil {
				tReferrer.ArtifactType = d.ArtifactType
				if d.Platform != nil {
					pCopy := *d.Platform
					tReferrer.Platform = &pCopy
				}
				tr.Referrer = append(tr.Referrer, tReferrer)
			}
			if err != nil {
				return &tr, err
			}
		}
	}

	// include digest tags if requested
	if opts.digestTags {
		prefix, err := referrer.FallbackTag(r)
		if err != nil {
			return &tr, fmt.Errorf("failed to compute fallback tag: %w", err)
		}
		for _, t := range tags {
			if strings.HasPrefix(t, prefix.Tag) && !slices.Contains(rl.Tags, t) {
				rTag := r.SetTag(t)
				tReferrer, err := opts.treeAddResult(ctx, rc, rTag, seen, rOpts, tags)
				if tReferrer != nil {
					tReferrer.Ref = tReferrer.Ref.SetTag(t)
					tr.Referrer = append(tr.Referrer, tReferrer)
				}
				if err != nil {
					return &tr, err
				}
			}
		}
	}

	return &tr, nil
}

type treeResult struct {
	Ref          ref.Ref            `json:"reference"`
	Manifest     manifest.Manifest  `json:"manifest"`
	Platform     *platform.Platform `json:"platform,omitempty"`
	ArtifactType string             `json:"artifactType,omitempty"`
	Child        []*treeResult      `json:"child,omitempty"`
	Referrer     []*treeResult      `json:"referrer,omitempty"`
	ReferrerSrc  ref.Ref            `json:"referrerSource"`
}

func (tr *treeResult) MarshalPretty() ([]byte, error) {
	mp, err := tr.marshalPretty("")
	if err != nil {
		return nil, err
	}
	return fmt.Appendf(nil, "Ref: %s\nDigest: %s", tr.Ref.CommonName(), mp), nil
}

func (tr *treeResult) marshalPretty(indent string) ([]byte, error) {
	result := bytes.NewBufferString("")
	_, err := result.WriteString(tr.Manifest.GetDescriptor().Digest.String())
	if err != nil {
		return nil, err
	}
	if tr.Platform != nil {
		_, err = result.WriteString(" [" + tr.Platform.String() + "]")
		if err != nil {
			return nil, err
		}
	}
	if tr.ArtifactType != "" {
		_, err = result.WriteString(": " + tr.ArtifactType)
		if err != nil {
			return nil, err
		}
	}
	if tr.ArtifactType == "" && strings.HasPrefix(tr.Ref.Tag, "sha256-") {
		_, err = result.WriteString(": " + tr.Ref.Tag)
		if err != nil {
			return nil, err
		}
	}
	_, err = result.WriteString("\n")
	if err != nil {
		return nil, err
	}
	if len(tr.Child) > 0 {
		_, err = result.WriteString(indent + "Children:\n")
		if err != nil {
			return nil, err
		}
		for _, trChild := range tr.Child {
			_, err = result.WriteString(indent + "  - ")
			if err != nil {
				return nil, err
			}
			childBytes, err := trChild.marshalPretty(indent + "    ")
			if err != nil {
				return nil, err
			}
			_, err = result.Write(childBytes)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(tr.Referrer) > 0 {
		_, err = result.WriteString(indent + "Referrers:\n")
		if err != nil {
			return nil, err
		}
		for _, trReferrer := range tr.Referrer {
			_, err = result.WriteString(indent + "  - ")
			if err != nil {
				return nil, err
			}
			referrerBytes, err := trReferrer.marshalPretty(indent + "    ")
			if err != nil {
				return nil, err
			}
			_, err = result.Write(referrerBytes)
			if err != nil {
				return nil, err
			}
		}
	}
	return result.Bytes(), nil
}
