package ocidir

import (
	"context"
	"fmt"
	"io/fs"
	"path"

	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func (o *OCIDir) Close(ctx context.Context, r ref.Ref) error {
	if !o.gc {
		return nil
	}

	// perform GC
	dl := map[string]bool{}
	// recurse through index, manifests, and blob lists, generating a digest list
	index, err := o.readIndex(r)
	if err != nil {
		return err
	}
	im, err := manifest.New(manifest.WithOrig(index))
	if err != nil {
		return err
	}
	err = o.closeProcManifest(ctx, r, im, &dl)
	if err != nil {
		return err
	}

	// go through filesystem digest list, removing entries not seen in recursive pass
	blobsPath := path.Join(r.Path, "blobs")
	blobDirs, err := fs.ReadDir(o.fs, blobsPath)
	if err != nil {
		return err
	}
	for _, blobDir := range blobDirs {
		if !blobDir.IsDir() {
			// should this warn or delete unexpected files in the blobs folder?
			continue
		}
		digestFiles, err := fs.ReadDir(o.fs, path.Join(blobsPath, blobDir.Name()))
		if err != nil {
			return err
		}
		for _, digestFile := range digestFiles {
			digest := fmt.Sprintf("%s:%s", blobDir.Name(), digestFile.Name())
			if !dl[digest] {
				o.log.WithFields(logrus.Fields{
					"digest": digest,
				}).Debug("ocidir garbage collect")
				// delete
				o.fs.Remove(path.Join(blobsPath, blobDir.Name(), digestFile.Name()))
			}
		}
	}
	return nil
}

func (o *OCIDir) closeProcManifest(ctx context.Context, r ref.Ref, m manifest.Manifest, dl *map[string]bool) error {
	if m.IsList() {
		// go through manifest list, updating dl, and recursively processing nested manifests
		ml, err := m.GetDescriptorList()
		if err != nil {
			return err
		}
		for _, cur := range ml {
			cr, _ := ref.New(r.CommonName())
			cr.Tag = ""
			cr.Digest = cur.Digest.String()
			(*dl)[cr.Digest] = true
			cm, err := o.ManifestGet(ctx, cr)
			if err != nil {
				// ignore errors in case a manifest has been deleted or sparse copy
				continue
			}
			err = o.closeProcManifest(ctx, cr, cm, dl)
			if err != nil {
				return err
			}
		}
	} else {
		// get config from manifest if it exists
		cd, err := m.GetConfigDescriptor()
		if err == nil {
			(*dl)[cd.Digest.String()] = true
		}
		// finally add all layers to digest list
		layers, err := m.GetLayers()
		if err != nil {
			return err
		}
		for _, layer := range layers {
			(*dl)[layer.Digest.String()] = true
		}
	}
	return nil
}
