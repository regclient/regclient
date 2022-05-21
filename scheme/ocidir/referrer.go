package ocidir

import (
	"context"
	"fmt"
	"regexp"

	"github.com/opencontainers/go-digest"
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
func (o *OCIDir) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	rl := referrer.ReferrerList{
		Ref: r,
	}
	// if ref is a tag, run a head request for the digest
	if r.Digest == "" {
		m, err := o.ManifestHead(ctx, r)
		if err != nil {
			return rl, err
		}
		r.Digest = m.GetDescriptor().Digest.String()
	}

	// use tag listing and convert into an index
	dig, err := digest.Parse(r.Digest)
	if err != nil {
		return rl, fmt.Errorf("failed to parse digest for referrers: %w", err)
	}
	// TODO: add support for filter on type
	re, err := regexp.Compile(fmt.Sprintf(`^%s-%s\.(?:([0-9a-f]*)\.|)(.*)$`, regexp.QuoteMeta(dig.Algorithm().String()), regexp.QuoteMeta(dig.Hex())))
	if err != nil {
		return rl, fmt.Errorf("failed to compile regexp for referrers: %w", err)
	}
	tl, err := o.TagList(ctx, r)
	if err != nil {
		return rl, fmt.Errorf("failed to list tags for referrers: %w", err)
	}
	ociM := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		Manifests: []types.Descriptor{},
	}

	for _, t := range tl.Tags {
		if re.MatchString(t) {
			// for each matching entry, add a descriptor in the generated index
			rt := r
			rt.Digest = ""
			rt.Tag = t
			mCur, err := o.ManifestGet(ctx, rt)
			if err != nil {
				return rl, fmt.Errorf("failed to lookup tag in referrers: %s: %w", rt.CommonName(), err)
			}
			mCurAnnot, ok := mCur.(manifest.Annotator)
			if !ok {
				return rl, fmt.Errorf("manifest does not support annotations: %w", types.ErrUnsupportedMediaType)
			}
			d := mCur.GetDescriptor()
			// pull up annotations
			d.Annotations, err = mCurAnnot.GetAnnotations()
			if err != nil {
				return rl, fmt.Errorf("failed to pull up annotations: %w", err)
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
		o.log.WithFields(logrus.Fields{
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
		o.log.WithFields(logrus.Fields{
			"orig": string(mRawOrig),
			"new":  string(mRawNew),
		}).Warn("digest changed")
	}

	// set tag to push
	desc, err := digest.Parse(r.Digest)
	if err != nil {
		return fmt.Errorf("digest could not be parsed for %s: %w", r.CommonName(), err)
	}
	rPush := r
	rPush.Digest = ""
	rPush.Tag = fmt.Sprintf("%s-%s.%s", desc.Algorithm().String(), stringMax(desc.Hex(), 64), stringMax(m.GetDigest().Hex(), 16))
	if refType != "" {
		rPush.Tag = fmt.Sprintf("%s.%s", rPush.Tag, stringMax(refType, 5))
	}

	// call manifest put
	return o.ManifestPut(ctx, rPush, m)
}

func stringMax(s string, max int) string {
	if len(s) < max {
		return s
	}
	return s[:max]
}
