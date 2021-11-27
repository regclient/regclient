package manifest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/containerd/containerd/platforms"
	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/wraperr"
	"github.com/regclient/regclient/regclient/types"
)

// Manifest abstracts the various types of manifests that are supported
type Manifest interface {
	GetConfigDescriptor() (ociv1.Descriptor, error)
	GetConfigDigest() (digest.Digest, error)
	GetDigest() digest.Digest
	GetDescriptorList() ([]ociv1.Descriptor, error)
	GetLayers() ([]ociv1.Descriptor, error)
	GetMediaType() string
	GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error)
	GetPlatformList() ([]*ociv1.Platform, error)
	GetOrigManifest() interface{}
	GetRateLimit() types.RateLimit
	GetRef() types.Ref
	HasRateLimit() bool
	IsList() bool
	IsSet() bool
	MarshalJSON() ([]byte, error)
	RawBody() ([]byte, error)
	RawHeaders() (http.Header, error)
}

// New creates a new manifest from an unparsed raw manifest
// mediaType: should be a known media-type. If empty, resp headers will be checked
// raw: body of the manifest. If empty, unset manifest for a HEAD request is returned
// ref: reference, may be unset
// header: headers from request, used to extract content type, digest, and rate limits
func New(mediaType string, raw []byte, ref types.Ref, header http.Header) (Manifest, error) {
	mc := common{
		ref:     ref,
		mt:      mediaType,
		rawBody: raw,
	}
	if header != nil {
		mc.rawHeader = header
		if mc.mt == "" {
			mc.mt = header.Get("Content-Type")
		}
		mc.digest, _ = digest.Parse(header.Get("Docker-Content-Digest"))
		mc.setRateLimit(header)
	}
	if len(raw) > 0 {
		mc.manifSet = true
	}
	return fromCommon(mc)
}

// FromDescriptor creates a new manifest from a descriptor and the raw manifest bytes.
func FromDescriptor(desc ociv1.Descriptor, mBytes []byte) (Manifest, error) {
	mc := common{
		digest:   desc.Digest,
		mt:       desc.MediaType,
		manifSet: true,
		rawBody:  mBytes,
	}
	return fromCommon(mc)
}

// FromOrig creates a new manifest from the original upstream manifest type.
// This method should be used if you are creating a new manifest rather than pulling one from a registry.
func FromOrig(orig interface{}) (Manifest, error) {
	var mt string
	var m Manifest

	mj, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}
	mc := common{
		digest:   digest.FromBytes(mj),
		rawBody:  mj,
		manifSet: true,
	}
	// create manifest based on type
	switch orig.(type) {
	case dockerSchema1.Manifest:
		mOrig := orig.(dockerSchema1.Manifest)
		mt = mOrig.MediaType
		mc.mt = MediaTypeDocker1Manifest
		m = &docker1Manifest{
			common:   mc,
			Manifest: mOrig,
		}
	case dockerSchema1.SignedManifest:
		mOrig := orig.(dockerSchema1.SignedManifest)
		mt = mOrig.MediaType
		// recompute digest on the canonical data
		mc.digest = digest.FromBytes(mOrig.Canonical)
		mc.mt = MediaTypeDocker1ManifestSigned
		m = &docker1SignedManifest{
			common:         mc,
			SignedManifest: mOrig,
		}
	case dockerSchema2.Manifest:
		mOrig := orig.(dockerSchema2.Manifest)
		mt = mOrig.MediaType
		mc.mt = MediaTypeDocker2Manifest
		m = &docker2Manifest{
			common:   mc,
			Manifest: mOrig,
		}
	case dockerManifestList.ManifestList:
		mOrig := orig.(dockerManifestList.ManifestList)
		mt = mOrig.MediaType
		mc.mt = MediaTypeDocker2ManifestList
		m = &docker2ManifestList{
			common:       mc,
			ManifestList: mOrig,
		}
	case ociv1.Manifest:
		mOrig := orig.(ociv1.Manifest)
		mt = mOrig.MediaType
		mc.mt = MediaTypeOCI1Manifest
		m = &oci1Manifest{
			common:   mc,
			Manifest: mOrig,
		}
	case ociv1.Index:
		mOrig := orig.(ociv1.Index)
		mt = mOrig.MediaType
		mc.mt = MediaTypeOCI1ManifestList
		m = &oci1Index{
			common: mc,
			Index:  orig.(ociv1.Index),
		}
	case UnknownData:
		m = &unknown{
			common:      mc,
			UnknownData: orig.(UnknownData),
		}
	default:
		return nil, fmt.Errorf("Unsupported type to convert to a manifest: %T", orig)
	}
	// verify media type
	err = verifyMT(mc.mt, mt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func fromCommon(mc common) (Manifest, error) {
	var err error
	var m Manifest
	var mt string
	// compute/verify digest
	if len(mc.rawBody) > 0 {
		var d digest.Digest
		if mc.mt == MediaTypeDocker1ManifestSigned {
			d = digest.FromBytes(m.(*docker1SignedManifest).Canonical)
		} else {
			d = digest.FromBytes(mc.rawBody)
		}
		if mc.digest == "" {
			mc.digest = d
		} else if mc.digest != d {
			return nil, fmt.Errorf("digest mismatch, expected %s, found %s", mc.digest.String(), d.String())
		}
	}
	switch mc.mt {
	case MediaTypeDocker1Manifest:
		var mOrig dockerSchema1.Manifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker1Manifest{common: mc, Manifest: mOrig}
	case MediaTypeDocker1ManifestSigned:
		var mOrig dockerSchema1.SignedManifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker1SignedManifest{common: mc, SignedManifest: mOrig}
	case MediaTypeDocker2Manifest:
		var mOrig dockerSchema2.Manifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker2Manifest{common: mc, Manifest: mOrig}
	case MediaTypeDocker2ManifestList:
		var mOrig dockerManifestList.ManifestList
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker2ManifestList{common: mc, ManifestList: mOrig}
	case MediaTypeOCI1Manifest:
		var mOrig ociv1.Manifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &oci1Manifest{common: mc, Manifest: mOrig}
	case MediaTypeOCI1ManifestList:
		var mOrig ociv1.Index
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &oci1Index{common: mc, Index: mOrig}
	default:
		var mOrig UnknownData
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
		}
		m = &unknown{common: mc, UnknownData: mOrig}
	}
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling manifest for %s: %w", mc.ref.CommonName(), err)
	}
	// verify media type
	err = verifyMT(mc.mt, mt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func verifyMT(expected, received string) error {
	if received != "" && expected != received {
		return fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", expected, received)
	}
	return nil
}

func getPlatformDesc(p *ociv1.Platform, dl []ociv1.Descriptor) (*ociv1.Descriptor, error) {
	platformCmp := platforms.NewMatcher(*p)
	for _, d := range dl {
		if d.Platform != nil && platformCmp.Match(*d.Platform) {
			return &d, nil
		}
	}
	return nil, wraperr.New(fmt.Errorf("Platform not found: %s", platforms.Format(*p)), ErrNotFound)
}

func getPlatformList(dl []ociv1.Descriptor) ([]*ociv1.Platform, error) {
	var l []*ociv1.Platform
	for _, d := range dl {
		if d.Platform != nil {
			l = append(l, d.Platform)
		}
	}
	return l, nil
}

func d2oDescriptor(sd dockerDistribution.Descriptor) *ociv1.Descriptor {
	return &ociv1.Descriptor{
		MediaType:   sd.MediaType,
		Digest:      sd.Digest,
		Size:        sd.Size,
		URLs:        sd.URLs,
		Annotations: sd.Annotations,
		Platform:    sd.Platform,
	}
}

func dl2oDescriptor(sd dockerManifestList.ManifestDescriptor) *ociv1.Descriptor {
	return &ociv1.Descriptor{
		MediaType:   sd.MediaType,
		Digest:      sd.Digest,
		Size:        sd.Size,
		URLs:        sd.URLs,
		Annotations: sd.Annotations,
		Platform:    dlp2Platform(sd.Platform),
	}
}

func dlp2Platform(sp dockerManifestList.PlatformSpec) *ociv1.Platform {
	return &ociv1.Platform{
		Architecture: sp.Architecture,
		OS:           sp.OS,
		Variant:      sp.Variant,
		OSVersion:    sp.OSVersion,
		OSFeatures:   sp.OSFeatures,
	}
}
