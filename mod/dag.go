package mod

import (
	"archive/tar"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
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
	stepsManifest  []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagManifest) error
	stepsOCIConfig []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagOCIConfig) error
	stepsLayer     []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagLayer, io.ReadCloser) (io.ReadCloser, error)
	stepsLayerFile []func(context.Context, *regclient.RegClient, ref.Ref, ref.Ref, *dagLayer, *tar.Header, io.Reader) (*tar.Header, io.Reader, changes, error)
	maxDataSize    int64
	rTgt           ref.Ref
	forceLayerWalk bool
}

type dagManifest struct {
	mod       changes
	top       bool // indicates the top level manifest (needed for manifest lists)
	origDesc  descriptor.Descriptor
	newDesc   descriptor.Descriptor
	m         manifest.Manifest
	config    *dagOCIConfig
	layers    []*dagLayer
	manifests []*dagManifest
	referrers []*dagManifest
}

type dagOCIConfig struct {
	modified bool
	newDesc  descriptor.Descriptor
	oc       blob.OCIConfig
}

type dagLayer struct {
	mod      changes
	newDesc  descriptor.Descriptor
	ucDigest digest.Digest // uncompressed descriptor
	desc     descriptor.Descriptor
	rSrc     ref.Ref
}

func dagGet(ctx context.Context, rc *regclient.RegClient, rSrc ref.Ref, d descriptor.Descriptor) (*dagManifest, error) {
	var err error
	getOpts := []regclient.ManifestOpts{}
	if d.Digest != "" {
		getOpts = append(getOpts, regclient.WithManifestDesc(d))
	}
	dm := dagManifest{}
	dm.m, err = rc.ManifestGet(ctx, rSrc, getOpts...)
	if err != nil {
		return nil, err
	}
	dm.origDesc = dm.m.GetDescriptor()
	if mi, ok := dm.m.(manifest.Indexer); ok {
		dl, err := mi.GetManifestList()
		if err != nil {
			return nil, err
		}
		for _, desc := range dl {
			rGet := rSrc.SetDigest(desc.Digest.String())
			curMM, err := dagGet(ctx, rc, rGet, desc)
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
		if err != nil && !errors.Is(err, errs.ErrUnsupportedMediaType) {
			return nil, err
		} else if err == nil && slices.Contains(mtKnownConfig, cd.MediaType) {
			oc, err := rc.BlobGetOCIConfig(ctx, rSrc, cd)
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
	// get a list of referrers
	rl, err := rc.ReferrerList(ctx, rSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to get referrers: %w", err)
	}
	for _, desc := range rl.Descriptors {
		// strip referrers metadata from descriptor (annotations and artifact type)
		desc.ArtifactType = ""
		if len(desc.Annotations) > 0 {
			desc.Annotations = nil
		}
		rGet := rSrc.SetDigest(desc.Digest.String())
		curMM, err := dagGet(ctx, rc, rGet, desc)
		if err != nil {
			return nil, err
		}
		dm.referrers = append(dm.referrers, curMM)
	}
	return &dm, nil
}

func dagPut(ctx context.Context, rc *regclient.RegClient, mc dagConfig, rSrc, rTgt ref.Ref, dm *dagManifest) error {
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
			err = dagPut(ctx, rc, mc, rSrc, rTgt, child)
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
				// set data field
				d.Data = mBytes
			} else if mc.maxDataSize >= 0 && d.Size > mc.maxDataSize && len(d.Data) > 0 {
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
			ociI.Manifests = slices.Delete(ociI.Manifests, i, i+1)
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
			oc = dm.config.oc.GetConfig()
			if oc.History != nil {
				iConfig = 0
			}
		}

		// first pass to add/modify layers
		for i, layer := range dm.layers {
			if i >= len(ociM.Layers) && layer.mod != added {
				return fmt.Errorf("manifest does not have enough layers")
			}
			// keep config index aligned
			for iConfig >= 0 && iConfig < len(oc.History) && oc.History[iConfig].EmptyLayer {
				iConfig++
				if iConfig >= len(oc.History) && layer.mod != added {
					return fmt.Errorf("config history does not have enough entries")
				}
			}
			if layer.mod == deleted {
				if iConfig >= 0 {
					iConfig++
				}
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
				br, err := rc.BlobGet(ctx, rTgt, d)
				if err != nil {
					return err
				}
				bBytes, err := io.ReadAll(br)
				_ = br.Close()
				if err != nil {
					return err
				}
				// set data field
				d.Data = bBytes
			} else if mc.maxDataSize >= 0 && d.Size > mc.maxDataSize && len(d.Data) > 0 {
				// strip data fields if above max size
				d.Data = []byte{}
			}
			if layer.mod == added {
				if len(ociM.Layers) == i {
					ociM.Layers = append(ociM.Layers, d)
					if oc.RootFS.DiffIDs != nil && len(oc.RootFS.DiffIDs) == i {
						oc.RootFS.DiffIDs = append(oc.RootFS.DiffIDs, layer.ucDigest)
					}
				} else {
					ociM.Layers = append(ociM.Layers[:i+1], ociM.Layers[i:]...)
					ociM.Layers[i] = d
					if oc.RootFS.DiffIDs != nil && len(oc.RootFS.DiffIDs) >= i {
						oc.RootFS.DiffIDs = append(oc.RootFS.DiffIDs[:i+1], oc.RootFS.DiffIDs[i:]...)
						oc.RootFS.DiffIDs[i] = layer.ucDigest
					}
				}
				newHistory := v1.History{
					Created: &timeStart,
					Comment: "regclient",
				}
				if iConfig < 0 {
					// noop
				} else if iConfig >= len(oc.History) {
					oc.History = append(oc.History, newHistory)
				} else {
					oc.History = append(oc.History[:iConfig+1], oc.History[iConfig:]...)
					oc.History[iConfig] = newHistory
				}
				changed = true
			} else if layer.mod == replaced || !bytes.Equal(ociM.Layers[i].Data, d.Data) {
				ociM.Layers[i] = d
				if oc.RootFS.DiffIDs != nil && len(oc.RootFS.DiffIDs) >= i+1 && layer.ucDigest != "" {
					oc.RootFS.DiffIDs[i] = layer.ucDigest
				}
				changed = true
			}
			if iConfig >= 0 {
				iConfig++
			}
		}
		// second pass in reverse to delete entries
		iConfig = len(oc.History) - 1
		for i := len(dm.layers) - 1; i >= 0; i-- {
			layer := dm.layers[i]
			for iConfig >= 0 && oc.History[iConfig].EmptyLayer {
				iConfig--
			}
			if layer.mod != deleted {
				if iConfig >= 0 {
					iConfig--
				}
				continue
			}
			ociM.Layers = slices.Delete(ociM.Layers, i, i+1)
			if oc.RootFS.DiffIDs != nil && len(oc.RootFS.DiffIDs) >= i+1 {
				oc.RootFS.DiffIDs = slices.Delete(oc.RootFS.DiffIDs, i, i+1)
			}
			if iConfig >= 0 {
				oc.History = slices.Delete(oc.History, iConfig, iConfig+1)
				iConfig--
			}
			changed = true
		}
		if changed && dm.config != nil {
			dm.config.oc.SetConfig(oc)
			dm.config.modified = true
		}
		var cBytes []byte
		if dm.config != nil {
			dm.config.newDesc = dm.config.oc.GetDescriptor()
			cBytes, err = dm.config.oc.RawBody()
			if err != nil {
				return err
			}
			if dm.config.modified {
				cRdr := bytes.NewReader(cBytes)
				_, err = rc.BlobPut(ctx, rTgt, dm.config.newDesc, cRdr)
				if err != nil {
					return err
				}
				ociM.Config.MediaType = dm.config.newDesc.MediaType
				ociM.Config.Digest = dm.config.newDesc.Digest
				ociM.Config.Size = dm.config.newDesc.Size
				changed = true
			} else if !ref.EqualRepository(rSrc, rTgt) {
				err = rc.BlobCopy(ctx, rSrc, rTgt, dm.config.oc.GetDescriptor())
				if err != nil {
					return err
				}
			}
		}
		if dm.config == nil && ociM.Config.Digest != "" && !ref.EqualRepository(rSrc, rTgt) {
			err = rc.BlobCopy(ctx, rSrc, rTgt, ociM.Config)
			if err != nil {
				return err
			}
		}
		// handle config data field
		if ociM.Config.Size <= mc.maxDataSize || (mc.maxDataSize < 0 && len(ociM.Config.Data) > 0) {
			// if config was not loaded into memory (e.g. artifact), load it now
			if cBytes == nil {
				cRdr, err := rc.BlobGet(ctx, rTgt, ociM.Config)
				if err != nil {
					return err
				}
				cBytes, err = io.ReadAll(cRdr)
				_ = cRdr.Close()
				if err != nil {
					return err
				}
			}
			if !bytes.Equal(ociM.Config.Data, cBytes) {
				ociM.Config.Data = cBytes
				changed = true
			}
		} else if mc.maxDataSize >= 0 && ociM.Config.Size > mc.maxDataSize && len(ociM.Config.Data) > 0 {
			// strip data fields if above max size
			ociM.Config.Data = []byte{}
			changed = true
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
	// update descriptor and update subject descriptor on all referrers
	if dm.mod == replaced || dm.mod == added {
		dm.newDesc = dm.m.GetDescriptor()
	}
	if ref.EqualRepository(rSrc, rTgt) {
		// only update referrers when modifying a manifest in the same repository
		for i := range dm.referrers {
			if dm.referrers[i].mod == deleted || !(dm.mod == replaced || dm.mod == added || dm.referrers[i].mod == added) {
				continue
			}
			sm, ok := dm.referrers[i].m.(manifest.Subjecter)
			if !ok {
				return fmt.Errorf("referrer does not support subject field, mt=%s", dm.referrers[i].m.GetDescriptor().MediaType)
			}
			d := dm.m.GetDescriptor()
			err = sm.SetSubject(&d)
			if err != nil {
				return fmt.Errorf("failed to set subject: %w", err)
			}
			if dm.referrers[i].mod == unchanged {
				dm.referrers[i].mod = replaced
			}
			dm.referrers[i].newDesc = dm.referrers[i].m.GetDescriptor()
		}
		// recursively push referrers
		for _, child := range dm.referrers {
			err = dagPut(ctx, rc, mc, rSrc, rTgt, child)
			if err != nil {
				return err
			}
		}
	}
	// push manifest
	if dm.mod == replaced || dm.mod == added || (dm.mod == unchanged && !ref.EqualRepository(rSrc, rTgt)) {
		mpOpts := []regclient.ManifestOpts{}
		rPut := rTgt
		if !dm.top {
			mpOpts = append(mpOpts, regclient.WithManifestChild())
			rPut = rPut.SetDigest(cmp.Or(dm.newDesc.Digest.String(), dm.origDesc.Digest.String()))
		} else if rPut.Tag == "" || rPut.Digest != "" {
			// update digest, prefer newDesc if set
			rPut = rPut.AddDigest(cmp.Or(dm.newDesc.Digest.String(), dm.origDesc.Digest.String()))
		}
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
