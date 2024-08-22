package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/internal/diff"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/warning"
)

type manifestCmd struct {
	rootOpts      *rootCmd
	byDigest      bool
	contentType   string
	diffCtx       int
	diffFullCtx   bool
	forceTagDeref bool
	formatGet     string
	formatHead    string
	formatPut     string
	list          bool
	platform      string
	referrers     bool
	requireDigest bool
	requireList   bool
}

func NewManifestCmd(rootOpts *rootCmd) *cobra.Command {
	manifestOpts := manifestCmd{
		rootOpts: rootOpts,
	}
	var manifestTopCmd = &cobra.Command{
		Use:   "manifest <cmd>",
		Short: "manage manifests",
	}

	var manifestDeleteCmd = &cobra.Command{
		Use:     "delete <image_ref>",
		Aliases: []string{"del", "rm", "remove"},
		Short:   "delete a manifest",
		Long: `Delete a manifest. This will delete the manifest, and all tags pointing to that
manifest. You must specify a digest, not a tag on this command (e.g. 
image_name@sha256:1234abc...). It is up to the registry whether the delete
API is supported. Additionally, registries may garbage collect the filesystem
layers (blobs) separately or not at all. See also the "tag delete" command.`,
		Example: `
# delete a manifest by digest
regctl manifest delete registry.example.org/repo@sha256:fab3c890d0480549d05d2ff3d746f42e360b7f0e3fe64bdf39fc572eab94911b

# delete the digest referenced by a tag (this is unsafe)
regctl manifest delete registry.example.org/repo:v1.2.3 --force-tag-dereference

# delete the digest and all manifests with a subject referencing the digest
regctl manifest delete --referrers \
  registry.example.org/repo@sha256:fab3c890d0480549d05d2ff3d746f42e360b7f0e3fe64bdf39fc572eab94911b`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete digests
		RunE:      manifestOpts.runManifestDelete,
	}

	var manifestDiffCmd = &cobra.Command{
		Use:   "diff <image_ref> <image_ref>",
		Short: "compare manifests",
		Long:  `Show the differences between two image manifests`,
		Example: `
# compare the scratch and alpine images
regctl manifest diff \
  ghcr.io/regclient/regctl:latest \
	ghcr.io/regclient/regctl:alpine

# compare two digests and show the full context
regctl manifest diff --context-full \
  ghcr.io/regclient/regctl@sha256:9b7057d06ce061cefc7a0b7cb28cad626164e6629a1a4f09cee4b4d400c9aef0 \
  ghcr.io/regclient/regctl@sha256:4d113b278bd425d094848ba5d7b4d6baca13a2a9d20d265b32bc12020d501002`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              manifestOpts.runManifestDiff,
	}

	var manifestGetCmd = &cobra.Command{
		Use:     "get <image_ref>",
		Aliases: []string{"pull"},
		Short:   "retrieve manifest or manifest list",
		Long:    `Shows the manifest or manifest list of the specified image.`,
		Example: `
# retrieve the manifest (pretty formatting)
regctl manifest get alpine

# show the original manifest body for the local platform
regctl manifest get alpine --format raw-body --platform local

# retrieve the manifest for a specific windows version
regctl manifest get golang --platform windows/amd64,osver=10.0.17763.4974`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              manifestOpts.runManifestGet,
	}

	var manifestHeadCmd = &cobra.Command{
		Use:     "head <image_ref>",
		Aliases: []string{"digest"},
		Short:   "http head request for manifest",
		Long:    `Shows the digest or headers from an http manifest head request.`,
		Example: `
# show the digest for an image
regctl manifest head alpine

# show the digest for a specific platform (this will perform a GET request)
regctl manifest head alpine --platform linux/arm64

# show all headers for the request
regctl manifest head alpine --format raw-headers`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              manifestOpts.runManifestHead,
	}

	var manifestPutCmd = &cobra.Command{
		Use:     "put <image_ref>",
		Aliases: []string{"push"},
		Short:   "push manifest or manifest list",
		Long:    `Pushes a manifest or manifest list to a repository.`,
		Example: `
# push an image manifest
regctl manifest put \
  --content-type application/vnd.oci.image.manifest.v1+json \
  registry.example.org/repo:v1 <manifest.json`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rootOpts.completeArgTag,
		RunE:              manifestOpts.runManifestPut,
	}

	manifestDeleteCmd.Flags().BoolVarP(&manifestOpts.forceTagDeref, "force-tag-dereference", "", false, "Dereference the a tag to a digest, this is unsafe")
	manifestDeleteCmd.Flags().BoolVarP(&manifestOpts.referrers, "referrers", "", false, "Check for referrers, recommended when deleting artifacts")

	manifestDiffCmd.Flags().IntVarP(&manifestOpts.diffCtx, "context", "", 3, "Lines of context")
	manifestDiffCmd.Flags().BoolVarP(&manifestOpts.diffFullCtx, "context-full", "", false, "Show all lines of context")

	manifestHeadCmd.Flags().StringVarP(&manifestOpts.formatHead, "format", "", "", "Format output with go template syntax (use \"raw-body\" for the original manifest)")
	manifestHeadCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", true, "Do not resolve platform from manifest list (enabled by default)")
	manifestHeadCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local, requires a get request)")
	manifestHeadCmd.Flags().BoolVarP(&manifestOpts.requireDigest, "require-digest", "", false, "Fallback to get request if digest is not received")
	manifestHeadCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	_ = manifestHeadCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	_ = manifestHeadCmd.Flags().MarkHidden("list")

	manifestGetCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", true, "Deprecated: Output manifest list if available")
	manifestGetCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	manifestGetCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Deprecated: Fail if manifest list is not received")
	manifestGetCmd.Flags().StringVarP(&manifestOpts.formatGet, "format", "", "{{printPretty .}}", "Format output with go template syntax (use \"raw-body\" for the original manifest)")
	_ = manifestGetCmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	_ = manifestGetCmd.RegisterFlagCompletionFunc("format", completeArgNone)
	_ = manifestGetCmd.Flags().MarkHidden("list")

	manifestPutCmd.Flags().BoolVarP(&manifestOpts.byDigest, "by-digest", "", false, "Push manifest by digest instead of tag")
	manifestPutCmd.Flags().StringVarP(&manifestOpts.contentType, "content-type", "t", "", "Specify content-type (e.g. application/vnd.docker.distribution.manifest.v2+json)")
	_ = manifestPutCmd.RegisterFlagCompletionFunc("content-type", completeArgMediaTypeManifest)
	manifestPutCmd.Flags().StringVarP(&manifestOpts.formatPut, "format", "", "", "Format output with go template syntax")

	manifestTopCmd.AddCommand(manifestDeleteCmd)
	manifestTopCmd.AddCommand(manifestDiffCmd)
	manifestTopCmd.AddCommand(manifestHeadCmd)
	manifestTopCmd.AddCommand(manifestGetCmd)
	manifestTopCmd.AddCommand(manifestPutCmd)
	return manifestTopCmd
}

func (manifestOpts *manifestCmd) runManifestDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := manifestOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	if r.Digest == "" && manifestOpts.forceTagDeref {
		m, err := rc.ManifestHead(ctx, r, regclient.WithManifestRequireDigest())
		if err != nil {
			return err
		}
		r.Digest = manifest.GetDigest(m).String()
		log.WithFields(logrus.Fields{
			"tag":    r.Tag,
			"digest": r.Digest,
		}).Debug("Forced dereference of tag")
	}

	log.WithFields(logrus.Fields{
		"host":   r.Registry,
		"repo":   r.Repository,
		"digest": r.Digest,
	}).Debug("Manifest delete")
	mOpts := []regclient.ManifestOpts{}
	if manifestOpts.referrers {
		mOpts = append(mOpts, regclient.WithManifestCheckReferrers())
	}

	err = rc.ManifestDelete(ctx, r, mOpts...)
	if err != nil {
		return err
	}
	return nil
}

func (manifestOpts *manifestCmd) runManifestDiff(cmd *cobra.Command, args []string) error {
	diffOpts := []diff.Opt{}
	if manifestOpts.diffCtx > 0 {
		diffOpts = append(diffOpts, diff.WithContext(manifestOpts.diffCtx, manifestOpts.diffCtx))
	}
	if manifestOpts.diffFullCtx {
		diffOpts = append(diffOpts, diff.WithFullContext())
	}
	ctx := cmd.Context()
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}
	r1, err := ref.New(args[0])
	if err != nil {
		return err
	}
	r2, err := ref.New(args[1])
	if err != nil {
		return err
	}

	rc := manifestOpts.rootOpts.newRegClient()

	log.WithFields(logrus.Fields{
		"ref1": r1.CommonName(),
		"ref2": r2.CommonName(),
	}).Debug("Manifest diff")

	m1, err := rc.ManifestGet(ctx, r1)
	if err != nil {
		return err
	}
	m2, err := rc.ManifestGet(ctx, r2)
	if err != nil {
		return err
	}

	m1Json, err := json.MarshalIndent(m1, "", "  ")
	if err != nil {
		return err
	}
	m2Json, err := json.MarshalIndent(m2, "", "  ")
	if err != nil {
		return err
	}

	mDiff := diff.Diff(strings.Split(string(m1Json), "\n"), strings.Split(string(m2Json), "\n"), diffOpts...)

	_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(mDiff, "\n"))
	return err
	// TODO: support templating
	// return template.Writer(cmd.OutOrStdout(), manifestOpts.format, mDiff)
}

func (manifestOpts *manifestCmd) runManifestHead(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if flagChanged(cmd, "list") {
		log.Info("list option has been deprecated, manifest list is output by default until a platform is specified")
	}
	if manifestOpts.platform != "" && manifestOpts.requireList {
		return fmt.Errorf("cannot request a platform and require-list simultaneously")
	}

	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := manifestOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"host": r.Registry,
		"repo": r.Repository,
		"tag":  r.Tag,
	}).Debug("Manifest head")

	mOpts := []regclient.ManifestOpts{}
	if manifestOpts.requireDigest || (!flagChanged(cmd, "require-digest") && !flagChanged(cmd, "format")) {
		mOpts = append(mOpts, regclient.WithManifestRequireDigest())
	}
	if manifestOpts.platform != "" {
		p, err := platform.Parse(manifestOpts.platform)
		if err != nil {
			return fmt.Errorf("failed to parse platform %s: %w", manifestOpts.platform, err)
		}
		mOpts = append(mOpts, regclient.WithManifestPlatform(p))
	}

	m, err := rc.ManifestHead(ctx, r, mOpts...)
	if err != nil {
		return err
	}

	switch manifestOpts.formatHead {
	case "", "digest":
		manifestOpts.formatHead = "{{ printf \"%s\\n\" .GetDescriptor.Digest }}"
	case "rawHeaders", "raw-headers", "headers":
		manifestOpts.formatHead = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), manifestOpts.formatHead, m)
}

func (manifestOpts *manifestCmd) runManifestGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if flagChanged(cmd, "list") {
		log.Info("list option has been deprecated, manifest list is output by default until a platform is specified")
	}
	if manifestOpts.platform != "" && manifestOpts.requireList {
		return fmt.Errorf("cannot request a platform and require-list simultaneously")
	}

	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := manifestOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"host": r.Registry,
		"repo": r.Repository,
		"tag":  r.Tag,
	}).Debug("Manifest get")

	mOpts := []regclient.ManifestOpts{}
	if manifestOpts.platform != "" {
		p, err := platform.Parse(manifestOpts.platform)
		if err != nil {
			return fmt.Errorf("failed to parse platform %s: %w", manifestOpts.platform, err)
		}
		mOpts = append(mOpts, regclient.WithManifestPlatform(p))
	}

	m, err := rc.ManifestGet(ctx, r, mOpts...)
	if err != nil {
		return err
	}

	switch manifestOpts.formatGet {
	case "raw":
		manifestOpts.formatGet = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		manifestOpts.formatGet = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		manifestOpts.formatGet = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), manifestOpts.formatGet, m)
}

func (manifestOpts *manifestCmd) runManifestPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := manifestOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	raw, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return err
	}
	opts := []manifest.Opts{
		manifest.WithRef(r),
		manifest.WithRaw(raw),
	}
	if manifestOpts.contentType != "" {
		opts = append(opts, manifest.WithDesc(descriptor.Descriptor{
			MediaType: manifestOpts.contentType,
		}))
	}
	rcM, err := manifest.New(opts...)
	if err != nil {
		return err
	}
	if manifestOpts.byDigest {
		r.Tag = ""
		r.Digest = rcM.GetDescriptor().Digest.String()
	}

	err = rc.ManifestPut(ctx, r, rcM)
	if err != nil {
		return err
	}

	result := struct {
		Manifest manifest.Manifest
	}{
		Manifest: rcM,
	}
	if manifestOpts.byDigest && manifestOpts.formatPut == "" {
		manifestOpts.formatPut = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), manifestOpts.formatPut, result)
}
