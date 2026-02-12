// Package mod changes an image according to the requested modifications.
package mod

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	zstd "github.com/klauspost/stdgozstd"
	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/warning"
)

// Opts defines options for Apply
type Opts func(*dagConfig, *dagManifest) error

// OptTime defines time settings for [WithConfigTimestamp] and [WithLayerTimestamp].
type OptTime struct {
	Set        time.Time // time to set, this or FromLabel are required
	FromLabel  string    // label from which to extract set time
	After      time.Time // only change times that are after this
	BaseRef    ref.Ref   // define base image, do not alter timestamps from base layers
	BaseLayers int       // define a number of layers to not modify (count of the layers in a base image)
}

var (
	// known tar media types
	mtKnownTar = []string{
		mediatype.Docker2Layer,
		mediatype.Docker2LayerGzip,
		mediatype.Docker2LayerZstd,
		mediatype.OCI1Layer,
		mediatype.OCI1LayerGzip,
		mediatype.OCI1LayerZstd,
	}
	// known config media types
	mtKnownConfig = []string{
		mediatype.Docker2ImageConfig,
		mediatype.OCI1ImageConfig,
	}
)

// Apply applies a set of modifications to an image (manifest, configs, and layers).
func Apply(ctx context.Context, rc *regclient.RegClient, rSrc ref.Ref, opts ...Opts) (ref.Ref, error) {
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	// pull the image metadata into a DAG
	dm, err := dagGet(ctx, rc, rSrc, descriptor.Descriptor{})
	if err != nil {
		return rSrc, err
	}
	dm.top = true

	// load the options
	rTgt := rSrc.SetTag("")
	dc := dagConfig{
		stepsManifest:  []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagManifest) error{},
		stepsOCIConfig: []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagOCIConfig) error{},
		stepsLayer:     []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagLayer, io.ReadCloser) (io.ReadCloser, error){},
		stepsLayerFile: []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagLayer, *tar.Header, io.Reader) (*tar.Header, io.Reader, changes, error){},
		maxDataSize:    -1, // unchanged, if a data field exists, preserve it
		rTgt:           rTgt,
	}
	for _, opt := range opts {
		if err := opt(&dc, dm); err != nil {
			return rSrc, err
		}
	}
	rTgt = dc.rTgt

	// perform manifest changes
	if len(dc.stepsManifest) > 0 {
		err = dagWalkManifests(dm, func(dm *dagManifest) (*dagManifest, error) {
			for _, fn := range dc.stepsManifest {
				err := fn(ctx, rc, rSrc, rTgt, dm)
				if err != nil {
					return nil, err
				}
			}
			return dm, nil
		})
		if err != nil {
			return rTgt, err
		}
	}
	// perform config changes
	if len(dc.stepsOCIConfig) > 0 {
		err = dagWalkOCIConfig(dm, func(doc *dagOCIConfig) (*dagOCIConfig, error) {
			for _, fn := range dc.stepsOCIConfig {
				err := fn(ctx, rc, rSrc, rTgt, doc)
				if err != nil {
					return nil, err
				}
			}
			return doc, nil
		})
		if err != nil {
			return rTgt, err
		}
	}
	// perform layer changes and copy layers to target repository
	if len(dc.stepsLayer) > 0 || len(dc.stepsLayerFile) > 0 || !ref.EqualRepository(rSrc, rTgt) || dc.forceLayerWalk {
		err = dagWalkLayers(dm, func(dl *dagLayer) (*dagLayer, error) {
			var rdr io.ReadCloser
			defer func() {
				if rdr != nil {
					_ = rdr.Close()
				}
			}()
			var err error
			rSrc := rSrc
			if dl.rSrc.IsSet() {
				rSrc = dl.rSrc
			}
			if dl.mod == deleted || len(dl.desc.URLs) > 0 {
				// skip deleted and external layers
				return dl, nil
			}
			// changes for the entire layer
			if len(dc.stepsLayer) > 0 {
				bRdr, err := rc.BlobGet(ctx, rSrc, dl.desc)
				if err != nil {
					return nil, err
				}
				rdr = bRdr
				for _, sl := range dc.stepsLayer {
					rdrNext, err := sl(ctx, rc, rSrc, rTgt, dl, rdr)
					if err != nil {
						return nil, err
					}
					rdr = rdrNext
				}
			}
			// changes for files within layers require extracting the tar and then repackaging it
			if len(dc.stepsLayerFile) > 0 && slices.Contains(mtKnownTar, dl.desc.MediaType) {
				if dl.mod == deleted {
					return dl, nil
				}
				if rdr == nil {
					bRdr, err := rc.BlobGet(ctx, rSrc, dl.desc)
					if err != nil {
						return nil, err
					}
					rdr = bRdr
				}
				changed := false
				empty := true
				desc := dl.desc
				if dl.newDesc.MediaType != "" {
					desc = dl.newDesc
				}
				// if compressed, setup a decompressing reader that passes through the close
				if desc.MediaType != mediatype.OCI1Layer && desc.MediaType != mediatype.Docker2Layer {
					dr, err := archive.Decompress(rdr)
					if err != nil {
						_ = rdr.Close()
						return nil, err
					}
					rdr = readCloserFn{Reader: dr, closeFn: rdr.Close}
				}
				// setup tar reader to process layer
				tr := tar.NewReader(rdr)
				// create temp file and setup tar writer
				fh, err := os.CreateTemp("", "regclient-mod-")
				if err != nil {
					_ = rdr.Close()
					return nil, err
				}
				defer func() {
					_ = fh.Close()
					_ = os.Remove(fh.Name())
				}()
				var tw *tar.Writer
				var gw *gzip.Writer
				var zw *zstd.Writer
				digRaw := desc.DigestAlgo().Digester() // raw/compressed digest
				digUC := desc.DigestAlgo().Digester()  // uncompressed digest
				if dl.desc.MediaType == mediatype.Docker2LayerGzip || dl.desc.MediaType == mediatype.OCI1LayerGzip {
					cw := io.MultiWriter(fh, digRaw.Hash())
					gw = gzip.NewWriter(cw)
					defer gw.Close()
					ucw := io.MultiWriter(gw, digUC.Hash())
					tw = tar.NewWriter(ucw)
				} else if dl.desc.MediaType == mediatype.Docker2LayerZstd || dl.desc.MediaType == mediatype.OCI1LayerZstd {
					cw := io.MultiWriter(fh, digRaw.Hash())
					zw = zstd.NewWriter(cw)
					defer zw.Close()
					ucw := io.MultiWriter(zw, digUC.Hash())
					tw = tar.NewWriter(ucw)
				} else {
					dw := io.MultiWriter(fh, digRaw.Hash(), digUC.Hash())
					tw = tar.NewWriter(dw)
				}
				// iterate over files in the layer
				for {
					th, err := tr.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						return nil, err
					}
					changeFile := unchanged
					var fileRdr io.Reader
					fileRdr = tr
					for _, slf := range dc.stepsLayerFile {
						var changeCur changes
						th, fileRdr, changeCur, err = slf(ctx, rc, rSrc, rTgt, dl, th, fileRdr)
						if err != nil {
							_ = rdr.Close()
							return nil, err
						}
						if changeCur != unchanged {
							changed = true
						}
						if changeCur == deleted {
							changeFile = deleted
							break
						}
					}
					// copy th and tr to temp tar writer file
					if changeFile != deleted {
						empty = false
						err = tw.WriteHeader(th)
						if err != nil {
							_ = rdr.Close()
							return nil, err
						}
						if th.Typeflag == tar.TypeReg && th.Size > 0 {
							_, err := io.CopyN(tw, fileRdr, th.Size)
							if err != nil {
								_ = rdr.Close()
								return nil, err
							}
						}
					}
				}
				if empty {
					dl.mod = deleted
					return dl, nil
				}
				if changed {
					// close to flush remaining content
					err = tw.Close()
					if err != nil {
						_ = rdr.Close()
						return nil, fmt.Errorf("failed to close temporary tar layer: %w", err)
					}
					if gw != nil {
						err = gw.Close()
						if err != nil {
							_ = rdr.Close()
							return nil, fmt.Errorf("failed to close gzip writer: %w", err)
						}
					}
					if zw != nil {
						err = zw.Close()
						if err != nil {
							_ = rdr.Close()
							return nil, fmt.Errorf("failed to close zstd writer: %w", err)
						}
					}
					err = rdr.Close()
					if err != nil {
						return nil, fmt.Errorf("failed to close layer reader: %w", err)
					}
					// replace the current reader and save the digests
					l, err := fh.Seek(0, 1)
					if err != nil {
						return nil, err
					}
					_, err = fh.Seek(0, 0)
					if err != nil {
						return nil, err
					}
					rdr = fh
					desc.Digest = digRaw.Digest()
					desc.Size = l
					dl.newDesc = desc
					dl.ucDigest = digUC.Digest()
					if dl.mod == unchanged {
						dl.mod = replaced
					}
				}
			}
			// if added or replaced, and reader not nil, push blob
			if (dl.mod == added || dl.mod == replaced) && rdr != nil {
				// push the blob and verify the results
				dNew, err := rc.BlobPut(ctx, rTgt, dl.newDesc, rdr)
				if err != nil {
					return nil, err
				}
				err = rdr.Close()
				if err != nil {
					return nil, err
				}
				if dl.newDesc.Digest == "" {
					dl.newDesc.Digest = dNew.Digest
				} else if dl.newDesc.Digest != dNew.Digest {
					return nil, fmt.Errorf("layer digest mismatch, pushed %s, expected %s", dNew.Digest.String(), dl.newDesc.Digest.String())
				}
				if dl.newDesc.Size == 0 {
					dl.newDesc.Size = dNew.Size
				} else if dl.newDesc.Size != dNew.Size {
					return nil, fmt.Errorf("layer size mismatch, pushed %d, expected %d", dNew.Size, dl.newDesc.Size)
				}
			}
			// for unchanged layers, if the repository is different, copy the blob
			if dl.mod == unchanged && !ref.EqualRepository(rSrc, rTgt) {
				err = rc.BlobCopy(ctx, rSrc, rTgt, dl.desc)
				if err != nil {
					return nil, err
				}
			}
			return dl, nil
		})
		if err != nil {
			return rTgt, err
		}
	}

	// push the resulting changed content, both manifests and configs
	err = dagPut(ctx, rc, dc, rSrc, rTgt, dm)
	if err != nil {
		return rTgt, err
	}
	if rTgt.Tag == "" || rTgt.Digest != "" {
		rTgt = rTgt.AddDigest(dm.m.GetDescriptor().Digest.String())
	}
	return rTgt, nil
}

// WithRefTgt sets the target manifest.
// Apply will default to pushing to the same name by digest.
func WithRefTgt(rTgt ref.Ref) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.rTgt = rTgt
		return nil
	}
}

// WithData sets the descriptor data field max size.
// This also strips the data field off descriptors above the max size.
func WithData(maxDataSize int64) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.maxDataSize = maxDataSize
		return nil
	}
}

// WithDigestAlgo sets the digest algorithm for both manifests and layers.
func WithDigestAlgo(algo digest.Algorithm) Opts {
	layerOpt := WithLayerDigestAlgo(algo)
	configOpt := WithConfigDigestAlgo(algo)
	manOpt := WithManifestDigestAlgo(algo)
	return func(dc *dagConfig, dm *dagManifest) error {
		err := layerOpt(dc, dm)
		if err != nil {
			return err
		}
		err = configOpt(dc, dm)
		if err != nil {
			return err
		}
		err = manOpt(dc, dm)
		if err != nil {
			return err
		}
		return nil
	}
}
