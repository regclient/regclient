package ocidir

import (
	"context"
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
)

const (
	annotType = "org.opencontainers.artifact.type"
)

// ReferrerList returns a list of referrers to a given reference
// This is EXPERIMENTAL
func (o *OCIDir) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	rl := referrer.ReferrerList{
		Ref:  r,
		Tags: []string{},
	}
	// if ref is a tag, run a head request for the digest
	if r.Digest == "" {
		m, err := o.ManifestHead(ctx, r)
		if err != nil {
			return rl, err
		}
		r.Digest = m.GetDescriptor().Digest.String()
	}

	// pull referrer list by tag
	dig, err := digest.Parse(r.Digest)
	if err != nil {
		return rl, fmt.Errorf("failed to parse digest for referrers: %w", err)
	}
	rr := r
	rr.Digest = ""
	rr.Tag = fmt.Sprintf("%s-%s", dig.Algorithm(), stringMax(dig.Hex(), 64))
	m, err := o.ManifestGet(ctx, rr)
	if err != nil {
		if errors.Is(err, types.ErrNotFound) {
			// empty list, initialize a new manifest
			rl.Manifest, err = manifest.New(manifest.WithOrig(v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: types.MediaTypeOCI1ManifestList,
			}))
			if err != nil {
				return rl, err
			}
			return rl, nil
		}
		return rl, err
	}
	ociML, ok := m.GetOrig().(v1.Index)
	if !ok {
		return rl, fmt.Errorf("manifest is not an OCI index: %s", rr.CommonName())
	}
	// TODO: filter resulting manifest entries
	// return resulting index
	rl.Manifest = m
	rl.Descriptors = ociML.Manifests
	rl.Annotations = ociML.Annotations
	rl.Tags = append(rl.Tags, rr.Tag)
	return rl, nil
}

// ReferrerPut pushes a new referrer associated with a given reference
// This is EXPERIMENTAL
func (o *OCIDir) ReferrerPut(ctx context.Context, r ref.Ref, m manifest.Manifest) error {
	// get descriptor for ref
	mRef, err := o.ManifestHead(ctx, r)
	if err != nil {
		return err
	}
	if r.Digest == "" {
		r.Digest = mRef.GetDescriptor().Digest.String()
	}

	// pull existing referrer list
	rl, err := o.ReferrerList(ctx, r)
	if err != nil {
		return err
	}
	rlM, ok := rl.Manifest.GetOrig().(v1.Index)
	if !ok {
		return fmt.Errorf("referrer list manifest is not an OCI index for %s", r.CommonName())
	}
	// set referrer if missing
	mRefer, ok := m.(manifest.Referrer)
	if !ok {
		return fmt.Errorf("manifest does not support refers: %w", types.ErrUnsupportedMediaType)
	}
	refers, err := mRefer.GetRefers()
	if err != nil {
		return err
	}
	// validate/set referrer descriptor
	mRefDesc := mRef.GetDescriptor()
	if refers == nil || refers.MediaType != mRefDesc.MediaType || refers.Digest != mRefDesc.Digest || refers.Size != mRefDesc.Size {
		err = mRefer.SetRefers(&mRefDesc)
		if err != nil {
			return err
		}
	}
	// push manifest (by digest, with child flag)
	rPut := r
	rPut.Tag = ""
	rPut.Digest = m.GetDescriptor().Digest.String()
	err = o.ManifestPut(ctx, rPut, m, scheme.WithManifestChild())
	if err != nil {
		return err
	}

	// if entry already exists, return
	mDesc := m.GetDescriptor()
	for _, d := range rlM.Manifests {
		if d.Digest == mDesc.Digest {
			return nil
		}
	}
	// update descriptor, pulling up artifact type and annotations
	switch mOrig := m.GetOrig().(type) {
	case v1.ArtifactManifest:
		mDesc.Annotations = mOrig.Annotations
		mDesc.ArtifactType = mOrig.ArtifactType
	case v1.Manifest:
		mDesc.Annotations = mOrig.Annotations
		mDesc.ArtifactType = mOrig.Config.MediaType
	default:
		// other types are not supported
		return fmt.Errorf("invalid manifest for referrer \"%t\": %w", m.GetOrig(), types.ErrUnsupportedMediaType)
	}
	// append descriptor to index
	rlM.Manifests = append(rlM.Manifests, mDesc)
	err = rl.Manifest.SetOrig(rlM)
	if err != nil {
		return err
	}

	// push updated referrer list by tag
	dig, err := digest.Parse(r.Digest)
	if err != nil {
		return fmt.Errorf("failed to parse digest for referrers: %w", err)
	}
	rr := r
	rr.Digest = ""
	rr.Tag = fmt.Sprintf("%s-%s", dig.Algorithm(), stringMax(dig.Hex(), 64))
	return o.ManifestPut(ctx, rr, rl.Manifest)
}

func stringMax(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
