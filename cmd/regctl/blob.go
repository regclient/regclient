package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient/internal/diff"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/warning"
)

type blopOpts struct {
	rootOpts       *rootOpts
	diffCtx        int
	diffFullCtx    bool
	diffIgnoreTime bool
	format         string
	mt             string
	digest         string
}

func NewBlobCmd(rOpts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "blob <cmd>",
		Aliases: []string{"layer"},
		Short:   "manage image blobs/layers",
	}
	cmd.AddCommand(newBlobCopyCmd(rOpts))
	cmd.AddCommand(newBlobDeleteCmd(rOpts))
	cmd.AddCommand(newBlobDiffConfigCmd(rOpts))
	cmd.AddCommand(newBlobDiffLayerCmd(rOpts))
	cmd.AddCommand(newBlobGetCmd(rOpts))
	cmd.AddCommand(newBlobGetFileCmd(rOpts))
	cmd.AddCommand(newBlobHeadCmd(rOpts))
	cmd.AddCommand(newBlobPutCmd(rOpts))
	return cmd
}

func newBlobCopyCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "copy <src_image_ref> <dst_image_ref> <digest>",
		Aliases: []string{"cp"},
		Short:   "copy blob",
		Long: `Copy a blob between repositories. This works in the same registry only. It
attempts to mount the layers between repositories. And within the same repository
it only sends the manifest with the new tag.`,
		Example: `
# copy a blob
regctl blob copy alpine registry.example.org/library/alpine \
  sha256:9123ac7c32f74759e6283f04dbf571f18246abe5bb2c779efcb32cd50f3ff13c`,
		Args:      cobra.ExactArgs(3),
		ValidArgs: []string{}, // do not auto complete repository or digest
		RunE:      opts.runBlobCopy,
	}
	return cmd
}

func newBlobDeleteCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "delete <repository> <digest>",
		Aliases: []string{"del", "rm"},
		Short:   "delete a blob",
		Long: `Delete a blob from the registry. This is rarely needed since registries should
have their own garbage collection algorithms and may clean unreferenced blobs
automatically. This command is useful for repairing a corrupt registry. The
blob or layer digest can be found in the image manifest.`,
		Example: `
# delete a blob
regctl blob delete registry.example.org/repo \
  sha256:a58ecd4f0c864650a4286c3c2d49c7219a3f2fc8d7a0bf478aa9834acfe14ae7`,
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{}, // do not auto complete repository or digest
		RunE:      opts.runBlobDelete,
	}
	return cmd
}

func newBlobDiffConfigCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:   "diff-config <repository> <digest> <repository> <digest>",
		Short: "diff two image configs",
		Long:  `This returns the difference between two configs, comparing the contents of each config json.`,
		Example: `
# compare two versions of busybox
regctl blob diff-config \
  busybox sha256:0c00acac9c2794adfa8bb7b13ef38504300b505a043bf68dff7a00068dcc732b \
  busybox sha256:3f57d9401f8d42f986df300f0c69192fc41da28ccc8d797829467780db3dd741`,
		Args:      cobra.ExactArgs(4),
		ValidArgs: []string{}, // do not auto complete repository or digest
		RunE:      opts.runBlobDiffConfig,
	}
	cmd.Flags().IntVarP(&opts.diffCtx, "context", "", 3, "Lines of context")
	cmd.Flags().BoolVarP(&opts.diffFullCtx, "context-full", "", false, "Show all lines of context")
	return cmd
}

func newBlobDiffLayerCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:   "diff-layer <repository> <digest> <repository> <digest>",
		Short: "diff two tar layers",
		Long:  `This returns the difference between two layers, comparing the contents of each tar.`,
		Example: `
# compare two versions of busybox, ignoring timestamp changes
regctl blob diff-layer \
  busybox sha256:2354422721e449fa3fa83b84465b9d5bb65ac5415ec93c06f598854312e8957e \
  busybox sha256:9ad63333ebc97e32b987ae66aa3cff81300e4c2e6d2f2395cef8a3ae18b249fe --ignore-timestamp`,
		Args:      cobra.ExactArgs(4),
		ValidArgs: []string{}, // do not auto complete repository or digest
		RunE:      opts.runBlobDiffLayer,
	}
	cmd.Flags().IntVarP(&opts.diffCtx, "context", "", 3, "Lines of context")
	cmd.Flags().BoolVarP(&opts.diffFullCtx, "context-full", "", false, "Show all lines of context")
	cmd.Flags().BoolVarP(&opts.diffIgnoreTime, "ignore-timestamp", "", false, "Ignore timestamps on files")
	return cmd
}

func newBlobGetCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "get <repository> <digest>",
		Aliases: []string{"pull"},
		Short:   "download a blob/layer",
		Long: `Download a blob from the registry. The output is the blob itself which may
be a compressed tar file, a json config, or any other blob supported by the
registry. The blob or layer digest can be found in the image manifest.`,
		Example: `
# inspect the layer contents of a busybox image
regctl blob get busybox \
  sha256:a58ecd4f0c864650a4286c3c2d49c7219a3f2fc8d7a0bf478aa9834acfe14ae7 \
  | tar -tvzf -`,
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{}, // do not auto complete repository or digest
		RunE:      opts.runBlobGet,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().StringVarP(&opts.mt, "media-type", "", "", "Set the requested mediaType (deprecated)")
	_ = cmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.Flags().MarkHidden("media-type")
	return cmd
}

func newBlobGetFileCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "get-file <repository> <digest> <file> [out-file]",
		Aliases: []string{"cat"},
		Short:   "get a file from a layer",
		Long:    `This returns a requested file from a layer.`,
		Example: `
# retrieve the contents of /etc/alpine-release
regctl blob get-file alpine \
  sha256:9123ac7c32f74759e6283f04dbf571f18246abe5bb2c779efcb32cd50f3ff13c \
  /etc/alpine-release`,
		Args:      cobra.RangeArgs(3, 4),
		ValidArgs: []string{}, // do not auto complete repository, digest, or filenames
		RunE:      opts.runBlobGetFile,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
}

func newBlobHeadCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "head <repository> <digest>",
		Aliases: []string{"digest"},
		Short:   "http head request for a blob",
		Long:    `Shows the headers for a blob head request.`,
		Example: `
# verify the existence of a blob
regctl blob head alpine \
  sha256:9123ac7c32f74759e6283f04dbf571f18246abe5bb2c779efcb32cd50f3ff13c`,
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{}, // do not auto complete repository or digest
		RunE:      opts.runBlobHead,
	}
	cmd.Flags().StringVarP(&opts.format, "format", "", "", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
}

func newBlobPutCmd(rOpts *rootOpts) *cobra.Command {
	opts := blopOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "put <repository>",
		Aliases: []string{"push"},
		Short:   "upload a blob/layer",
		Long: `Upload a blob to a repository. Stdin must be the blob contents. The output
is the digest of the blob.`,
		Example: `
# push a blob
regctl blob put registry.example.org/repo <layer.tgz`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete repository
		RunE:      opts.runBlobPut,
	}
	cmd.Flags().StringVarP(&opts.mt, "content-type", "", "", "Set the requested content type (deprecated)")
	_ = cmd.RegisterFlagCompletionFunc("content-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.Flags().MarkHidden("content-type")
	cmd.Flags().StringVarP(&opts.digest, "digest", "", "", "Set the expected digest")
	_ = cmd.RegisterFlagCompletionFunc("digest", completeArgNone)
	cmd.Flags().StringVarP(&opts.format, "format", "", "{{println .Digest}}", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	return cmd
}

func (opts *blopOpts) runBlobCopy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	rSrc, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rTgt, err := ref.New(args[1])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[2])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, rSrc)

	opts.rootOpts.log.Debug("Blob copy",
		slog.String("source", rSrc.CommonName()),
		slog.String("target", rTgt.CommonName()),
		slog.String("digest", args[2]))
	err = rc.BlobCopy(ctx, rSrc, rTgt, descriptor.Descriptor{Digest: d})
	if err != nil {
		return err
	}
	return nil
}

func (opts *blopOpts) runBlobDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	opts.rootOpts.log.Debug("Deleting blob",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository),
		slog.String("digest", args[1]))
	return rc.BlobDelete(ctx, r, descriptor.Descriptor{Digest: d})
}

func (opts *blopOpts) runBlobDiffConfig(cmd *cobra.Command, args []string) error {
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
	r2, err := ref.New(args[2])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()

	// open both configs, and output each as formatted json
	d1, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	c1, err := rc.BlobGetOCIConfig(ctx, r1, descriptor.Descriptor{Digest: d1})
	if err != nil {
		return err
	}
	c1Json, err := json.MarshalIndent(c1, "", "  ")
	if err != nil {
		return err
	}

	d2, err := digest.Parse(args[3])
	if err != nil {
		return err
	}
	c2, err := rc.BlobGetOCIConfig(ctx, r2, descriptor.Descriptor{Digest: d2})
	if err != nil {
		return err
	}
	c2Json, err := json.MarshalIndent(c2, "", "  ")
	if err != nil {
		return err
	}

	cDiff := diff.Diff(strings.Split(string(c1Json), "\n"), strings.Split(string(c2Json), "\n"), diffOpts...)

	_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(cDiff, "\n"))
	return err
	// TODO: support templating
	// return template.Writer(cmd.OutOrStdout(), blobOpts.format, cDiff)
}

func (opts *blopOpts) runBlobDiffLayer(cmd *cobra.Command, args []string) error {
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
	r2, err := ref.New(args[2])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()

	// open both blobs, and generate reports of each content
	d1, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	b1, err := rc.BlobGet(ctx, r1, descriptor.Descriptor{Digest: d1})
	if err != nil {
		return err
	}
	defer b1.Close()
	btr1, err := b1.ToTarReader()
	if err != nil {
		return err
	}
	tr1, err := btr1.GetTarReader()
	if err != nil {
		return err
	}
	rep1, err := opts.blobReportLayer(tr1)
	if err != nil {
		return err
	}
	err = btr1.Close()
	if err != nil {
		return err
	}

	d2, err := digest.Parse(args[3])
	if err != nil {
		return err
	}
	b2, err := rc.BlobGet(ctx, r2, descriptor.Descriptor{Digest: d2})
	if err != nil {
		return err
	}
	defer b2.Close()
	btr2, err := b2.ToTarReader()
	if err != nil {
		return err
	}
	tr2, err := btr2.GetTarReader()
	if err != nil {
		return err
	}
	rep2, err := opts.blobReportLayer(tr2)
	if err != nil {
		return err
	}
	err = btr2.Close()
	if err != nil {
		return err
	}

	// run diff and output result
	lDiff := diff.Diff(rep1, rep2, diffOpts...)
	_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(lDiff, "\n"))
	return err
}

func (opts *blopOpts) runBlobGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)
	if opts.mt != "" {
		opts.rootOpts.log.Info("Specifying the blob media type is deprecated",
			slog.String("mt", opts.mt))
	}

	opts.rootOpts.log.Debug("Pulling blob",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository),
		slog.String("digest", args[1]))
	blob, err := rc.BlobGet(ctx, r, descriptor.Descriptor{Digest: d})
	if err != nil {
		return err
	}

	switch opts.format {
	case "raw":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		_, err = io.Copy(cmd.OutOrStdout(), blob)
		return err
	case "rawHeaders", "raw-headers", "headers":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	case "{{printPretty .}}":
		_, err = io.Copy(cmd.OutOrStdout(), blob)
		return err
	}

	return template.Writer(cmd.OutOrStdout(), opts.format, blob)
}

func (opts *blopOpts) runBlobGetFile(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	filename := args[2]
	filename = strings.TrimPrefix(filename, "/")
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	opts.rootOpts.log.Debug("Get file",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository),
		slog.String("digest", args[1]),
		slog.String("filename", filename))
	blob, err := rc.BlobGet(ctx, r, descriptor.Descriptor{Digest: d})
	if err != nil {
		return err
	}
	tr, err := blob.ToTarReader()
	if err != nil {
		return err
	}
	th, rdr, err := tr.ReadFile(filename)
	if err != nil {
		return err
	}
	if opts.format != "" {
		data := struct {
			Header *tar.Header
			Reader io.Reader
		}{
			Header: th,
			Reader: rdr,
		}
		return template.Writer(cmd.OutOrStdout(), opts.format, data)
	}
	var w io.Writer
	if len(args) < 4 {
		w = cmd.OutOrStdout()
	} else {
		w, err = os.Create(args[3])
		if err != nil {
			return err
		}
	}
	_, err = io.Copy(w, rdr)
	if err != nil {
		return err
	}
	if err := tr.Close(); err != nil {
		return err
	}
	if err := blob.Close(); err != nil {
		return err
	}
	return nil
}

func (opts *blopOpts) runBlobHead(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	opts.rootOpts.log.Debug("Blob head",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository),
		slog.String("digest", args[1]))
	blob, err := rc.BlobHead(ctx, r, descriptor.Descriptor{Digest: d})
	if err != nil {
		return err
	}

	switch opts.format {
	case "", "rawHeaders", "raw-headers", "headers":
		opts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}

	return template.Writer(cmd.OutOrStdout(), opts.format, blob)
}

func (opts *blopOpts) runBlobPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := opts.rootOpts.newRegClient()

	if opts.mt != "" {
		opts.rootOpts.log.Info("Specifying the blob media type is deprecated",
			slog.String("mt", opts.mt))
	}

	opts.rootOpts.log.Debug("Pushing blob",
		slog.String("host", r.Registry),
		slog.String("repository", r.Repository),
		slog.String("digest", opts.digest))
	dOut, err := rc.BlobPut(ctx, r, descriptor.Descriptor{Digest: digest.Digest(opts.digest)}, cmd.InOrStdin())
	if err != nil {
		return err
	}

	result := struct {
		Digest digest.Digest
		Size   int64
	}{
		Digest: dOut.Digest,
		Size:   dOut.Size,
	}

	return template.Writer(cmd.OutOrStdout(), opts.format, result)
}

func (opts *blopOpts) blobReportLayer(tr *tar.Reader) ([]string, error) {
	report := []string{}
	if tr == nil {
		return report, nil
	}
	for {
		th, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return report, err
		}
		if th.Mode < 0 || th.Mode > math.MaxUint32 {
			return report, fmt.Errorf("integer conversion overflow/underflow (file mode = %d)", th.Mode)
		}
		line := fmt.Sprintf("%s %d/%d %8d", fs.FileMode(th.Mode).String(), th.Uid, th.Gid, th.Size)
		if !opts.diffIgnoreTime {
			line += " " + th.ModTime.Format(time.RFC3339)
		}
		line += fmt.Sprintf(" %-40s", th.Name)
		if th.Size > 0 {
			d := digest.Canonical.Digester()
			size, err := io.Copy(d.Hash(), tr)
			if err != nil {
				return report, fmt.Errorf("failed to read %s: %w", th.Name, err)
			}
			if size != th.Size {
				return report, fmt.Errorf("size mismatch for %s, expected %d, read %d", th.Name, th.Size, size)
			}
			line += " " + d.Digest().String()
		}
		report = append(report, line)
	}
	return report, nil
}
