// Package mod changes an image according to the requested modifications.
package mod

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
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
	// whitelist of tar media types
	mtWLTar = []string{
		mediatype.Docker2LayerGzip,
		mediatype.OCI1Layer,
		mediatype.OCI1LayerGzip,
		mediatype.OCI1LayerZstd,
	}
	mtWLConfig = []string{
		mediatype.Docker2ImageConfig,
		mediatype.OCI1ImageConfig,
	}
)

// Apply applies a set of modifications to an image (manifest, configs, and layers).
func Apply(ctx context.Context, rc *regclient.RegClient, rSrc ref.Ref, opts ...Opts) (ref.Ref, error) {
	// check for the various types of mods (manifest, config, layer)
	// some may span like copying layers from config to manifest
	// run changes in order (deleting layers before pulling and changing a layer)
	// to span steps, some changes will output other mods to apply
	// e.g. layer hash changing in config, or a deleted layer from the config deleting from the manifest
	// do I need to store a DAG in memory with pointers back to parents and modified bool, so change to digest can be rippled up and modified objects are pushed?

	// pull the image metadata into a DAG
	dm, err := dagGet(ctx, rc, rSrc, descriptor.Descriptor{})
	if err != nil {
		return rSrc, err
	}
	dm.top = true

	// load the options
	rTgt := rSrc
	rTgt.Tag = ""
	rTgt.Digest = ""
	dc := dagConfig{
		stepsManifest:  []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagManifest) error{},
		stepsOCIConfig: []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagOCIConfig) error{},
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
	if len(dc.stepsLayerFile) > 0 || !ref.EqualRepository(rSrc, rTgt) || dc.forceLayerWalk {
		err = dagWalkLayers(dm, func(dl *dagLayer) (*dagLayer, error) {
			rSrc := rSrc
			if dl.rSrc.IsSet() {
				rSrc = dl.rSrc
			}
			if dl.mod == deleted || len(dl.desc.URLs) > 0 {
				// skip deleted or external layers
				return dl, nil
			}
			if len(dc.stepsLayerFile) > 0 && dl.mod != deleted && inListStr(dl.desc.MediaType, mtWLTar) {
				br, err := rc.BlobGet(ctx, rSrc, dl.desc)
				if err != nil {
					return nil, err
				}
				defer br.Close()
				changed := false
				empty := true
				// setup tar reader to process layer
				dr, err := archive.Decompress(br)
				if err != nil {
					return nil, err
				}
				tr := tar.NewReader(dr)
				// create temp file
				fh, err := os.CreateTemp("", "regclient-mod-")
				if err != nil {
					return nil, err
				}
				defer fh.Close()
				defer os.Remove(fh.Name())
				// create tar writer, optional recompress
				var tw *tar.Writer
				var gw *gzip.Writer
				digRaw := digest.Canonical.Digester() // raw/compressed digest
				digUC := digest.Canonical.Digester()  // uncompressed digest
				if dl.desc.MediaType == mediatype.Docker2LayerGzip || dl.desc.MediaType == mediatype.OCI1LayerGzip {
					cw := io.MultiWriter(fh, digRaw.Hash())
					gw = gzip.NewWriter(cw)
					defer gw.Close()
					ucw := io.MultiWriter(gw, digUC.Hash())
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
					var rdr io.Reader
					rdr = tr
					for _, slf := range dc.stepsLayerFile {
						var changeCur changes
						th, rdr, changeCur, err = slf(ctx, rc, rSrc, rTgt, dl, th, rdr)
						if err != nil {
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
							return nil, err
						}
						if th.Typeflag == tar.TypeReg && th.Size > 0 {
							_, err := io.CopyN(tw, rdr, th.Size)
							if err != nil {
								return nil, err
							}
						}
					}
				}
				err = br.Close()
				if err != nil {
					return nil, fmt.Errorf("failed to close blob: %w", err)
				}
				br = nil
				if empty || dl.mod == deleted {
					dl.mod = deleted
					return dl, nil
				}
				if changed {
					// if modified, push blob
					err = tw.Close()
					if err != nil {
						return nil, fmt.Errorf("failed to close temporary tar layer: %w", err)
					}
					if gw != nil {
						err = gw.Close()
						if err != nil {
							return nil, fmt.Errorf("failed to close gzip writer: %w", err)
						}
					}
					// get the file size
					l, err := fh.Seek(0, 1)
					if err != nil {
						return nil, err
					}
					dl.newDesc = dl.desc
					dl.newDesc.Digest = digRaw.Digest()
					dl.newDesc.Size = l
					dl.ucDigest = digUC.Digest()
					_, err = fh.Seek(0, 0)
					if err != nil {
						return nil, err
					}
					_, err = rc.BlobPut(ctx, rTgt, dl.newDesc, fh)
					if err != nil {
						return nil, err
					}
					dl.mod = replaced
				}
			}
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

	err = dagPut(ctx, rc, dc, rSrc, rTgt, dm)
	if err != nil {
		return rTgt, err
	}
	if rTgt.Tag == "" {
		rTgt.Digest = dm.m.GetDescriptor().Digest.String()
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

func inListStr(str string, list []string) bool {
	for _, s := range list {
		if str == s {
			return true
		}
	}
	return false
}

func eqStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
