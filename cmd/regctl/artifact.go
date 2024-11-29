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

type artifactCmd struct {
	rootOpts         *rootCmd
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
	formatList       string
	formatPut        string
	formatTree       string
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

func NewArtifactCmd(rootOpts *rootCmd) *cobra.Command {
	artifactOpts := artifactCmd{
		rootOpts: rootOpts,
	}

	var artifactTopCmd = &cobra.Command{
		Use:   "artifact <cmd>",
		Short: "manage artifacts",
	}
	var artifactGetCmd = &cobra.Command{
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
		RunE:      artifactOpts.runArtifactGet,
	}
	var artifactListCmd = &cobra.Command{
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
		RunE:      artifactOpts.runArtifactList,
	}
	var artifactPutCmd = &cobra.Command{
		Use:     "put <reference>",
		Aliases: []string{"push"},
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
		RunE:      artifactOpts.runArtifactPut,
	}
	var artifactTreeCmd = &cobra.Command{
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
		RunE:      artifactOpts.runArtifactTree,
	}

	artifactGetCmd.Flags().StringVar(&artifactOpts.subject, "subject", "", "Get a referrer to the subject reference")
	artifactGetCmd.Flags().StringVar(&artifactOpts.externalRepo, "external", "", "Query referrers from a separate source")
	artifactGetCmd.Flags().StringVarP(&artifactOpts.platform, "platform", "p", "", "Specify platform of a subject (e.g. linux/amd64 or local)")
	artifactGetCmd.Flags().StringVar(&artifactOpts.filterAT, "filter-artifact-type", "", "Filter referrers by artifactType")
	artifactGetCmd.Flags().StringArrayVar(&artifactOpts.filterAnnot, "filter-annotation", []string{}, "Filter referrers by annotation (key=value)")
	artifactGetCmd.Flags().BoolVar(&artifactOpts.getConfig, "config", false, "Show the config, overrides file options")
	artifactGetCmd.Flags().StringVar(&artifactOpts.artifactConfig, "config-file", "", "Output config to a file")
	artifactGetCmd.Flags().StringArrayVarP(&artifactOpts.artifactFile, "file", "f", []string{}, "Filter by artifact filename")
	artifactGetCmd.Flags().StringArrayVarP(&artifactOpts.artifactFileMT, "file-media-type", "m", []string{}, "Filter by artifact media-type")
	_ = artifactGetCmd.RegisterFlagCompletionFunc("file-media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return artifactFileKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	artifactGetCmd.Flags().BoolVar(&artifactOpts.latest, "latest", false, "Get the most recent referrer using the OCI created annotation")
	artifactGetCmd.Flags().StringVarP(&artifactOpts.outputDir, "output", "o", "", "Output directory for multiple artifacts")
	artifactGetCmd.Flags().BoolVar(&artifactOpts.stripDirs, "strip-dirs", false, "Strip directories from filenames in output dir")
	artifactGetCmd.Flags().StringVar(&artifactOpts.refers, "refers", "", "Deprecated: Get a referrer to the reference")
	_ = artifactGetCmd.Flags().MarkHidden("refers")
	artifactGetCmd.Flags().StringVar(&artifactOpts.sortAnnot, "sort-annotation", "", "Annotation used for sorting results")
	artifactGetCmd.Flags().BoolVar(&artifactOpts.sortDesc, "sort-desc", false, "Sort in descending order")

	artifactListCmd.Flags().BoolVar(&artifactOpts.digestTags, "digest-tags", false, "Include digest tags")
	artifactListCmd.Flags().StringVar(&artifactOpts.externalRepo, "external", "", "Query referrers from a separate source")
	artifactListCmd.Flags().StringVar(&artifactOpts.filterAT, "filter-artifact-type", "", "Filter descriptors by artifactType")
	artifactListCmd.Flags().StringArrayVar(&artifactOpts.filterAnnot, "filter-annotation", []string{}, "Filter descriptors by annotation (key=value)")
	artifactListCmd.Flags().StringVar(&artifactOpts.formatList, "format", "{{printPretty .}}", "Format output with go template syntax")
	artifactListCmd.Flags().BoolVar(&artifactOpts.latest, "latest", false, "Sort using the OCI created annotation")
	artifactListCmd.Flags().StringVarP(&artifactOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	artifactListCmd.Flags().StringVar(&artifactOpts.sortAnnot, "sort-annotation", "", "Annotation used for sorting results")
	artifactListCmd.Flags().BoolVar(&artifactOpts.sortDesc, "sort-desc", false, "Sort in descending order")

	artifactPutCmd.Flags().StringVarP(&artifactOpts.artifactMT, "media-type", "", mediatype.OCI1Manifest, "EXPERIMENTAL: Manifest media-type")
	_ = artifactPutCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return manifestKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	_ = artifactPutCmd.Flags().MarkHidden("media-type")
	artifactPutCmd.Flags().StringVar(&artifactOpts.artifactType, "artifact-type", "", "Artifact type (recommended)")
	_ = artifactPutCmd.RegisterFlagCompletionFunc("artifact-type", completeArgNone)
	artifactPutCmd.Flags().StringVar(&artifactOpts.artifactConfig, "config-file", "", "Filename for config content")
	artifactPutCmd.Flags().StringVar(&artifactOpts.artifactConfigMT, "config-type", "", "Config mediaType")
	_ = artifactPutCmd.RegisterFlagCompletionFunc("config-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return configKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	artifactPutCmd.Flags().StringArrayVarP(&artifactOpts.artifactFile, "file", "f", []string{}, "Artifact filename")
	artifactPutCmd.Flags().StringArrayVarP(&artifactOpts.artifactFileMT, "file-media-type", "m", []string{}, "Set the mediaType for the individual files")
	_ = artifactPutCmd.RegisterFlagCompletionFunc("file-media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return artifactFileKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	artifactPutCmd.Flags().BoolVar(&artifactOpts.artifactTitle, "file-title", false, "Include a title annotation with the filename")
	artifactPutCmd.Flags().StringArrayVar(&artifactOpts.annotations, "annotation", []string{}, "Annotation to include on manifest")
	artifactPutCmd.Flags().BoolVar(&artifactOpts.byDigest, "by-digest", false, "Push manifest by digest instead of tag")
	artifactPutCmd.Flags().StringVar(&artifactOpts.formatPut, "format", "", "Format output with go template syntax")
	artifactPutCmd.Flags().BoolVar(&artifactOpts.index, "index", false, "Create/append artifact to an index")
	artifactPutCmd.Flags().StringVar(&artifactOpts.subject, "subject", "", "Set the subject to a reference (used for referrer queries)")
	artifactPutCmd.Flags().BoolVar(&artifactOpts.stripDirs, "strip-dirs", false, "Strip directories from filenames in file-title")
	artifactPutCmd.Flags().StringVarP(&artifactOpts.platform, "platform", "p", "", "Specify platform of a subject (e.g. linux/amd64 or local)")
	artifactPutCmd.Flags().StringVar(&artifactOpts.refers, "refers", "", "EXPERIMENTAL: Set a referrer to the reference")
	_ = artifactPutCmd.Flags().MarkHidden("refers")

	artifactTreeCmd.Flags().BoolVar(&artifactOpts.digestTags, "digest-tags", false, "Include digest tags")
	artifactTreeCmd.Flags().StringVar(&artifactOpts.externalRepo, "external", "", "Query referrers from a separate source")
	artifactTreeCmd.Flags().StringVar(&artifactOpts.filterAT, "filter-artifact-type", "", "Filter descriptors by artifactType")
	artifactTreeCmd.Flags().StringArrayVar(&artifactOpts.filterAnnot, "filter-annotation", []string{}, "Filter descriptors by annotation (key=value)")
	artifactTreeCmd.Flags().StringVar(&artifactOpts.formatTree, "format", "{{printPretty .}}", "Format output with go template syntax")

	artifactTopCmd.AddCommand(artifactGetCmd)
	artifactTopCmd.AddCommand(artifactListCmd)
	artifactTopCmd.AddCommand(artifactPutCmd)
	artifactTopCmd.AddCommand(artifactTreeCmd)
	return artifactTopCmd
}

func (artifactOpts *artifactCmd) runArtifactGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	rc := artifactOpts.rootOpts.newRegClient()

	// validate inputs
	if artifactOpts.refers != "" {
		artifactOpts.rootOpts.log.Warn("--refers is deprecated, use --subject instead")
		if artifactOpts.subject == "" {
			artifactOpts.subject = artifactOpts.refers
		}
	}
	if artifactOpts.externalRepo != "" && artifactOpts.subject == "" {
		artifactOpts.rootOpts.log.Warn("--external option depends on --subject")
	}
	if artifactOpts.latest && artifactOpts.sortAnnot != "" {
		return fmt.Errorf("--latest cannot be used with --sort-annotation")
	}
	// if output dir defined, ensure it exists
	if artifactOpts.outputDir != "" {
		fi, err := os.Stat(artifactOpts.outputDir)
		if err != nil {
			return fmt.Errorf("output directory unavailable: %w", err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("output must be a directory: \"%s\"", artifactOpts.outputDir)
		}
	}
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	r := ref.Ref{}
	matchOpts := descriptor.MatchOpt{
		ArtifactType:   artifactOpts.filterAT,
		SortAnnotation: artifactOpts.sortAnnot,
		SortDesc:       artifactOpts.sortDesc,
	}
	if artifactOpts.filterAnnot != nil {
		matchOpts.Annotations = map[string]string{}
		for _, kv := range artifactOpts.filterAnnot {
			kvSplit := strings.SplitN(kv, "=", 2)
			if len(kvSplit) == 2 {
				matchOpts.Annotations[kvSplit[0]] = kvSplit[1]
			} else {
				matchOpts.Annotations[kv] = ""
			}
		}
	}
	if artifactOpts.latest {
		matchOpts.SortAnnotation = types.AnnotationCreated
		matchOpts.SortDesc = true
	}
	if artifactOpts.platform != "" {
		p, err := platform.Parse(artifactOpts.platform)
		if err != nil {
			return fmt.Errorf("platform could not be parsed: %w", err)
		}
		matchOpts.Platform = &p
	}

	// lookup referrers to the subject
	if len(args) == 0 && artifactOpts.subject != "" {
		rSubject, err := ref.New(artifactOpts.subject)
		if err != nil {
			return err
		}
		referrerMatchOpts := matchOpts
		referrerMatchOpts.Platform = nil
		referrerOpts := []scheme.ReferrerOpts{
			scheme.WithReferrerMatchOpt(referrerMatchOpts),
		}
		if artifactOpts.platform != "" {
			referrerOpts = append(referrerOpts, scheme.WithReferrerPlatform(artifactOpts.platform))
		}
		if artifactOpts.externalRepo != "" {
			rExt, err := ref.New(artifactOpts.externalRepo)
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
			return fmt.Errorf("no matching referrers to %s", artifactOpts.subject)
		} else if len(rl.Descriptors) > 1 && artifactOpts.sortAnnot == "" && !artifactOpts.latest {
			artifactOpts.rootOpts.log.Warn("multiple referrers match, using first match",
				slog.Int("match count", len(rl.Descriptors)),
				slog.String("subject", artifactOpts.subject))
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
		r.Digest = d.Digest.String()
		m, err = rc.ManifestGet(ctx, r)
		if err != nil {
			return err
		}
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		return fmt.Errorf("manifest does not support image methods%.0w", errs.ErrUnsupportedMediaType)
	}

	// if config-file defined, create file as writer, perform a blob get
	if artifactOpts.artifactConfig != "" || artifactOpts.getConfig {
		d, err := mi.GetConfig()
		if err != nil {
			return err
		}
		rdr, err := rc.BlobGet(ctx, r, d)
		if err != nil {
			return err
		}
		defer rdr.Close()
		if artifactOpts.artifactConfig != "" {
			fh, err := os.Create(artifactOpts.artifactConfig)
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
		if artifactOpts.getConfig {
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
	if len(artifactOpts.artifactFileMT) > 0 {
		for i := len(layers) - 1; i >= 0; i-- {
			found := false
			for _, mt := range artifactOpts.artifactFileMT {
				if layers[i].MediaType == mt {
					found = true
					break
				}
			}
			if !found {
				// remove from slice
				layers = append(layers[:i], layers[i+1:]...)
			}
		}
	}
	// filter by filename if defined
	if len(artifactOpts.artifactFile) > 0 {
		for i := len(layers) - 1; i >= 0; i-- {
			found := false
			af, ok := layers[i].Annotations[ociAnnotTitle]
			if ok {
				for _, f := range artifactOpts.artifactFile {
					if af == f {
						found = true
						break
					}
				}
			}
			if !found {
				// remove from slice
				layers = append(layers[:i], layers[i+1:]...)
			}
		}
	}

	if len(layers) == 0 {
		return fmt.Errorf("no matching layers found in the artifact, verify media-type and filename")
	}

	if artifactOpts.outputDir != "" {
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
				if artifactOpts.stripDirs {
					f = f[strings.LastIndex(f, "/"):]
				}
				dirs := strings.Split(f, "/")
				// create nested folders if needed
				if len(dirs) > 2 {
					// strip the leading empty dir and trailing filename
					dirs = dirs[1 : len(dirs)-1]
					dest := filepath.Join(artifactOpts.outputDir, filepath.Join(dirs...))
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
					err = archive.Extract(ctx, filepath.Join(artifactOpts.outputDir, f), rdr)
					if err != nil {
						return err
					}
				} else {
					// create file as writer
					out := filepath.Join(artifactOpts.outputDir, f)
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

func (artifactOpts *artifactCmd) runArtifactList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate inputs
	rSubject, err := ref.New(args[0])
	if err != nil {
		return err
	}
	if artifactOpts.latest && artifactOpts.sortAnnot != "" {
		return fmt.Errorf("--latest cannot be used with --sort-annotation")
	}
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	rc := artifactOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, rSubject)

	matchOpts := descriptor.MatchOpt{
		ArtifactType:   artifactOpts.filterAT,
		SortAnnotation: artifactOpts.sortAnnot,
		SortDesc:       artifactOpts.sortDesc,
	}
	if artifactOpts.filterAnnot != nil {
		matchOpts.Annotations = map[string]string{}
		for _, kv := range artifactOpts.filterAnnot {
			kvSplit := strings.SplitN(kv, "=", 2)
			if len(kvSplit) == 2 {
				matchOpts.Annotations[kvSplit[0]] = kvSplit[1]
			} else {
				matchOpts.Annotations[kv] = ""
			}
		}
	}
	if artifactOpts.latest {
		matchOpts.SortAnnotation = types.AnnotationCreated
		matchOpts.SortDesc = true
	}
	referrerOpts := []scheme.ReferrerOpts{
		scheme.WithReferrerMatchOpt(matchOpts),
	}
	if artifactOpts.platform != "" {
		referrerOpts = append(referrerOpts, scheme.WithReferrerPlatform(artifactOpts.platform))
	}
	if artifactOpts.externalRepo != "" {
		rExternal, err := ref.New(artifactOpts.externalRepo)
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
	if artifactOpts.digestTags {
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
			if strings.HasPrefix(t, prefix.Tag) && !sliceHasStr(rl.Tags, t) {
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

	switch artifactOpts.formatList {
	case "raw":
		artifactOpts.formatList = "{{ range $key,$vals := .Manifest.RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .Manifest.RawBody}}"
	case "rawBody", "raw-body", "body":
		artifactOpts.formatList = "{{printf \"%s\" .Manifest.RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		artifactOpts.formatList = "{{ range $key,$vals := .Manifest.RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), artifactOpts.formatList, rl)
}

func (artifactOpts *artifactCmd) runArtifactPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	hasConfig := false
	var r, rArt, rSubject ref.Ref
	var err error

	switch artifactOpts.artifactMT {
	case mediatype.OCI1Artifact:
		artifactOpts.rootOpts.log.Warn("changing media-type is experimental and non-portable")
		hasConfig = false
	case "", mediatype.OCI1Manifest:
		hasConfig = true
	default:
		return fmt.Errorf("unsupported manifest media type: %s%.0w", artifactOpts.artifactMT, errs.ErrUnsupportedMediaType)
	}

	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	// validate inputs
	if artifactOpts.refers != "" {
		artifactOpts.rootOpts.log.Warn("--refers is deprecated, use --subject instead")
		if artifactOpts.subject == "" {
			artifactOpts.subject = artifactOpts.refers
		}
	}
	if len(args) == 0 && artifactOpts.subject == "" {
		return fmt.Errorf("either a reference or subject must be provided")
	}
	if artifactOpts.subject != "" {
		rSubject, err = ref.New(artifactOpts.subject)
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
	if !rArt.IsSet() && !rSubject.IsSet() {
		return fmt.Errorf("either a reference or subject must be provided")
	}

	// validate/set artifactType and config.mediaType
	if artifactOpts.artifactConfigMT != "" && !mediatype.Valid(artifactOpts.artifactConfigMT) {
		return fmt.Errorf("invalid media type: %s%.0w", artifactOpts.artifactConfigMT, errs.ErrUnsupportedMediaType)
	}
	if artifactOpts.artifactType != "" && !mediatype.Valid(artifactOpts.artifactType) {
		return fmt.Errorf("invalid media type: %s%.0w", artifactOpts.artifactType, errs.ErrUnsupportedMediaType)
	}
	for _, mt := range artifactOpts.artifactFileMT {
		if !mediatype.Valid(mt) {
			return fmt.Errorf("invalid media type: %s%.0w", mt, errs.ErrUnsupportedMediaType)
		}
	}
	if hasConfig && artifactOpts.artifactConfigMT == "" {
		if artifactOpts.artifactConfig == "" {
			artifactOpts.artifactConfigMT = mediatype.OCI1Empty
		} else {
			if artifactOpts.artifactType != "" {
				artifactOpts.artifactConfigMT = artifactOpts.artifactType
				artifactOpts.rootOpts.log.Warn("setting config-type using artifact-type")
			} else {
				return fmt.Errorf("config-type is required for config-file")
			}
		}
	}
	if !hasConfig && (artifactOpts.artifactConfig != "" || artifactOpts.artifactConfigMT != "") {
		return fmt.Errorf("cannot set config-type or config-file on %s%.0w", artifactOpts.artifactMT, errs.ErrUnsupportedMediaType)
	}
	if artifactOpts.artifactType == "" {
		if !hasConfig || artifactOpts.artifactConfigMT == mediatype.OCI1Empty {
			artifactOpts.rootOpts.log.Warn("using default value for artifact-type is not recommended")
			artifactOpts.artifactType = defaultMTArtifact
		}
	}

	// set and validate artifact files with media types
	if len(artifactOpts.artifactFile) <= 1 && len(artifactOpts.artifactFileMT) == 0 && artifactOpts.artifactType != "" && artifactOpts.artifactType != defaultMTArtifact {
		// special case for single file and artifact-type
		artifactOpts.artifactFileMT = []string{artifactOpts.artifactType}
	} else if len(artifactOpts.artifactFile) == 1 && len(artifactOpts.artifactFileMT) == 0 {
		// default media-type for a single file, same is used for stdin
		artifactOpts.artifactFileMT = []string{defaultMTLayer}
	} else if len(artifactOpts.artifactFile) == 0 && len(artifactOpts.artifactFileMT) == 1 {
		// no-op, special case for stdin with a media type
	} else if len(artifactOpts.artifactFile) != len(artifactOpts.artifactFileMT) {
		// all other mis-matches are invalid
		return fmt.Errorf("one artifact media-type must be set for each artifact file")
	}

	// include annotations
	annotations := map[string]string{}
	for _, a := range artifactOpts.annotations {
		aSplit := strings.SplitN(a, "=", 2)
		if len(aSplit) == 1 {
			annotations[aSplit[0]] = ""
		} else {
			annotations[aSplit[0]] = aSplit[1]
		}
	}

	// setup regclient
	rc := artifactOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	var subjectDesc *descriptor.Descriptor
	if rSubject.IsSet() {
		mOpts := []regclient.ManifestOpts{regclient.WithManifestRequireDigest()}
		if artifactOpts.platform != "" {
			p, err := platform.Parse(artifactOpts.platform)
			if err != nil {
				return fmt.Errorf("failed to parse platform %s: %w", artifactOpts.platform, err)
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
		if artifactOpts.artifactConfig == "" {
			configBytes = descriptor.EmptyData
			configDigest = descriptor.EmptyDigest
		} else {
			var err error
			configBytes, err = os.ReadFile(artifactOpts.artifactConfig)
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
			MediaType: artifactOpts.artifactConfigMT,
			Digest:    configDigest,
			Size:      int64(len(configBytes)),
		}
	}

	blobs := []descriptor.Descriptor{}
	if len(artifactOpts.artifactFile) > 0 {
		// if files were passed
		for i, f := range artifactOpts.artifactFile {
			// wrap in a closure to trigger defer on each step, avoiding open file handles
			err = func() error {
				mt := artifactOpts.artifactFileMT[i]
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
				if artifactOpts.artifactTitle {
					af := f
					if artifactOpts.stripDirs {
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
		if len(artifactOpts.artifactFileMT) > 0 {
			mt = artifactOpts.artifactFileMT[0]
		}
		d, err := rc.BlobPut(ctx, r, descriptor.Descriptor{}, cmd.InOrStdin())
		if err != nil {
			return err
		}
		d.MediaType = mt
		blobs = append(blobs, d)
	}

	mOpts := []manifest.Opts{}
	switch artifactOpts.artifactMT {
	case mediatype.OCI1Artifact:
		m := v1.ArtifactManifest{
			MediaType:    mediatype.OCI1Artifact,
			ArtifactType: artifactOpts.artifactType,
			Blobs:        blobs,
			Annotations:  annotations,
			Subject:      subjectDesc,
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	case "", mediatype.OCI1Manifest:
		m := v1.Manifest{
			Versioned:    v1.ManifestSchemaVersion,
			MediaType:    mediatype.OCI1Manifest,
			ArtifactType: artifactOpts.artifactType,
			Config:       confDesc,
			Layers:       blobs,
			Annotations:  annotations,
			Subject:      subjectDesc,
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	default:
		return fmt.Errorf("unsupported manifest media type: %s", artifactOpts.artifactMT)
	}

	// generate manifest
	mm, err := manifest.New(mOpts...)
	if err != nil {
		return err
	}

	if artifactOpts.byDigest || artifactOpts.index || rArt.IsZero() {
		r.Tag = ""
		r.Digest = mm.GetDescriptor().Digest.String()
	}

	// push manifest
	putOpts := []regclient.ManifestOpts{}
	if rArt.IsZero() || artifactOpts.index {
		putOpts = append(putOpts, regclient.WithManifestChild())
	}
	err = rc.ManifestPut(ctx, r, mm, putOpts...)
	if err != nil {
		return err
	}

	// create/append to index
	if artifactOpts.index && rArt.IsSet() {
		// create a descriptor to add
		d := mm.GetDescriptor()
		d.ArtifactType = artifactOpts.artifactType
		d.Annotations = annotations
		if artifactOpts.platform != "" {
			p, err := platform.Parse(artifactOpts.platform)
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
	if artifactOpts.byDigest && artifactOpts.formatPut == "" {
		artifactOpts.formatPut = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), artifactOpts.formatPut, result)
}

func (artifactOpts *artifactCmd) runArtifactTree(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate inputs
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	rc := artifactOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}
	referrerOpts := []scheme.ReferrerOpts{}
	if artifactOpts.filterAT != "" {
		referrerOpts = append(referrerOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: artifactOpts.filterAT}))
	}
	if artifactOpts.filterAnnot != nil {
		af := map[string]string{}
		for _, kv := range artifactOpts.filterAnnot {
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
	if artifactOpts.externalRepo != "" {
		rRefSrc, err = ref.New(artifactOpts.externalRepo)
		if err != nil {
			return fmt.Errorf("failed to parse external ref: %w", err)
		}
		referrerOpts = append(referrerOpts, scheme.WithReferrerSource(rRefSrc))
	}

	// include digest tags if requested
	tags := []string{}
	if artifactOpts.digestTags {
		tl, err := rc.TagList(ctx, r)
		if err != nil {
			return fmt.Errorf("failed to list tags: %w", err)
		}
		tags = tl.Tags
	}

	seen := []string{}
	tr, err := artifactOpts.treeAddResult(ctx, rc, r, rRefSrc, seen, referrerOpts, tags)
	var twErr error
	if tr != nil {
		twErr = template.Writer(cmd.OutOrStdout(), artifactOpts.formatTree, tr)
	}
	if err != nil {
		return err
	}
	return twErr
}

func (artifactOpts *artifactCmd) treeAddResult(ctx context.Context, rc *regclient.RegClient, r, rRefSrc ref.Ref, seen []string, rOpts []scheme.ReferrerOpts, tags []string) (*treeResult, error) {
	tr := treeResult{
		Ref: r,
	}

	// get manifest
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return nil, err
	}
	tr.Manifest = m
	if r.Digest == "" {
		r.Digest = m.GetDescriptor().Digest.String()
	}

	// track already seen manifests
	dig := m.GetDescriptor().Digest.String()
	if sliceHasStr(seen, dig) {
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
			tChild, err := artifactOpts.treeAddResult(ctx, rc, rChild, rRefSrc, seen, rOpts, tags)
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
		tr.Referrer = []*treeResult{}
		for _, d := range rl.Descriptors {
			rReferrer := rRefSrc.SetDigest(d.Digest.String())
			tReferrer, err := artifactOpts.treeAddResult(ctx, rc, rReferrer, rRefSrc, seen, rOpts, tags)
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
	if artifactOpts.digestTags {
		prefix, err := referrer.FallbackTag(r)
		if err != nil {
			return &tr, fmt.Errorf("failed to compute fallback tag: %w", err)
		}
		for _, t := range tags {
			if strings.HasPrefix(t, prefix.Tag) && !sliceHasStr(rl.Tags, t) {
				rTag := r.SetTag(t)
				tReferrer, err := artifactOpts.treeAddResult(ctx, rc, rTag, rRefSrc, seen, rOpts, tags)
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

func sliceHasStr(list []string, search string) bool {
	for _, el := range list {
		if el == search {
			return true
		}
	}
	return false
}

type treeResult struct {
	Ref          ref.Ref            `json:"reference"`
	Manifest     manifest.Manifest  `json:"manifest"`
	Platform     *platform.Platform `json:"platform,omitempty"`
	ArtifactType string             `json:"artifactType,omitempty"`
	Child        []*treeResult      `json:"child,omitempty"`
	Referrer     []*treeResult      `json:"referrer,omitempty"`
}

func (tr *treeResult) MarshalPretty() ([]byte, error) {
	mp, err := tr.marshalPretty("")
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("Ref: %s\nDigest: %s", tr.Ref.CommonName(), mp)), nil
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
