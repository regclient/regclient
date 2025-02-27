package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

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

type manifestOpts struct {
	rootOpts      *rootOpts
	byDigest      bool
	contentType   string
	diffCtx       int
	diffFullCtx   bool
	forceTagDeref bool
	format        string
	list          bool
	platform      string
	referrers     bool
	requireDigest bool
	requireList   bool
}

func NewManifestCmd(rOpts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest <cmd>",
		Short: "manage manifests",
	}

	cmd.AddCommand(newManifestDeleteCmd(rOpts))
	cmd.AddCommand(newManifestDiffCmd(rOpts))
	cmd.AddCommand(newManifestHeadCmd(rOpts))
	cmd.AddCommand(newManifestGetCmd(rOpts))
	cmd.AddCommand(newManifestPutCmd(rOpts))
	return cmd
}

func newManifestDeleteCmd(rOpts *rootOpts) *cobra.Command {
	opts := manifestOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		RunE:      opts.runManifestDelete,
	}
	cmd.Flags().BoolVarP(&opts.forceTagDeref, "force-tag-dereference", "", false, "Dereference the a tag to a digest, this is unsafe")
	cmd.Flags().BoolVarP(&opts.referrers, "referrers", "", false, "Check for referrers, recommended when deleting artifacts")
	return cmd
}

func newManifestDiffCmd(rOpts *rootOpts) *cobra.Command {
	opts := manifestOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		ValidArgsFunction: rOpts.completeArgTag,
		RunE:              opts.runManifestDiff,
	}
	cmd.Flags().IntVarP(&opts.diffCtx, "context", "", 3, "Lines of context")
	cmd.Flags().BoolVarP(&opts.diffFullCtx, "context-full", "", false, "Show all lines of context")
	return cmd
}

func newManifestGetCmd(rOpts *rootOpts) *cobra.Command {
	opts := manifestOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		ValidArgsFunction: rOpts.completeArgTag,
		RunE:              opts.runManifestGet,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax (use \"raw-body\" for the original manifest)")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().BoolVarP(&opts.list, "list", "", true, "Deprecated: Output manifest list if available")
	_ = cmd.Flags().MarkHidden("list")
	cmd.Flags().StringVarP(&opts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local)")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	cmd.Flags().BoolVarP(&opts.requireList, "require-list", "", false, "Deprecated: Fail if manifest list is not received")
	return cmd
}

func newManifestHeadCmd(rOpts *rootOpts) *cobra.Command {
	opts := manifestOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "head <image_ref>",
		Aliases: []string{"digest"},
		Short:   "http head request for manifest",
		Long:    `Shows the digest or headers from an http manifest head request.`,
		Example: `
# show the digest for an image
regctl manifest head alpine

# "regctl image digest" is an alias
regctl image digest alpine

# show the digest for a specific platform (this will perform a GET request)
regctl manifest head alpine --platform linux/arm64

# show all headers for the request
regctl manifest head alpine --format raw-headers`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: rOpts.completeArgTag,
		RunE:              opts.runManifestHead,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "", "Format output with go template syntax (use \"raw-body\" for the original manifest)")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().BoolVarP(&opts.list, "list", "", true, "Do not resolve platform from manifest list (enabled by default)")
	_ = cmd.Flags().MarkHidden("list")
	cmd.Flags().StringVarP(&opts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64 or local, requires a get request)")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	cmd.Flags().BoolVarP(&opts.requireDigest, "require-digest", "", false, "Fallback to a GET request if digest is not received")
	cmd.Flags().BoolVarP(&opts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	return cmd
}

func newManifestPutCmd(rOpts *rootOpts) *cobra.Command {
	opts := manifestOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
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
		ValidArgsFunction: rOpts.completeArgTag,
		RunE:              opts.runManifestPut,
	}
	cmd.Flags().BoolVarP(&opts.byDigest, "by-digest", "", false, "Push manifest by digest instead of tag")
	cmd.Flags().StringVarP(&opts.contentType, "content-type", "t", "", "Specify content-type (e.g. application/vnd.docker.distribution.manifest.v2+json)")
	_ = cmd.RegisterFlagCompletionFunc("content-type", completeArgMediaTypeManifest)
	cmd.Flags().StringVarP(&opts.format, "format", "", "", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
}

func (opts *manifestOpts) runManifestDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	if r.Digest == "" && opts.forceTagDeref {
		m, err := rc.ManifestHead(ctx, r, regclient.WithManifestRequireDigest())
		if err != nil {
			return err
		}
		r = r.AddDigest(manifest.GetDigest(m).String())
		opts.rootOpts.log.Debug("Forced dereference of tag",
			slog.String("orig", args[0]),
			slog.String("resolved", r.CommonName()))
	}

	opts.rootOpts.log.Debug("Manifest delete",
		slog.String("host", r.Registry),
		slog.String("repo", r.Repository),
		slog.String("digest", r.Digest))
	mOpts := []regclient.ManifestOpts{}
	if opts.referrers {
		mOpts = append(mOpts, regclient.WithManifestCheckReferrers())
	}

	err = rc.ManifestDelete(ctx, r, mOpts...)
	if err != nil {
		return err
	}
	return nil
}

func (opts *manifestOpts) runManifestDiff(cmd *cobra.Command, args []string) error {
	diffOpts := []diff.Opt{}
	if opts.diffCtx > 0 {
		diffOpts = append(diffOpts, diff.WithContext(opts.diffCtx, opts.diffCtx))
	}
	if opts.diffFullCtx {
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

	rc := opts.rootOpts.newRegClient()

	opts.rootOpts.log.Debug("Manifest diff",
		slog.String("ref1", r1.CommonName()),
		slog.String("ref2", r2.CommonName()))

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

func (opts *manifestOpts) runManifestHead(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if flagChanged(cmd, "list") {
		opts.rootOpts.log.Info("list option has been deprecated, manifest list is output by default until a platform is specified")
	}
	if opts.platform != "" && opts.requireList {
		return fmt.Errorf("cannot request a platform and require-list simultaneously")
	}

	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	opts.rootOpts.log.Debug("Manifest head",
		slog.String("host", r.Registry),
		slog.String("repo", r.Repository),
		slog.String("tag", r.Tag))

	mOpts := []regclient.ManifestOpts{}
	if opts.requireDigest || (!flagChanged(cmd, "require-digest") && !flagChanged(cmd, "format")) {
		mOpts = append(mOpts, regclient.WithManifestRequireDigest())
	}
	if opts.platform != "" {
		p, err := platform.Parse(opts.platform)
		if err != nil {
			return fmt.Errorf("failed to parse platform %s: %w", opts.platform, err)
		}
		mOpts = append(mOpts, regclient.WithManifestPlatform(p))
	}

	m, err := rc.ManifestHead(ctx, r, mOpts...)
	if err != nil {
		return err
	}

	switch opts.format {
	case "", "digest":
		opts.format = "{{ printf \"%s\\n\" .GetDescriptor.Digest }}"
	case "rawHeaders", "raw-headers", "headers":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, m)
}

func (opts *manifestOpts) runManifestGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if flagChanged(cmd, "list") {
		opts.rootOpts.log.Info("list option has been deprecated, manifest list is output by default until a platform is specified")
	}
	if opts.platform != "" && opts.requireList {
		return fmt.Errorf("cannot request a platform and require-list simultaneously")
	}

	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	opts.rootOpts.log.Debug("Manifest get",
		slog.String("host", r.Registry),
		slog.String("repo", r.Repository),
		slog.String("tag", r.Tag))

	mOpts := []regclient.ManifestOpts{}
	if opts.platform != "" {
		p, err := platform.Parse(opts.platform)
		if err != nil {
			return fmt.Errorf("failed to parse platform %s: %w", opts.platform, err)
		}
		mOpts = append(mOpts, regclient.WithManifestPlatform(p))
	}

	m, err := rc.ManifestGet(ctx, r, mOpts...)
	if err != nil {
		return err
	}

	switch opts.format {
	case "raw":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		opts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, m)
}

func (opts *manifestOpts) runManifestPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	raw, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return err
	}
	mOpts := []manifest.Opts{
		manifest.WithRef(r),
		manifest.WithRaw(raw),
	}
	if opts.contentType != "" {
		mOpts = append(mOpts, manifest.WithDesc(descriptor.Descriptor{
			MediaType: opts.contentType,
		}))
	}
	rcM, err := manifest.New(mOpts...)
	if err != nil {
		return err
	}
	if opts.byDigest {
		r = r.SetDigest(rcM.GetDescriptor().Digest.String())
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
	if opts.byDigest && opts.format == "" {
		opts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, result)
}
