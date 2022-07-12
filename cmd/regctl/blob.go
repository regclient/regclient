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
	"github.com/regclient/regclient/internal/diff"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var blobCmd = &cobra.Command{
	Use:     "blob <cmd>",
	Aliases: []string{"layer"},
	Short:   "manage image blobs/layers",
}
var blobDiffConfigCmd = &cobra.Command{
	Use:       "diff-config <repository> <digest> <repository> <digest>",
	Short:     "diff two image configs",
	Long:      `This returns the difference between two configs, comparing the contents of each config json.`,
	Args:      cobra.ExactArgs(4),
	ValidArgs: []string{}, // do not auto complete repository or digest
	RunE:      runBlobDiffConfig,
}
var blobDiffLayerCmd = &cobra.Command{
	Use:       "diff-layer <repository> <digest> <repository> <digest>",
	Short:     "diff two tar layers",
	Long:      `This returns the difference between two layers, comparing the contents of each tar.`,
	Args:      cobra.ExactArgs(4),
	ValidArgs: []string{}, // do not auto complete repository or digest
	RunE:      runBlobDiffLayer,
}
var blobGetCmd = &cobra.Command{
	Use:     "get <repository> <digest>",
	Aliases: []string{"pull"},
	Short:   "download a blob/layer",
	Long: `Download a blob from the registry. The output is the blob itself which may
be a compressed tar file, a json config, or any other blob supported by the
registry. The blob or layer digest can be found in the image manifest.`,
	Args:      cobra.ExactArgs(2),
	ValidArgs: []string{}, // do not auto complete repository or digest
	RunE:      runBlobGet,
}
var blobPutCmd = &cobra.Command{
	Use:     "put <repository>",
	Aliases: []string{"push"},
	Short:   "upload a blob/layer",
	Long: `Upload a blob to a repository. Stdin must be the blob contents. The output
is the digest of the blob.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete repository
	RunE:      runBlobPut,
}

var blobOpts struct {
	diffCtx        int
	diffFullCtx    bool
	diffIgnoreTime bool
	format         string
	formatPut      string
	mt             string
	digest         string
}

func init() {
	blobDiffConfigCmd.Flags().IntVarP(&blobOpts.diffCtx, "context", "", 3, "Lines of context")
	blobDiffConfigCmd.Flags().BoolVarP(&blobOpts.diffFullCtx, "context-full", "", false, "Show all lines of context")

	blobDiffLayerCmd.Flags().IntVarP(&blobOpts.diffCtx, "context", "", 3, "Lines of context")
	blobDiffLayerCmd.Flags().BoolVarP(&blobOpts.diffFullCtx, "context-full", "", false, "Show all lines of context")
	blobDiffLayerCmd.Flags().BoolVarP(&blobOpts.diffIgnoreTime, "ignore-timestamp", "", false, "Ignore timestamps on files")

	blobGetCmd.Flags().StringVarP(&blobOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	blobGetCmd.Flags().StringVarP(&blobOpts.mt, "media-type", "", "", "Set the requested mediaType (deprecated)")
	blobGetCmd.RegisterFlagCompletionFunc("format", completeArgNone)
	blobGetCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	blobGetCmd.Flags().MarkHidden("media-type")

	blobPutCmd.Flags().StringVarP(&blobOpts.mt, "content-type", "", "", "Set the requested content type (deprecated)")
	blobPutCmd.Flags().StringVarP(&blobOpts.digest, "digest", "", "", "Set the expected digest")
	blobPutCmd.Flags().StringVarP(&blobOpts.formatPut, "format", "", "{{println .Digest}}", "Format output with go template syntax")
	blobPutCmd.RegisterFlagCompletionFunc("content-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	blobPutCmd.RegisterFlagCompletionFunc("digest", completeArgNone)
	blobPutCmd.Flags().MarkHidden("content-type")

	blobCmd.AddCommand(blobDiffConfigCmd)
	blobCmd.AddCommand(blobDiffLayerCmd)
	blobCmd.AddCommand(blobGetCmd)
	blobCmd.AddCommand(blobPutCmd)
	rootCmd.AddCommand(blobCmd)
}

func runBlobDiffConfig(cmd *cobra.Command, args []string) error {
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
	rc := newRegClient()

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

	_, err = fmt.Fprintln(os.Stdout, strings.Join(cDiff, "\n"))
	return err
	// TODO: support templating
	// return template.Writer(os.Stdout, blobOpts.format, cDiff)
}

func runBlobDiffLayer(cmd *cobra.Command, args []string) error {
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
	rc := newRegClient()

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
	rep1, err := blobReportLayer(tr1)
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
	rep2, err := blobReportLayer(tr2)
	if err != nil {
		return err
	}
	err = btr2.Close()
	if err != nil {
		return err
	}

	// run diff and output result
	lDiff := diff.Diff(rep1, rep2, diffOpts...)
	_, err = fmt.Fprintln(os.Stdout, strings.Join(lDiff, "\n"))
	return err
}

func runBlobGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
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
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	blob, err := rc.BlobGet(ctx, r, types.Descriptor{Digest: d})
	if err != nil {
		return err
	}

	switch blobOpts.format {
	case "raw":
		blobOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		_, err = io.Copy(os.Stdout, blob)
		return err
	case "rawHeaders", "raw-headers", "headers":
		blobOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	case "{{printPretty .}}":
		_, err = io.Copy(os.Stdout, blob)
		return err
	}

	return template.Writer(os.Stdout, blobOpts.format, blob)
}

func runBlobPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

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
	dOut, err := rc.BlobPut(ctx, r, types.Descriptor{Digest: digest.Digest(blobOpts.digest)}, os.Stdin)
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

	return template.Writer(os.Stdout, blobOpts.formatPut, result)
}

func blobReportLayer(tr *tar.Reader) ([]string, error) {
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
