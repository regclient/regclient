package reg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
)

// ReferrerList returns a list of referrers to a given reference
// This is EXPERIMENTAL
func (reg *Reg) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	rl := referrer.ReferrerList{
		Ref:  r,
		Tags: []string{},
	}
	// if ref is a tag, run a head request for the digest
	if r.Digest == "" {
		m, err := reg.ManifestHead(ctx, r)
		if err != nil {
			return rl, err
		}
		r.Digest = m.GetDescriptor().Digest.String()
	}

	// TODO: attempt to call the referrer API when approved by OCI
	// attempt to call the referrer extension API
	rlAPI, err := reg.referrerListExtAPI(ctx, r)
	if err == nil {
		return rlAPI, nil
	}

	// fall back to pulling by tag
	dig, err := digest.Parse(r.Digest)
	if err != nil {
		return rl, fmt.Errorf("failed to parse digest for referrers: %w", err)
	}
	rr := r
	rr.Digest = ""
	rr.Tag = fmt.Sprintf("%s-%s", dig.Algorithm(), stringMax(dig.Hex(), 64))
	m, err := reg.ManifestGet(ctx, rr)
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

func (reg *Reg) referrerListExtAPI(ctx context.Context, r ref.Ref) (referrer.ReferrerList, error) {
	rl := referrer.ReferrerList{
		Ref:  r,
		Tags: []string{},
	}
	query := url.Values{}
	query.Set("digest", r.Digest)
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "GET",
				Repository: r.Repository,
				Path:       "_oci/artifacts/referrers",
				Query:      query,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return rl, fmt.Errorf("failed to get referrers %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return rl, fmt.Errorf("failed to get referrers %s: %w", r.CommonName(), reghttp.HTTPError(resp.HTTPResponse().StatusCode))
	}

	// read manifest
	rawBody, err := io.ReadAll(resp)
	if err != nil {
		return rl, fmt.Errorf("error reading referrers for %s: %w", r.CommonName(), err)
	}

	m, err := manifest.New(
		manifest.WithRef(r),
		manifest.WithHeader(resp.HTTPResponse().Header),
		manifest.WithRaw(rawBody),
	)
	if err != nil {
		return rl, err
	}
	ociML, ok := m.GetOrig().(v1.Index)
	if !ok {
		return rl, fmt.Errorf("unexpected manifest type for referrers: %s, %w", m.GetDescriptor().MediaType, types.ErrUnsupportedMediaType)
	}
	rl.Manifest = m
	rl.Descriptors = ociML.Manifests
	rl.Annotations = ociML.Annotations
	return rl, nil
}

// ReferrerPut pushes a new referrer associated with a given reference
// This is EXPERIMENTAL
func (reg *Reg) ReferrerPut(ctx context.Context, r ref.Ref, m manifest.Manifest) error {
	// get descriptor for ref
	mRef, err := reg.ManifestHead(ctx, r)
	if err != nil {
		return err
	}
	if r.Digest == "" {
		r.Digest = mRef.GetDescriptor().Digest.String()
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

	// push manifest by digest
	rPut := r
	rPut.Tag = ""
	rPut.Digest = m.GetDescriptor().Digest.String()
	err = reg.ManifestPut(ctx, rPut, m, scheme.WithManifestChild())
	if err != nil {
		return err
	}

	// if referrer API is available, return
	if reg.referrerPing(ctx, r) {
		return nil
	}

	// fallback to using tag schema for refers
	rl, err := reg.ReferrerList(ctx, r)
	if err != nil {
		return err
	}
	rlM, ok := rl.Manifest.GetOrig().(v1.Index)
	if !ok {
		return fmt.Errorf("referrer list manifest is not an OCI index for %s", r.CommonName())
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
	return reg.ManifestPut(ctx, rr, rl.Manifest)
}

func (reg *Reg) referrerPing(ctx context.Context, r ref.Ref) bool {
	// TODO: add ping for OCI path when approved
	query := url.Values{}
	query.Set("digest", r.Digest)
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "GET",
				Repository: r.Repository,
				Path:       "_oci/artifacts/referrers",
				Query:      query,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return false
	}
	defer resp.Close()
	return resp.HTTPResponse().StatusCode == 200
}

func stringMax(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
