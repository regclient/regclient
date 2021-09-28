package main

import (
	"context"
	"io"
	"os"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var blobCmd = &cobra.Command{
	Use:     "blob <cmd>",
	Aliases: []string{"layer"},
	Short:   "manage image blobs/layers",
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
	Aliases: []string{"put", "push"},
	Short:   "upload a blob/layer",
	Long: `Upload a blob to a repository. Stdin must be the blob contents. The output
is the digest of the blob.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete repository
	RunE:      runBlobPut,
}

var blobOpts struct {
	format string
	mt     string
	digest string
}

func init() {
	blobGetCmd.Flags().StringVarP(&blobOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")
	blobGetCmd.Flags().StringVarP(&blobOpts.mt, "media-type", "", "", "Set the requested mediaType")
	blobGetCmd.RegisterFlagCompletionFunc("format", completeArgNone)
	blobGetCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	blobPutCmd.Flags().StringVarP(&blobOpts.mt, "content-type", "", "", "Set the requested content type")
	blobPutCmd.Flags().StringVarP(&blobOpts.digest, "digest", "", "", "Set the expected digest")
	blobPutCmd.RegisterFlagCompletionFunc("content-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"application/octet-stream",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	blobPutCmd.RegisterFlagCompletionFunc("digest", completeArgNone)

	blobCmd.AddCommand(blobGetCmd)
	blobCmd.AddCommand(blobPutCmd)
	rootCmd.AddCommand(blobCmd)
}

func runBlobGet(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	accepts := []string{}
	if blobOpts.mt != "" {
		accepts = []string{blobOpts.mt}
	}

	log.WithFields(logrus.Fields{
		"host":       ref.Registry,
		"repository": ref.Repository,
		"digest":     args[1],
	}).Debug("Pulling blob")
	d, err := digest.Parse(args[1])
	if err != nil {
		return err
	}
	blob, err := rc.BlobGet(context.Background(), ref, d, accepts)
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

	return template.Writer(os.Stdout, blobOpts.format, blob, template.WithFuncs(regclient.TemplateFuncs))
}

func runBlobPut(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host":         ref.Registry,
		"repository":   ref.Repository,
		"digest":       blobOpts.digest,
		"content-type": blobOpts.mt,
	}).Debug("Pushing blob")
	dOut, size, err := rc.BlobPut(context.Background(), ref, digest.Digest(blobOpts.digest), os.Stdin, blobOpts.mt, 0)
	if err != nil {
		return err
	}

	result := struct {
		Digest digest.Digest
		Size   int64
	}{
		Digest: dOut,
		Size:   size,
	}

	return template.Writer(os.Stdout, blobOpts.format, result, template.WithFuncs(regclient.TemplateFuncs))
}
