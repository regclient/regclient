package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient/internal/diff"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

type blobCmd struct {
	rootOpts       *rootCmd
	diffCtx        int
	diffFullCtx    bool
	diffIgnoreTime bool
	formatGet      string
	formatFile     string
	formatHead     string
	formatPut      string
	mt             string
	digest         string
}

func NewBlobCmd(rootOpts *rootCmd) *cobra.Command {
	blobOpts := blobCmd{
		rootOpts: rootOpts,
	}

	var blobTopCmd = &cobra.Command{
		Use:     "blob <cmd>",
		Aliases: []string{"layer"},
		Short:   "manage image blobs/layers",
	}
	var blobDiffConfigCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobDiffConfig,
	}
	var blobDiffLayerCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobDiffLayer,
	}
	var blobGetCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobGet,
	}
	var blobGetFileCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobGetFile,
	}
	var blobHeadCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobHead,
	}
	var blobPutCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobPut,
	}
	var blobCopyCmd = &cobra.Command{
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
		RunE:      blobOpts.runBlobCopy,
	}

	blobDiffConfigCmd.Flags().IntVarP(&blobOpts.diffCtx, "context", "", 3, "Lines of context")
	blobDiffConfigCmd.Flags().BoolVarP(&blobOpts.diffFullCtx, "context-full", "", false, "Show all lines of context")

	blobDiffLayerCmd.Flags().IntVarP(&blobOpts.diffCtx, "context", "", 3, "Lines of context")
	blobDiffLayerCmd.Flags().BoolVarP(&blobOpts.diffFullCtx, "context-full", "", false, "Show all lines of context")
	blobDiffLayerCmd.Flags().BoolVarP(&blobOpts.diffIgnoreTime, "ignore-timestamp", "", false, "Ignore timestamps on files")

	blobGetCmd.Flags().StringVarP(&blobOpts.formatGet, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	blobGetCmd.Flags().StringVarP(&blobOpts.mt, "media-type", "", "", "Set the requested mediaType (deprecated)")
	_ = blobGetCmd.RegisterFlagCompletionFunc("format", completeArgNone)
	_ = blobGetCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = blobGetCmd.Flags().MarkHidden("media-type")

	blobGetFileCmd.Flags().StringVarP(&blobOpts.formatFile, "format", "", "", "Format output with go template syntax")

	blobHeadCmd.Flags().StringVarP(&blobOpts.formatHead, "format", "", "", "Format output with go template syntax")
	_ = blobHeadCmd.RegisterFlagCompletionFunc("format", completeArgNone)

	blobPutCmd.Flags().StringVarP(&blobOpts.mt, "content-type", "", "", "Set the requested content type (deprecated)")
	blobPutCmd.Flags().StringVarP(&blobOpts.digest, "digest", "", "", "Set the expected digest")
	blobPutCmd.Flags().StringVarP(&blobOpts.formatPut, "format", "", "{{println .Digest}}", "Format output with go template syntax")
	_ = blobPutCmd.RegisterFlagCompletionFunc("content-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = blobPutCmd.RegisterFlagCompletionFunc("digest", completeArgNone)
	_ = blobPutCmd.Flags().MarkHidden("content-type")

	blobTopCmd.AddCommand(blobDiffConfigCmd)
	blobTopCmd.AddCommand(blobDiffLayerCmd)
	blobTopCmd.AddCommand(blobGetCmd)
	blobTopCmd.AddCommand(blobGetFileCmd)
	blobTopCmd.AddCommand(blobHeadCmd)
	blobTopCmd.AddCommand(blobPutCmd)
	blobTopCmd.AddCommand(blobCopyCmd)

	return blobTopCmd
}

func (blobOpts *blobCmd) runBlobDiffConfig(cmd *cobra.Command, args []string) error {
	diffOpts := []diff.Opt{}
	if blobOpts.diffCtx > 0 {
		diffOpts = append(diffOpts, diff.WithContext(blobOpts.diffCtx, blobOpts.diffCtx))
	}
	if blobOpts.diffFullCtx {
		diffOpts = append(diffOpts, diff.WithFullContext())
	}
	ctx := cmd.Context()
	r1, err := ref.New(args[0])
	if err != nil {
		return err
	}
	r2, err := ref.New(args[2])
	if err != nil {
		return err
	}
	rc := blobOpts.rootOpts.newRegClient()

	// open both configs, and output each as formatted json
	d1, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	c1, err := rc.BlobGetOCIConfig(ctx, r1, types.Descriptor{Digest: d1})
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
	c2, err := rc.BlobGetOCIConfig(ctx, r2, types.Descriptor{Digest: d2})
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

func (blobOpts *blobCmd) runBlobDiffLayer(cmd *cobra.Command, args []string) error {
	diffOpts := []diff.Opt{}
	if blobOpts.diffCtx > 0 {
		diffOpts = append(diffOpts, diff.WithContext(blobOpts.diffCtx, blobOpts.diffCtx))
	}
	if blobOpts.diffFullCtx {
		diffOpts = append(diffOpts, diff.WithFullContext())
	}
	ctx := cmd.Context()
	r1, err := ref.New(args[0])
	if err != nil {
		return err
	}
	r2, err := ref.New(args[2])
	if err != nil {
		return err
	}
	rc := blobOpts.rootOpts.newRegClient()

	// open both blobs, and generate reports of each content
	d1, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	b1, err := rc.BlobGet(ctx, r1, types.Descriptor{Digest: d1})
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
	rep1, err := blobOpts.blobReportLayer(tr1)
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
	b2, err := rc.BlobGet(ctx, r2, types.Descriptor{Digest: d2})
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
	rep2, err := blobOpts.blobReportLayer(tr2)
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

func (blobOpts *blobCmd) runBlobGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	rc := blobOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)
	if blobOpts.mt != "" {
		log.WithFields(logrus.Fields{
			"mt": blobOpts.mt,
		}).Info("Specifying the blob media type is deprecated")
	}

	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
		"digest":     args[1],
	}).Debug("Pulling blob")
	blob, err := rc.BlobGet(ctx, r, types.Descriptor{Digest: d})
	if err != nil {
		return err
	}

	switch blobOpts.formatGet {
	case "raw":
		blobOpts.formatGet = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		_, err = io.Copy(cmd.OutOrStdout(), blob)
		return err
	case "rawHeaders", "raw-headers", "headers":
		blobOpts.formatGet = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	case "{{printPretty .}}":
		_, err = io.Copy(cmd.OutOrStdout(), blob)
		return err
	}

	return template.Writer(cmd.OutOrStdout(), blobOpts.formatGet, blob)
}

func (blobOpts *blobCmd) runBlobGetFile(cmd *cobra.Command, args []string) error {
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
	rc := blobOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
		"digest":     args[1],
		"filename":   filename,
	}).Debug("Get file")
	blob, err := rc.BlobGet(ctx, r, types.Descriptor{Digest: d})
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
	if blobOpts.formatFile != "" {
		data := struct {
			Header *tar.Header
			Reader io.Reader
		}{
			Header: th,
			Reader: rdr,
		}
		return template.Writer(cmd.OutOrStdout(), blobOpts.formatFile, data)
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

func (blobOpts *blobCmd) runBlobHead(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	rc := blobOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
		"digest":     args[1],
	}).Debug("Blob head")
	blob, err := rc.BlobHead(ctx, r, types.Descriptor{Digest: d})
	if err != nil {
		return err
	}

	switch blobOpts.formatHead {
	case "", "rawHeaders", "raw-headers", "headers":
		blobOpts.formatHead = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}

	return template.Writer(cmd.OutOrStdout(), blobOpts.formatHead, blob)
}

func (blobOpts *blobCmd) runBlobPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := blobOpts.rootOpts.newRegClient()

	if blobOpts.mt != "" {
		log.WithFields(logrus.Fields{
			"mt": blobOpts.mt,
		}).Info("Specifying the blob media type is deprecated")
	}

	log.WithFields(logrus.Fields{
		"host":       r.Registry,
		"repository": r.Repository,
		"digest":     blobOpts.digest,
	}).Debug("Pushing blob")
	dOut, err := rc.BlobPut(ctx, r, types.Descriptor{Digest: digest.Digest(blobOpts.digest)}, cmd.InOrStdin())
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

	return template.Writer(cmd.OutOrStdout(), blobOpts.formatPut, result)
}

func (blobOpts *blobCmd) runBlobCopy(cmd *cobra.Command, args []string) error {
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
	rc := blobOpts.rootOpts.newRegClient()
	defer rc.Close(ctx, rSrc)

	log.WithFields(logrus.Fields{
		"source": rSrc.CommonName(),
		"target": rTgt.CommonName(),
		"digest": args[2],
	}).Debug("Blob copy")
	err = rc.BlobCopy(ctx, rSrc, rTgt, types.Descriptor{Digest: d})
	if err != nil {
		return err
	}
	return nil
}

func (blobOpts *blobCmd) blobReportLayer(tr *tar.Reader) ([]string, error) {
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
		line := fmt.Sprintf("%s %d/%d %8d", fs.FileMode(th.Mode).String(), th.Uid, th.Gid, th.Size)
		if !blobOpts.diffIgnoreTime {
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
