package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/spf13/cobra"
)

const (
	ociAnnotTitle   = "org.opencontainers.image.title"
	defaultMTConfig = "application/vnd.unknown.config.v1+json"
	defaultMTLayer  = "application/octet-stream"
)

var manifestKnownTypes = []string{
	types.MediaTypeOCI1Manifest,
	types.MediaTypeOCI1Artifact,
}
var artifactKnownTypes = []string{
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
	"application/vnd.unknown.config.v1+json",
}

var artifactCmd = &cobra.Command{
	Use:   "artifact <cmd>",
	Short: "manage artifacts",
}
var artifactGetCmd = &cobra.Command{
	Use:       "get <reference>",
	Aliases:   []string{"pull"},
	Short:     "download artifacts",
	Long:      `Download artifacts from the registry.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete repository/tag
	RunE:      runArtifactGet,
}
var artifactListCmd = &cobra.Command{
	Use:     "list <reference>",
	Aliases: []string{"pull"},
	// TODO: remove experimental label when stable
	Short:     "EXPERIMENTAL: list artifacts that refer to the given reference",
	Long:      `List artifacts that refer to the given reference.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete repository/tag
	RunE:      runArtifactList,
}
var artifactPutCmd = &cobra.Command{
	Use:       "put <reference>",
	Aliases:   []string{"push"},
	Short:     "upload artifacts",
	Long:      `Upload artifacts to the registry.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete repository/tag
	RunE:      runArtifactPut,
}

var artifactOpts struct {
	annotations  []string
	artifactFile []string
	artifactMT   []string
	configFile   string
	configMT     string
	forceGet     bool
	format       string
	manifestMT   string
	outputDir    string
	refers       bool
	stripDirs    bool
}

func init() {
	artifactGetCmd.Flags().StringArrayVarP(&artifactOpts.artifactFile, "file", "f", []string{}, "Filter by artifact filename")
	artifactGetCmd.Flags().StringArrayVarP(&artifactOpts.artifactMT, "media-type", "", []string{}, "Filter by artifact media-type")
	artifactGetCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return artifactKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	artifactGetCmd.Flags().StringVarP(&artifactOpts.configFile, "config-file", "", "", "Config filename to output")
	artifactGetCmd.Flags().StringVarP(&artifactOpts.outputDir, "output", "o", "", "Output directory for multiple artifacts")
	artifactGetCmd.Flags().BoolVarP(&artifactOpts.stripDirs, "strip-dirs", "", false, "Strip directories from filenames in output dir")

	artifactListCmd.Flags().BoolVarP(&artifactOpts.forceGet, "force-get", "", false, "Force get of manifests to populate annotations")
	artifactListCmd.Flags().StringVarP(&artifactOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	artifactPutCmd.Flags().StringArrayVarP(&artifactOpts.annotations, "annotation", "", []string{}, "Annotation to include on manifest")
	artifactPutCmd.Flags().StringArrayVarP(&artifactOpts.artifactFile, "file", "f", []string{}, "Artifact filename")
	artifactPutCmd.Flags().StringArrayVarP(&artifactOpts.artifactMT, "media-type", "m", []string{}, "Set the artifact media-type")
	artifactPutCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return artifactKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	artifactPutCmd.Flags().StringVarP(&artifactOpts.configFile, "config-file", "", "", "Config filename")
	artifactPutCmd.Flags().StringVarP(&artifactOpts.configMT, "config-media-type", "", "", "Config media-type")
	artifactPutCmd.RegisterFlagCompletionFunc("config-media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return configKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	artifactPutCmd.Flags().StringVarP(&artifactOpts.manifestMT, "manifest-media-type", "", types.MediaTypeOCI1Manifest, "Manifest media-type")
	artifactPutCmd.RegisterFlagCompletionFunc("manifest-media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return manifestKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	// TODO: remove experimental label when stable
	artifactPutCmd.Flags().BoolVarP(&artifactOpts.refers, "refers", "", false, "EXPERIMENTAL: Create a referrer to the reference")
	artifactPutCmd.Flags().BoolVarP(&artifactOpts.stripDirs, "strip-dirs", "", false, "Strip directories from filenames in artifact")

	artifactCmd.AddCommand(artifactGetCmd)
	artifactCmd.AddCommand(artifactListCmd)
	artifactCmd.AddCommand(artifactPutCmd)
	rootCmd.AddCommand(artifactCmd)
}

func runArtifactGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate inputs
	// if output dir defined, ensure it exists
	if artifactOpts.outputDir != "" {
		fi, err := os.Stat(artifactOpts.outputDir)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("output must be a directory: \"%s\"", artifactOpts.outputDir)
		}
	}

	// pull the manifest
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	defer rc.Close(ctx, r)
	mm, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}

	// if config-file defined, create file as writer, perform a blob get
	if artifactOpts.configFile != "" {
		d, err := mm.GetConfig()
		if err != nil {
			return err
		}
		rdr, err := rc.BlobGet(ctx, r, d)
		if err != nil {
			return err
		}
		defer rdr.Close()
		fh, err := os.Create(artifactOpts.configFile)
		if err != nil {
			return err
		}
		defer fh.Close()
		io.Copy(fh, rdr)
	}

	// get list of layers
	layers, err := mm.GetLayers()
	if err != nil {
		return err
	}
	// filter by media-type if defined
	if len(artifactOpts.artifactMT) > 0 {
		for i := len(layers) - 1; i >= 0; i-- {
			found := false
			for _, mt := range artifactOpts.artifactMT {
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
		io.Copy(os.Stdout, rdr)
	}

	return nil
}

func runArtifactList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate inputs
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	rc := newRegClient()
	defer rc.Close(ctx, r)

	referrerOpts := []scheme.ReferrerOpts{}
	if artifactOpts.forceGet {
		referrerOpts = append(referrerOpts, scheme.WithReferrerForceGet())
	}

	rl, err := rc.RefererrList(ctx, r, referrerOpts...)
	if err != nil {
		return err
	}
	switch artifactOpts.format {
	case "raw":
		artifactOpts.format = "{{ range $key,$vals := .Manifest.RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .Manifest.RawBody}}"
	case "rawBody", "raw-body", "body":
		artifactOpts.format = "{{printf \"%s\" .Manifest.RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		artifactOpts.format = "{{ range $key,$vals := .Manifest.RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(os.Stdout, artifactOpts.format, rl)
}

func runArtifactPut(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mOpts := []manifest.Opts{}
	hasConfig := false

	switch artifactOpts.manifestMT {
	case types.MediaTypeOCI1Artifact:
		hasConfig = false
	case "", types.MediaTypeOCI1Manifest:
		hasConfig = true
	default:
		return fmt.Errorf("unsupported manifest media type: %s", artifactOpts.manifestMT)
	}

	// validate inputs
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}
	if len(artifactOpts.artifactFile) == 1 && len(artifactOpts.artifactMT) == 0 {
		// default media-type for a single file, same is used for stdin
		artifactOpts.artifactMT = []string{defaultMTLayer}
	} else if len(artifactOpts.artifactFile) == 0 && len(artifactOpts.artifactMT) == 1 {
		// no-op, special case for stdin with a media type
	} else if len(artifactOpts.artifactFile) != len(artifactOpts.artifactMT) {
		// all other mis-matches are invalid
		return fmt.Errorf("one artifact media-type must be set for each artifact file")
	}
	if artifactOpts.configMT == "" {
		artifactOpts.configMT = defaultMTConfig
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
	rc := newRegClient()
	defer rc.Close(ctx, r)

	refDesc := types.Descriptor{}
	if artifactOpts.refers {
		rmh, err := rc.ManifestHead(ctx, r)
		if err != nil {
			return fmt.Errorf("unable to find referenced manifest: %w", err)
		}
		refDesc = rmh.GetDescriptor()
	}

	// read config, or initialize to an empty json config
	confDesc := types.Descriptor{}
	if hasConfig {
		configBytes := []byte("{}")
		if artifactOpts.configFile != "" {
			var err error
			configBytes, err = os.ReadFile(artifactOpts.configFile)
			if err != nil {
				return err
			}
		}
		configDigest := digest.FromBytes(configBytes)
		// push config to registry
		_, err = rc.BlobPut(ctx, r, types.Descriptor{Digest: configDigest, Size: int64(len(configBytes))}, bytes.NewReader(configBytes))
		if err != nil {
			return err
		}
		// save config descriptor to manifest
		confDesc = types.Descriptor{
			MediaType: artifactOpts.configMT,
			Digest:    configDigest,
			Size:      int64(len(configBytes)),
		}
	} else if artifactOpts.configFile != "" {
		return fmt.Errorf("config is not supported with media type %s", artifactOpts.manifestMT)
	}

	blobs := []types.Descriptor{}
	if len(artifactOpts.artifactFile) > 0 {
		// if files were passed
		for i, f := range artifactOpts.artifactFile {
			// wrap in a closure to trigger defer on each step, avoiding open file handles
			err = func() error {
				mt := artifactOpts.artifactMT[i]
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
				rdr, err := os.Open(openF)
				if err != nil {
					return err
				}
				defer rdr.Close()
				// compute digest on file
				digester := digest.Canonical.Digester()
				l, err := io.Copy(digester.Hash(), rdr)
				if err != nil {
					return err
				}
				d := digester.Digest()
				// add layer to manifest
				af := f
				if artifactOpts.stripDirs {
					fSplit := strings.Split(f, "/")
					if fSplit[len(fSplit)-1] != "" {
						af = fSplit[len(fSplit)-1]
					} else if len(fSplit) > 1 {
						af = fSplit[len(fSplit)-2] + "/"
					}
				}
				blobs = append(blobs, types.Descriptor{
					MediaType: mt,
					Digest:    d,
					Size:      l,
					Annotations: map[string]string{
						ociAnnotTitle: af,
					},
				})
				// if blob already exists, skip Put
				bRdr, err := rc.BlobHead(ctx, r, types.Descriptor{Digest: d})
				if err == nil {
					bRdr.Close()
					return nil
				}
				// need to put blob
				_, err = rdr.Seek(0, 0)
				if err != nil {
					return err
				}
				_, err = rc.BlobPut(ctx, r, types.Descriptor{Digest: d, Size: l}, rdr)
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
		if len(artifactOpts.artifactMT) > 0 {
			mt = artifactOpts.artifactMT[0]
		}
		d, err := rc.BlobPut(ctx, r, types.Descriptor{}, os.Stdin)
		if err != nil {
			return err
		}
		d.MediaType = mt
		blobs = append(blobs, d)
	}

	if artifactOpts.manifestMT == types.MediaTypeOCI1Artifact {
		m := v1.ArtifactManifest{
			Versioned:   v1.ArtifactSchemaVersion,
			MediaType:   types.MediaTypeOCI1Artifact,
			Blobs:       blobs,
			Annotations: annotations,
			Refers:      refDesc,
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	} else {
		m := v1.Manifest{
			Versioned:   v1.ManifestSchemaVersion,
			MediaType:   types.MediaTypeOCI1Manifest,
			Config:      confDesc,
			Layers:      blobs,
			Annotations: annotations,
			Refers:      refDesc,
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	}

	// generate manifest
	mm, err := manifest.New(mOpts...)
	if err != nil {
		return err
	}

	// push manifest
	if artifactOpts.refers {
		return rc.ReferrerPut(ctx, r, mm)
	} else {
		return rc.ManifestPut(ctx, r, mm)
	}
}
