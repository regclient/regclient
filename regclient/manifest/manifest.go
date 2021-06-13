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
		if mc.digest == "" {
			mc.digest = digest.FromBytes(raw)
		}
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
	var d digest.Digest
	mj, err := json.Marshal(orig)
	if err == nil {
		d = digest.FromBytes(mj)
	}

	mc := common{
		digest:   d,
		manifSet: true,
	}

	switch orig.(type) {
	case dockerSchema1.Manifest:
		mc.mt = MediaTypeDocker1Manifest
		return &docker1Manifest{
			common:   mc,
			Manifest: orig.(dockerSchema1.Manifest),
		}, nil
	case dockerSchema1.SignedManifest:
		mc.digest = digest.FromBytes(orig.(dockerSchema1.SignedManifest).Canonical)
		mc.mt = MediaTypeDocker1ManifestSigned
		return &docker1SignedManifest{
			common:         mc,
			SignedManifest: orig.(dockerSchema1.SignedManifest),
		}, nil
	case dockerSchema2.Manifest:
		mc.mt = MediaTypeDocker2Manifest
		return &docker2Manifest{
			common:   mc,
			Manifest: orig.(dockerSchema2.Manifest),
		}, nil
	case dockerManifestList.ManifestList:
		mc.mt = MediaTypeDocker2ManifestList
		return &docker2ManifestList{
			common:       mc,
			ManifestList: orig.(dockerManifestList.ManifestList),
		}, nil
	case ociv1.Manifest:
		mc.mt = MediaTypeOCI1Manifest
		return &oci1Manifest{
			common:   mc,
			Manifest: orig.(ociv1.Manifest),
		}, nil
	case ociv1.Index:
		mc.mt = MediaTypeOCI1ManifestList
		return &oci1Index{
			common: mc,
			Index:  orig.(ociv1.Index),
		}, nil
	case UnknownData:
		return &unknown{
			common:      mc,
			UnknownData: orig.(UnknownData),
		}, nil
	default:
		return nil, fmt.Errorf("Unsupported type to convert to a manifest: %T", orig)
	}

}

func fromCommon(mc common) (Manifest, error) {
	var err error
	var m Manifest
	switch mc.mt {
	case MediaTypeDocker1Manifest:
		var mOrig dockerSchema1.Manifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
		}
		m = &docker1Manifest{common: mc, Manifest: mOrig}
	case MediaTypeDocker1ManifestSigned:
		var mOrig dockerSchema1.SignedManifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
			mc.digest = digest.FromBytes(mOrig.Canonical)
		}
		m = &docker1SignedManifest{common: mc, SignedManifest: mOrig}
	case MediaTypeDocker2Manifest:
		var mOrig dockerSchema2.Manifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
		}
		m = &docker2Manifest{common: mc, Manifest: mOrig}
	case MediaTypeDocker2ManifestList:
		var mOrig dockerManifestList.ManifestList
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
		}
		m = &docker2ManifestList{common: mc, ManifestList: mOrig}
	case MediaTypeOCI1Manifest:
		var mOrig ociv1.Manifest
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
		}
		m = &oci1Manifest{common: mc, Manifest: mOrig}
	case MediaTypeOCI1ManifestList:
		var mOrig ociv1.Index
		if len(mc.rawBody) > 0 {
			err = json.Unmarshal(mc.rawBody, &mOrig)
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
		return nil, fmt.Errorf("Error unmarshaling manifest for %s: %w", mc.ref.CommonName(), err)
	}
	return m, nil
}

func getPlatformDesc(p *ociv1.Platform, dl []ociv1.Descriptor) (*ociv1.Descriptor, error) {
	platformCmp := platforms.NewMatcher(*p)
	for _, d := range dl {
		if platformCmp.Match(*d.Platform) {
			return &d, nil
		}
	}
	return nil, wraperr.New(fmt.Errorf("Platform not found: %v", p), ErrNotFound)
}

func getPlatformList(dl []ociv1.Descriptor) ([]*ociv1.Platform, error) {
	var l []*ociv1.Platform
	for _, d := range dl {
		l = append(l, d.Platform)
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
