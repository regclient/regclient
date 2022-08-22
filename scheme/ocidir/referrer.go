package ocidir

import (
	"context"
	"errors"
	"fmt"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
)

// ReferrerList returns a list of referrers to a given reference
// This is EXPERIMENTAL
func (o *OCIDir) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	config := scheme.ReferrerConfig{}
	for _, opt := range opts {
		opt(&config)
	}
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
	rlTag, err := referrer.FallbackTag(r)
	if err != nil {
		return rl, err
	}
	m, err := o.ManifestGet(ctx, rlTag)
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
		return rl, fmt.Errorf("manifest is not an OCI index: %s", rlTag.CommonName())
	}
	// update referrer list
	rl.Manifest = m
	rl.Descriptors = ociML.Manifests
	rl.Annotations = ociML.Annotations
	rl.Tags = append(rl.Tags, rlTag.Tag)

	// filter resulting descriptor list
	if config.FilterArtifactType != "" && len(rl.Descriptors) > 0 {
		for i := len(rl.Descriptors) - 1; i >= 0; i-- {
			if rl.Descriptors[i].ArtifactType != config.FilterArtifactType {
				rl.Descriptors = append(rl.Descriptors[:i], rl.Descriptors[i+1:]...)
			}
		}
	}
	for k, v := range config.FilterAnnotation {
		if len(rl.Descriptors) > 0 {
			for i := len(rl.Descriptors) - 1; i >= 0; i-- {
				if rl.Descriptors[i].Annotations == nil || rl.Descriptors[i].Annotations[k] != v {
					rl.Descriptors = append(rl.Descriptors[:i], rl.Descriptors[i+1:]...)
				}
			}
		}
	}

	return rl, nil
}

// referrerDelete deletes a referrer associated with a manifest
func (o *OCIDir) referrerDelete(ctx context.Context, r ref.Ref, m manifest.Manifest) error {
	// get refers field
	mRefer, ok := m.(manifest.Refers)
	if !ok {
		return fmt.Errorf("manifest does not support refers: %w", types.ErrUnsupportedMediaType)
	}
	refers, err := mRefer.GetRefers()
	if err != nil {
		return err
	}
	// validate/set referrer descriptor
	if refers == nil || refers.MediaType == "" || refers.Digest == "" || refers.Size <= 0 {
		return fmt.Errorf("refers is not set%.0w", types.ErrNotFound)
	}

	// get descriptor for refers
	rRef := r
	rRef.Tag = ""
	rRef.Digest = refers.Digest.String()

	// pull existing referrer list
	rl, err := o.ReferrerList(ctx, rRef)
	if err != nil {
		return err
	}
	err = rl.Delete(m)
	if err != nil {
		return err
	}

	// push updated referrer list by tag
	rlTag, err := referrer.FallbackTag(rRef)
	if err != nil {
		return err
	}
	if rl.IsEmpty() {
		err = o.TagDelete(ctx, rlTag)
		if err == nil {
			return nil
		}
		// if delete is not supported, fall back to pushing empty list
	}
	return o.ManifestPut(ctx, rlTag, rl.Manifest)
}

// referrerPut pushes a new referrer associated with a given reference
func (o *OCIDir) referrerPut(ctx context.Context, r ref.Ref, m manifest.Manifest) error {
	// get refers field
	mRefer, ok := m.(manifest.Refers)
	if !ok {
		return fmt.Errorf("manifest does not support refers: %w", types.ErrUnsupportedMediaType)
	}
	refers, err := mRefer.GetRefers()
	if err != nil {
		return err
	}
	// validate/set referrer descriptor
	if refers == nil || refers.MediaType == "" || refers.Digest == "" || refers.Size <= 0 {
		return fmt.Errorf("refers is not set%.0w", types.ErrNotFound)
	}

	// get descriptor for refers
	rRef := r
	rRef.Tag = ""
	rRef.Digest = refers.Digest.String()

	// pull existing referrer list
	rl, err := o.ReferrerList(ctx, rRef)
	if err != nil {
		return err
	}
	err = rl.Add(m)
	if err != nil {
		return err
	}

	// push updated referrer list by tag
	rlTag, err := referrer.FallbackTag(rRef)
	if err != nil {
		return err
	}
	return o.ManifestPut(ctx, rlTag, rl.Manifest)
}
