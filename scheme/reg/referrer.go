package reg

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
	"github.com/sirupsen/logrus"
)

const (
	annotType = "org.opencontainers.artifact.type"
)

// ReferrerList returns a list of referrers to a given reference
// This is EXPERIMENTAL
func (reg *Reg) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	config := scheme.ReferrerConfig{}
	for _, opt := range opts {
		opt(&config)
	}
	rl := referrer.ReferrerList{
		Ref: r,
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

	// fall back to tag listing and converting into an index
	dig, err := digest.Parse(r.Digest)
	if err != nil {
		return rl, fmt.Errorf("failed to parse digest for referrers: %w", err)
	}
	// TODO: add support for filter on type
	re, err := regexp.Compile(fmt.Sprintf(`^%s-%s\.(?:([0-9a-f]*)\.|)(.*)$`, regexp.QuoteMeta(dig.Algorithm().String()), regexp.QuoteMeta(dig.Hex())))
	if err != nil {
		return rl, fmt.Errorf("failed to compile regexp for referrers: %w", err)
	}
	tl, err := reg.TagList(ctx, r)
	if err != nil {
		return rl, fmt.Errorf("failed to list tags for referrers: %w", err)
	}
	ociM := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		Manifests: []types.Descriptor{},
	}

	for _, t := range tl.Tags {
		if re.MatchString(t) {
			// for each matching entry, make a head request on the tag to build a descriptor for the generated index
			rt := r
			rt.Digest = ""
			rt.Tag = t
			var d types.Descriptor
			if config.ForceGet {
				mCur, err := reg.ManifestGet(ctx, rt)
				if err != nil {
					return rl, fmt.Errorf("failed to pull manifest: %s: %w", rt.CommonName(), err)
				}
				d = mCur.GetDescriptor()
				d.Annotations, err = mCur.GetAnnotations()
				if err != nil {
					return rl, fmt.Errorf("failed pulling up annotations: %s: %w", rt.CommonName(), err)
				}
			} else {
				mCur, err := reg.ManifestHead(ctx, rt)
				if err != nil {
					return rl, fmt.Errorf("failed to pull manifest headers: %s: %w", rt.CommonName(), err)
				}
				d = mCur.GetDescriptor()
			}
			ociM.Manifests = append(ociM.Manifests, d)
		}
	}
	mRet, err := manifest.New(manifest.WithOrig(ociM))
	if err != nil {
		return rl, fmt.Errorf("failed to build manifest of referrers: %w", err)
	}
	rl.Manifest = mRet
	rl.Descriptors = ociM.Manifests
	rl.Annotations = ociM.Annotations

	return rl, nil
}

func (reg *Reg) referrerListExtAPI(ctx context.Context, r ref.Ref) (referrer.ReferrerList, error) {
	rl := referrer.ReferrerList{
		Ref: r,
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
	if m.GetMediaType() != types.MediaTypeOCI1ManifestList {
		return rl, fmt.Errorf("unexpected media type for referrers: %s, %w", m.GetMediaType(), types.ErrUnsupportedMediaType)
	}
	rl.Manifest = m
	rl.Descriptors, err = m.GetManifestList()
	if err != nil {
		return rl, err
	}
	rl.Annotations, err = m.GetAnnotations()
	if err != nil {
		return rl, err
	}
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
	// TODO: support artifact media type
	mRawOrig, err := m.RawBody()
	if err != nil {
		return err
	}
	mDigOrig := m.GetDescriptor().Digest
	mOrig := m.GetOrig()
	ociM, err := manifest.OCIManifestFromAny(mOrig)
	if err != nil {
		return fmt.Errorf("failed to convert to manifest: %w", err)
	}
	// set annotations and refers field in manifest
	refType := ""
	if ociM.Annotations != nil && ociM.Annotations[annotType] != "" {
		refType = ociM.Annotations[annotType]
	}
	if ociM.Refers.MediaType != mRef.GetDescriptor().MediaType || ociM.Refers.Digest != mRef.GetDescriptor().Digest || ociM.Refers.Size != mRef.GetDescriptor().Size {
		reg.log.WithFields(logrus.Fields{
			"old MT":   ociM.Refers.MediaType,
			"new MT":   mRef.GetDescriptor().MediaType,
			"old Dig":  ociM.Refers.Digest.String(),
			"new Dig":  mRef.GetDescriptor().Digest.String(),
			"old Size": ociM.Refers.Size,
			"new Size": mRef.GetDescriptor().Size,
		}).Debug("refers field updated")
		ociM.Refers = mRef.GetDescriptor()
	}
	err = manifest.OCIManifestToAny(ociM, &mOrig)
	if err != nil {
		return err
	}
	err = m.SetOrig(mOrig)
	if err != nil {
		return err
	}
	mRawNew, err := m.RawBody()
	if err != nil {
		return err
	}
	mDigNew := m.GetDescriptor().Digest
	if mDigOrig != mDigNew {
		reg.log.WithFields(logrus.Fields{
			"orig": string(mRawOrig),
			"new":  string(mRawNew),
		}).Warn("digest changed")
	}

	// check if API available
	apiAvail := reg.referrerPing(ctx, r)

	rPush := r
	if apiAvail {
		// if available, push manifest by digest
		rPush.Tag = ""
		rPush.Digest = m.GetDescriptor().Digest.String()
	} else {
		// else set tag to push
		desc, err := digest.Parse(r.Digest)
		if err != nil {
			return fmt.Errorf("digest could not be parsed for %s: %w", r.CommonName(), err)
		}
		rPush.Digest = ""
		rPush.Tag = fmt.Sprintf("%s-%s.%s", desc.Algorithm().String(), stringMax(desc.Hex(), 64), stringMax(m.GetDigest().Hex(), 16))
		if refType != "" {
			rPush.Tag = fmt.Sprintf("%s.%s", rPush.Tag, stringMax(refType, 5))
		}
	}

	// call manifest put
	return reg.ManifestPut(ctx, rPush, m)
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
	if len(s) < max {
		return s
	}
	return s[:max]
}
