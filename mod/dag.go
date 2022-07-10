package mod

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

type changes int

const (
	unchanged changes = iota
	added
	replaced
	deleted
)

type dagConfig struct {
	stepsManifest  []func(context.Context, *regclient.RegClient, ref.Ref, *dagManifest) error
	stepsOCIConfig []func(context.Context, *regclient.RegClient, ref.Ref, *dagOCIConfig) error
	stepsLayer     []func(context.Context, *regclient.RegClient, ref.Ref, *dagLayer) error
	stepsLayerFile []func(context.Context, *regclient.RegClient, ref.Ref, *dagLayer, *tar.Header, io.Reader) (*tar.Header, io.Reader, changes, error)
	maxDataSize    int64
}

type dagManifest struct {
	mod       changes
	top       bool // indicates the top level manifest (needed for manifest lists)
	newDesc   types.Descriptor
	m         manifest.Manifest
	config    *dagOCIConfig
	layers    []*dagLayer
	manifests []*dagManifest
}

type dagOCIConfig struct {
	modified bool
	newDesc  types.Descriptor
	oc       blob.OCIConfig
}

type dagLayer struct {
	mod      changes
	newDesc  types.Descriptor
	ucDigest digest.Digest // uncompressed descriptor
	desc     types.Descriptor
}

func dagGet(ctx context.Context, rc *regclient.RegClient, r ref.Ref, d types.Descriptor) (*dagManifest, error) {
	var err error
	getOpts := []regclient.ManifestOpts{}
	if d.Digest != "" {
		getOpts = append(getOpts, regclient.ManifestWithDesc(d))
	}
	dm := dagManifest{}
	dm.m, err = rc.ManifestGet(ctx, r, getOpts...)
	if err != nil {
		return nil, err
	}
	if mi, ok := dm.m.(manifest.Indexer); ok {
		dl, err := mi.GetManifestList()
		if err != nil {
			return nil, err
		}
		for _, desc := range dl {
			curMM, err := dagGet(ctx, rc, r, desc)
			if err != nil {
				return nil, err
			}
			dm.manifests = append(dm.manifests, curMM)
		}
	}
	if mi, ok := dm.m.(manifest.Imager); ok {
		// pull config
		doc := dagOCIConfig{}
		cd, err := mi.GetConfig()
		if err != nil && !errors.Is(err, types.ErrUnsupportedMediaType) {
			return nil, err
		} else if err == nil {
			oc, err := rc.BlobGetOCIConfig(ctx, r, cd)
			if err != nil {
				return nil, err
			}
			doc.oc = oc
			dm.config = &doc
		}
		// init layers
		layers, err := mi.GetLayers()
		if err != nil {
			return nil, err
		}
		for _, layer := range layers {
			dl := dagLayer{
				desc: layer,
			}
			dm.layers = append(dm.layers, &dl)
		}
	}
	return &dm, nil
}

func dagPut(ctx context.Context, rc *regclient.RegClient, mc dagConfig, r ref.Ref, dm *dagManifest) error {
	var err error
	// recursively push children to get new digests to include in the modified manifest
	om := dm.m.GetOrig()
	changed := false
	if dm.m.IsList() {
		ociI, err := manifest.OCIIndexFromAny(om)
		if err != nil {
			return err
		}
		// two passes through manifests, first to add/update entries
		for i, child := range dm.manifests {
			if i >= len(ociI.Manifests) && child.mod != added {
				return fmt.Errorf("manifest does not have enough child manifests")
			}
			if child.mod == deleted {
				continue
			}
			// recursively make changes
			err = dagPut(ctx, rc, mc, r, child)
			if err != nil {
				return err
			}
			// handle data field
			d := child.m.GetDescriptor()
			if child.mod != unchanged && child.newDesc.Digest != "" {
				d = child.newDesc
			}
			if d.Size <= mc.maxDataSize || (mc.maxDataSize < 0 && len(d.Data) > 0) {
				// if data field should be set
				// retrieve the body
				mBytes, err := dm.m.RawBody()
				if err != nil {
					return err
				}
				// base64 encode, update data field
				mB64 := []byte(base64.StdEncoding.EncodeToString(mBytes))
				d.Data = mB64
			} else if d.Size > mc.maxDataSize && len(d.Data) > 0 {
				// strip data fields if above max size
				d.Data = []byte{}
			}
			// update the descriptor list
			if child.mod == added {
				// TODO: need to set the platform and any annotations
				if len(ociI.Manifests) == i {
					ociI.Manifests = append(ociI.Manifests, d)
				} else {
					ociI.Manifests = append(ociI.Manifests[:i+1], ociI.Manifests[i:]...)
					ociI.Manifests[i] = d
				}
				changed = true
			} else if child.mod == replaced || !bytes.Equal(ociI.Manifests[i].Data, d.Data) {
				ociI.Manifests[i].Digest = d.Digest
				ociI.Manifests[i].Size = d.Size
				ociI.Manifests[i].MediaType = d.MediaType
				ociI.Manifests[i].Data = d.Data
				changed = true
			}
		}
		// second pass in reverse to delete entries
		for i := len(dm.manifests) - 1; i >= 0; i-- {
			child := dm.manifests[i]
			if child.mod != deleted {
				continue
			}
			ociI.Manifests = append(ociI.Manifests[:i], ociI.Manifests[i+1:]...)
			changed = true
		}
		err = manifest.OCIIndexToAny(ociI, &om)
		if err != nil {
			return err
		}
	} else { // !mm.m.IsList()
		ociM, err := manifest.OCIManifestFromAny(om)
		if err != nil {
			return err
		}
		oc := v1.Image{}
		iConfig := -1
		if dm.config != nil {
			iConfig = 0
			oc = dm.config.oc.GetConfig()
		}

		// first pass to add/modify layers
		for i, layer := range dm.layers {
			if i >= len(ociM.Layers) && layer.mod != added {
				return fmt.Errorf("manifest does not have enough layers")
			}
			// keep config index aligned
			for iConfig >= 0 && oc.History[iConfig].EmptyLayer {
				iConfig++
				if iConfig >= len(oc.History) {
					return fmt.Errorf("config history does not have enough entries")
				}
			}
			if layer.mod == deleted {
				iConfig++
				continue
			}
			// handle data field
			d := layer.desc
			if layer.mod != unchanged && layer.newDesc.Digest != "" {
				d = layer.newDesc
			}
			if d.Size <= mc.maxDataSize || (mc.maxDataSize < 0 && len(d.Data) > 0) {
				// if data field should be set
				// retrieve the body
				br, err := rc.BlobGet(ctx, r, d)
				if err != nil {
					return err
				}
				bBytes, err := io.ReadAll(br)
				if err != nil {
					return err
				}
				// base64 encode, update data field
				bB64 := []byte(base64.StdEncoding.EncodeToString(bBytes))
				d.Data = bB64
			} else if d.Size > mc.maxDataSize && len(d.Data) > 0 {
				// strip data fields if above max size
				d.Data = []byte{}
			}
			if layer.mod == added {
				if len(ociM.Layers) == i {
					ociM.Layers = append(ociM.Layers, d)
					oc.RootFS.DiffIDs = append(oc.RootFS.DiffIDs, layer.ucDigest)
				} else {
					ociM.Layers = append(ociM.Layers[:i+1], ociM.Layers[i:]...)
					ociM.Layers[i] = d
					oc.RootFS.DiffIDs = append(oc.RootFS.DiffIDs[:i+1], oc.RootFS.DiffIDs[i:]...)
					oc.RootFS.DiffIDs[i] = layer.ucDigest
				}
				newHistory := v1.History{
					Created: &timeNow,
					Comment: "regclient",
				}
				if iConfig < 0 {
					// noop
				} else if len(oc.History) == iConfig {
					oc.History = append(oc.History, newHistory)
				} else {
					oc.History = append(oc.History[:iConfig+1], oc.History[iConfig:]...)
					oc.History[iConfig] = newHistory
				}
				changed = true
			} else if layer.mod == replaced || !bytes.Equal(ociM.Layers[i].Data, d.Data) {
				ociM.Layers[i] = d
				if layer.ucDigest != "" {
					oc.RootFS.DiffIDs[i] = layer.ucDigest
				}
				changed = true
			}
			iConfig++
		}
		// second pass in reverse to delete entries
		iConfig = len(oc.History) - 1
		for i := len(dm.layers) - 1; i >= 0; i-- {
			layer := dm.layers[i]
			for iConfig >= 0 && oc.History[iConfig].EmptyLayer {
				iConfig--
			}
			if layer.mod != deleted {
				iConfig--
				continue
			}
			ociM.Layers = append(ociM.Layers[:i], ociM.Layers[i+1:]...)
			oc.RootFS.DiffIDs = append(oc.RootFS.DiffIDs[:i], oc.RootFS.DiffIDs[i+1:]...)
			if iConfig >= 0 {
				oc.History = append(oc.History[:iConfig], oc.History[iConfig+1:]...)
			}
			changed = true
			iConfig--
		}
		if changed && dm.config != nil {
			dm.config.oc.SetConfig(oc)
			dm.config.modified = true
		}
		if dm.config != nil {
			dm.config.newDesc = dm.config.oc.GetDescriptor()
			cBytes, err := dm.config.oc.RawBody()
			if err != nil {
				return err
			}
			if dm.config.modified {
				cRdr := bytes.NewReader(cBytes)
				_, err = rc.BlobPut(ctx, r, dm.config.newDesc, cRdr)
				if err != nil {
					return err
				}
				ociM.Config.MediaType = dm.config.newDesc.MediaType
				ociM.Config.Digest = dm.config.newDesc.Digest
				ociM.Config.Size = dm.config.newDesc.Size
				changed = true
			}
			// handle config data field
			if ociM.Config.Size <= mc.maxDataSize || (mc.maxDataSize < 0 && len(ociM.Config.Data) > 0) {
				// if data field should be set
				// base64 encode, update data field
				cB64 := []byte(base64.StdEncoding.EncodeToString(cBytes))
				if !bytes.Equal(ociM.Config.Data, cB64) {
					ociM.Config.Data = cB64
					changed = true
				}
			} else if ociM.Config.Size > mc.maxDataSize && len(ociM.Config.Data) > 0 {
				// strip data fields if above max size
				ociM.Config.Data = []byte{}
				changed = true
			}
		}

		if changed {
			err = manifest.OCIManifestToAny(ociM, &om)
			if err != nil {
				return err
			}
		}
	}
	if changed {
		dm.mod = replaced
		err = dm.m.SetOrig(om)
		if err != nil {
			return err
		}
	}
	if dm.mod == replaced || dm.mod == added {
		dm.newDesc = dm.m.GetDescriptor()
		mpOpts := []scheme.ManifestOpts{}
		if !dm.top {
			mpOpts = append(mpOpts, scheme.WithManifestChild())
		}
		rPut := r
		rPut.Tag = ""
		rPut.Digest = dm.newDesc.Digest.String()
		err = rc.ManifestPut(ctx, rPut, dm.m, mpOpts...)
		if err != nil {
			return err
		}
	}

	return nil
}

func dagWalkManifests(dm *dagManifest, fn func(*dagManifest) (*dagManifest, error)) error {
	if dm.manifests != nil {
		for _, child := range dm.manifests {
			err := dagWalkManifests(child, fn)
			if err != nil {
				return err
			}
		}
	}
	mmNew, err := fn(dm)
	if err != nil {
		return err
	}
	*dm = *mmNew
	return nil
}

func dagWalkOCIConfig(dm *dagManifest, fn func(*dagOCIConfig) (*dagOCIConfig, error)) error {
	if dm.manifests != nil {
		for _, child := range dm.manifests {
			err := dagWalkOCIConfig(child, fn)
			if err != nil {
				return err
			}
		}
	}
	if dm.config != nil {
		docNew, err := fn(dm.config)
		if err != nil {
			return err
		}
		dm.config = docNew
	}
	return nil
}

func dagWalkLayers(dm *dagManifest, fn func(*dagLayer) (*dagLayer, error)) error {
	var err error
	if dm.manifests != nil {
		for _, child := range dm.manifests {
			err = dagWalkLayers(child, fn)
			if err != nil {
				return err
			}
		}
	}
	if dm.layers != nil {
		for i, layer := range dm.layers {
			if layer.mod == deleted {
				continue
			}
			mlNew, err := fn(layer)
			if err != nil {
				return err
			}
			dm.layers[i] = mlNew
		}
	}
	return nil
}
