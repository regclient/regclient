package manifest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/containerd/containerd/platforms"
	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/wraperr"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
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
	GetRef() ref.Ref
	HasRateLimit() bool
	IsList() bool
	IsSet() bool
	MarshalJSON() ([]byte, error)
	RawBody() ([]byte, error)
	RawHeaders() (http.Header, error)
}

type ManifestConfig struct {
	r      ref.Ref
	desc   ociv1.Descriptor
	raw    []byte
	orig   interface{}
	header http.Header
}
type Opts func(*ManifestConfig)

// New creates a new manifest based on provided options
func New(opts ...Opts) (Manifest, error) {
	mc := ManifestConfig{}
	for _, opt := range opts {
		opt(&mc)
	}
	c := common{
		r:         mc.r,
		desc:      mc.desc,
		rawBody:   mc.raw,
		rawHeader: mc.header,
	}
	// extract fields from header where available
	if mc.header != nil {
		if c.desc.MediaType == "" {
			c.desc.MediaType = mc.header.Get("Content-Type")
		}
		if mc.desc.Size == 0 {
			cl, _ := strconv.Atoi(mc.header.Get("Content-Length"))
			mc.desc.Size = int64(cl)
		}
		if c.desc.Digest == "" {
			c.desc.Digest, _ = digest.Parse(mc.header.Get("Docker-Content-Digest"))
		}
		c.setRateLimit(mc.header)
	}
	if mc.orig != nil {
		return fromOrig(c, mc.orig)
	}
	return fromCommon(c)
}
func WithDesc(desc ociv1.Descriptor) Opts {
	return func(mc *ManifestConfig) {
		mc.desc = desc
	}
}
func WithHeader(header http.Header) Opts {
	return func(mc *ManifestConfig) {
		mc.header = header
	}
}
func WithOrig(orig interface{}) Opts {
	return func(mc *ManifestConfig) {
		mc.orig = orig
	}
}
func WithRaw(raw []byte) Opts {
	return func(mc *ManifestConfig) {
		mc.raw = raw
	}
}
func WithRef(r ref.Ref) Opts {
	return func(mc *ManifestConfig) {
		mc.r = r
	}
}

// FromOrig creates a new manifest from the original upstream manifest type.
// This method should be used if you are creating a new manifest rather than pulling one from a registry.
func fromOrig(c common, orig interface{}) (Manifest, error) {
	var mt string
	var m Manifest

	mj, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}
	c.manifSet = true
	if len(c.rawBody) == 0 {
		c.rawBody = mj
	}
	if _, ok := orig.(dockerSchema1.SignedManifest); !ok && c.desc.Digest == "" {
		c.desc.Digest = digest.FromBytes(mj)
	}
	if c.desc.Size == 0 {
		c.desc.Size = int64(len(mj))
	}
	// create manifest based on type
	switch orig.(type) {
	case dockerSchema1.Manifest:
		mOrig := orig.(dockerSchema1.Manifest)
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker1Manifest
		m = &docker1Manifest{
			common:   c,
			Manifest: mOrig,
		}
	case dockerSchema1.SignedManifest:
		mOrig := orig.(dockerSchema1.SignedManifest)
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker1ManifestSigned
		// recompute digest on the canonical data
		if c.desc.Digest == "" {
			c.desc.Digest = digest.FromBytes(mOrig.Canonical)
		}
		m = &docker1SignedManifest{
			common:         c,
			SignedManifest: mOrig,
		}
	case dockerSchema2.Manifest:
		mOrig := orig.(dockerSchema2.Manifest)
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker2Manifest
		m = &docker2Manifest{
			common:   c,
			Manifest: mOrig,
		}
	case dockerManifestList.ManifestList:
		mOrig := orig.(dockerManifestList.ManifestList)
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker2ManifestList
		m = &docker2ManifestList{
			common:       c,
			ManifestList: mOrig,
		}
	case ociv1.Manifest:
		mOrig := orig.(ociv1.Manifest)
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeOCI1Manifest
		m = &oci1Manifest{
			common:   c,
			Manifest: mOrig,
		}
	case ociv1.Index:
		mOrig := orig.(ociv1.Index)
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeOCI1ManifestList
		m = &oci1Index{
			common: c,
			Index:  orig.(ociv1.Index),
		}
	default:
		return nil, fmt.Errorf("Unsupported type to convert to a manifest: %T", orig)
	}
	// verify media type
	err = verifyMT(c.desc.MediaType, mt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func fromCommon(c common) (Manifest, error) {
	var err error
	var m Manifest
	var mt string
	// compute/verify digest
	if len(c.rawBody) > 0 {
		c.manifSet = true
		if c.desc.MediaType != MediaTypeDocker1ManifestSigned {
			d := digest.FromBytes(c.rawBody)
			if c.desc.Digest == "" {
				c.desc.Digest = d
			} else if c.desc.Digest != d {
				return nil, fmt.Errorf("digest mismatch, expected %s, found %s", c.desc.Digest.String(), d.String())
			}
		}
	}
	switch c.desc.MediaType {
	case MediaTypeDocker1Manifest:
		var mOrig dockerSchema1.Manifest
		if len(c.rawBody) > 0 {
			err = json.Unmarshal(c.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker1Manifest{common: c, Manifest: mOrig}
	case MediaTypeDocker1ManifestSigned:
		var mOrig dockerSchema1.SignedManifest
		if len(c.rawBody) > 0 {
			err = json.Unmarshal(c.rawBody, &mOrig)
			mt = mOrig.MediaType
			d := digest.FromBytes(mOrig.Canonical)
			if c.desc.Digest == "" {
				c.desc.Digest = d
			} else if c.desc.Digest != d {
				return nil, fmt.Errorf("digest mismatch, expected %s, found %s", c.desc.Digest.String(), d.String())
			}
		}
		m = &docker1SignedManifest{common: c, SignedManifest: mOrig}
	case MediaTypeDocker2Manifest:
		var mOrig dockerSchema2.Manifest
		if len(c.rawBody) > 0 {
			err = json.Unmarshal(c.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker2Manifest{common: c, Manifest: mOrig}
	case MediaTypeDocker2ManifestList:
		var mOrig dockerManifestList.ManifestList
		if len(c.rawBody) > 0 {
			err = json.Unmarshal(c.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &docker2ManifestList{common: c, ManifestList: mOrig}
	case MediaTypeOCI1Manifest:
		var mOrig ociv1.Manifest
		if len(c.rawBody) > 0 {
			err = json.Unmarshal(c.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &oci1Manifest{common: c, Manifest: mOrig}
	case MediaTypeOCI1ManifestList:
		var mOrig ociv1.Index
		if len(c.rawBody) > 0 {
			err = json.Unmarshal(c.rawBody, &mOrig)
			mt = mOrig.MediaType
		}
		m = &oci1Index{common: c, Index: mOrig}
	default:
		return nil, fmt.Errorf("%w: \"%s\"", types.ErrUnsupportedMediaType, c.desc.MediaType)
	}
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling manifest for %s: %w", c.r.CommonName(), err)
	}
	// verify media type
	err = verifyMT(c.desc.MediaType, mt)
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
	return nil, wraperr.New(fmt.Errorf("Platform not found: %s", platforms.Format(*p)), types.ErrNotFound)
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
