package ocidir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/internal/wraperr"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

// ManifestDelete removes a manifest, including all tags that point to that manifest
func (o *OCIDir) ManifestDelete(ctx context.Context, r ref.Ref) error {
	if r.Digest == "" {
		return wraperr.New(fmt.Errorf("digest required to delete manifest, reference %s", r.CommonName()), types.ErrMissingDigest)
	}

	// get index
	changed := false
	index, err := o.readIndex(r)
	if err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}
	for i := len(index.Manifests) - 1; i >= 0; i-- {
		// remove matching entry from index
		if r.Digest != "" && index.Manifests[i].Digest.String() == r.Digest {
			changed = true
			index.Manifests = append(index.Manifests[:i], index.Manifests[i+1:]...)
		}
	}
	// push manifest back out
	if changed {
		err = o.writeIndex(r, index)
		if err != nil {
			return fmt.Errorf("failed to write index: %w", err)
		}
	}

	// delete from filesystem like a registry would do
	d := digest.Digest(r.Digest)
	file := path.Join(r.Path, "blobs", d.Algorithm().String(), d.Encoded())
	err = o.fs.Remove(file)
	if err != nil {
		return fmt.Errorf("failed to delete manifest: %w", err)
	}
	o.refMod(r)
	return nil
}

// ManifestGet retrieves a manifest from a repository
func (o *OCIDir) ManifestGet(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	index, err := o.readIndex(r)
	if err != nil {
		return nil, fmt.Errorf("unable to read oci index: %w", err)
	}
	if r.Digest == "" && r.Tag == "" {
		r.Tag = "latest"
	}
	desc, err := indexGet(index, r)
	if err != nil {
		if r.Digest != "" {
			desc.Digest = digest.Digest(r.Digest)
		} else {
			return nil, err
		}
	}
	if desc.Digest == "" {
		return nil, types.ErrNotFound
	}
	file := path.Join(r.Path, "blobs", desc.Digest.Algorithm().String(), desc.Digest.Encoded())
	fd, err := o.fs.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest: %w", err)
	}
	defer fd.Close()
	mb, err := io.ReadAll(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}
	if desc.Size == 0 {
		desc.Size = int64(len(mb))
	}
	o.log.WithFields(logrus.Fields{
		"ref":  r.CommonName(),
		"file": file,
	}).Debug("retrieved manifest")
	return manifest.New(
		manifest.WithRef(r),
		manifest.WithDesc(desc),
		manifest.WithRaw(mb),
	)
}

// ManifestHead gets metadata about the manifest (existence, digest, mediatype, size)
func (o *OCIDir) ManifestHead(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	index, err := o.readIndex(r)
	if err != nil {
		return nil, err
	}
	var desc types.Descriptor
	if r.Digest == "" && r.Tag == "" {
		r.Tag = "latest"
	}
	for _, im := range index.Manifests {
		if r.Digest != "" && im.Digest.String() == r.Digest {
			desc = im
		} else if name, ok := im.Annotations[aRefName]; ok && name == r.Tag {
			desc = im
			break
		}
	}
	if desc.Digest == "" || desc.MediaType == "" {
		return nil, types.ErrNotFound
	}

	return manifest.New(
		manifest.WithRef(r),
		manifest.WithDesc(desc),
	)
}

// ManifestPut sends a manifest to the repository
func (o *OCIDir) ManifestPut(ctx context.Context, r ref.Ref, m manifest.Manifest, opts ...scheme.ManifestOpts) error {
	config := scheme.ManifestConfig{}
	for _, opt := range opts {
		opt(&config)
	}
	if !config.Child && r.Digest == "" && r.Tag == "" {
		r.Tag = "latest"
	}

	index, err := o.readIndex(r)
	if err != nil {
		index = indexCreate()
	}
	desc := m.GetDescriptor()
	b, err := m.RawBody()
	if err != nil {
		return fmt.Errorf("could not serialize manifest: %w", err)
	}
	if r.Tag == "" {
		// force digest to match manifest value
		r.Digest = desc.Digest.String()
	}
	if r.Tag != "" {
		desc.Annotations = map[string]string{
			aRefName: r.Tag,
		}
	}
	// create manifest CAS file
	dir := path.Join(r.Path, "blobs", desc.Digest.Algorithm().String())
	err = rwfs.MkdirAll(o.fs, dir, 0777)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("failed creating %s: %w", dir, err)
	}
	file := path.Join(dir, desc.Digest.Encoded())
	fd, err := o.fs.Create(file)
	if err != nil {
		return fmt.Errorf("failed to create manifest: %w", err)
	}
	defer fd.Close()
	_, err = fd.Write(b)
	if err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	// replace existing tag or create a new entry
	if !config.Child {
		err := indexSet(&index, r, desc)
		if err != nil {
			return fmt.Errorf("failed to update index: %w", err)
		}
		err = o.writeIndex(r, index)
		if err != nil {
			return fmt.Errorf("failed to write index: %w", err)
		}
	}
	o.refMod(r)
	o.log.WithFields(logrus.Fields{
		"ref":  r.CommonName(),
		"file": file,
	}).Debug("pushed manifest")
	return nil
}
