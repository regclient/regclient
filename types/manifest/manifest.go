// Package manifest abstracts the various types of supported manifests.
// Supported types include OCI index and image, and Docker manifest list and manifest.
package manifest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/containerd/containerd/platforms"
	dockerDistribution "github.com/docker/distribution"
	"github.com/docker/distribution/manifest"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/wraperr"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

// Manifest interface is implemented by all supported manifests but
// many calls are only supported by certain underlying media types.
type Manifest interface {
	GetConfig() (ociv1.Descriptor, error)
	GetDescriptor() ociv1.Descriptor
	GetLayers() ([]ociv1.Descriptor, error)
	GetManifestList() ([]ociv1.Descriptor, error)
	GetOrig() interface{}
	GetRef() ref.Ref
	IsList() bool
	IsSet() bool
	MarshalJSON() ([]byte, error)
	RawBody() ([]byte, error)
	RawHeaders() (http.Header, error)
	SetOrig(interface{}) error

	GetConfigDigest() (digest.Digest, error)                      // TODO: deprecate
	GetDigest() digest.Digest                                     // TODO: deprecate
	GetMediaType() string                                         // TODO: deprecate
	GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) // TODO: deprecate
	GetPlatformList() ([]*ociv1.Platform, error)                  // TODO: deprecate
	GetRateLimit() types.RateLimit                                // TODO: deprecate
	HasRateLimit() bool                                           // TODO: deprecate
}

type manifestConfig struct {
	r      ref.Ref
	desc   ociv1.Descriptor
	raw    []byte
	orig   interface{}
	header http.Header
}
type Opts func(*manifestConfig)

// New creates a new manifest based on provided options
func New(opts ...Opts) (Manifest, error) {
	mc := manifestConfig{}
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

// WithDesc specifies the descriptor for the manifest
func WithDesc(desc ociv1.Descriptor) Opts {
	return func(mc *manifestConfig) {
		mc.desc = desc
	}
}

// WithHeader provides the headers from the response when pulling the manifest
func WithHeader(header http.Header) Opts {
	return func(mc *manifestConfig) {
		mc.header = header
	}
}

// WithOrig provides the original manifest variable
func WithOrig(orig interface{}) Opts {
	return func(mc *manifestConfig) {
		mc.orig = orig
	}
}

// WithRaw provides the manifest bytes or HTTP response body
func WithRaw(raw []byte) Opts {
	return func(mc *manifestConfig) {
		mc.raw = raw
	}
}

// WithRef provides the reference used to get the manifest
func WithRef(r ref.Ref) Opts {
	return func(mc *manifestConfig) {
		mc.r = r
	}
}

// GetDigest returns the digest from the manifest descriptor
func GetDigest(m Manifest) digest.Digest {
	d := m.GetDescriptor()
	return d.Digest
}

// GetMediaType returns the media type from the manifest descriptor
func GetMediaType(m Manifest) string {
	d := m.GetDescriptor()
	return d.MediaType
}

// GetPlatformDesc returns the descriptor for a specific platform from an index
func GetPlatformDesc(m Manifest, p *ociv1.Platform) (*ociv1.Descriptor, error) {
	dl, err := m.GetManifestList()
	if err != nil {
		return nil, err
	}
	platformCmp := platforms.NewMatcher(*p)
	for _, d := range dl {
		if d.Platform != nil && platformCmp.Match(*d.Platform) {
			return &d, nil
		}
	}
	return nil, wraperr.New(fmt.Errorf("platform not found: %s", platforms.Format(*p)), types.ErrNotFound)
}

// GetPlatformList returns the list of platforms from an index
func GetPlatformList(m Manifest) ([]*ociv1.Platform, error) {
	dl, err := m.GetManifestList()
	if err != nil {
		return nil, err
	}
	var l []*ociv1.Platform
	for _, d := range dl {
		if d.Platform != nil {
			l = append(l, d.Platform)
		}
	}
	return l, nil
}

// GetRateLimit returns the current rate limit seen in headers
func GetRateLimit(m Manifest) types.RateLimit {
	rl := types.RateLimit{}
	header, err := m.RawHeaders()
	if err != nil {
		return rl
	}
	// check for rate limit headers
	rlLimit := header.Get("RateLimit-Limit")
	rlRemain := header.Get("RateLimit-Remaining")
	rlReset := header.Get("RateLimit-Reset")
	if rlLimit != "" {
		lpSplit := strings.Split(rlLimit, ",")
		lSplit := strings.Split(lpSplit[0], ";")
		rlLimitI, err := strconv.Atoi(lSplit[0])
		if err != nil {
			rl.Limit = 0
		} else {
			rl.Limit = rlLimitI
		}
		if len(lSplit) > 1 {
			rl.Policies = lpSplit
		} else if len(lpSplit) > 1 {
			rl.Policies = lpSplit[1:]
		}
	}
	if rlRemain != "" {
		rSplit := strings.Split(rlRemain, ";")
		rlRemainI, err := strconv.Atoi(rSplit[0])
		if err != nil {
			rl.Remain = 0
		} else {
			rl.Remain = rlRemainI
			rl.Set = true
		}
	}
	if rlReset != "" {
		rlResetI, err := strconv.Atoi(rlReset)
		if err != nil {
			rl.Reset = 0
		} else {
			rl.Reset = rlResetI
		}
	}
	return rl
}

// HasRateLimit indicates whether the rate limit is set and available
func HasRateLimit(m Manifest) bool {
	rl := GetRateLimit(m)
	return rl.Set
}

func OCIIndexFromAny(orig interface{}) (ociv1.Index, error) {
	ociI := ociv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: types.MediaTypeOCI1ManifestList,
	}
	switch orig := orig.(type) {
	case dockerManifestList.ManifestList:
		ml := make([]ociv1.Descriptor, len(orig.Manifests))
		for i, d := range orig.Manifests {
			ml[i] = *dl2oDescriptor(d)
		}
		ociI.Manifests = ml
	case ociv1.Index:
		ociI = orig
	default:
		return ociI, fmt.Errorf("unable to convert %T to OCI index", orig)
	}
	return ociI, nil
}

func OCIIndexToAny(ociI ociv1.Index, origP interface{}) error {
	// reflect is used to handle both *interface{} and *Manifest
	rv := reflect.ValueOf(origP)
	for rv.IsValid() && rv.Type().Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return fmt.Errorf("invalid manifest output parameter: %T", origP)
	}
	if !rv.CanSet() {
		return fmt.Errorf("manifest output must be a pointer: %T", origP)
	}
	origR := rv.Interface()
	switch orig := (origR).(type) {
	case dockerManifestList.ManifestList:
		ml := make([]dockerManifestList.ManifestDescriptor, len(ociI.Manifests))
		for i, d := range ociI.Manifests {
			ml[i] = dockerManifestList.ManifestDescriptor{
				Descriptor: dockerDistribution.Descriptor{
					MediaType:   d.MediaType,
					Size:        d.Size,
					Digest:      d.Digest,
					URLs:        d.URLs,
					Annotations: d.Annotations,
					Platform:    d.Platform,
				},
			}
			if d.Platform != nil {
				ml[i].Platform = dockerManifestList.PlatformSpec{
					Architecture: d.Platform.Architecture,
					OS:           d.Platform.OS,
					OSVersion:    d.Platform.OSVersion,
					OSFeatures:   d.Platform.OSFeatures,
					Variant:      d.Platform.Variant,
				}
			}
		}
		orig.Manifests = ml
		orig.Versioned = manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     types.MediaTypeDocker2ManifestList,
		}
		rv.Set(reflect.ValueOf(orig))
	case ociv1.Index:
		rv.Set(reflect.ValueOf(ociI))
	default:
		return fmt.Errorf("unable to convert OCI index to %T", origR)
	}
	return nil
}

func OCIManifestFromAny(orig interface{}) (ociv1.Manifest, error) {
	ociM := ociv1.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: types.MediaTypeOCI1Manifest,
	}
	switch orig := orig.(type) {
	case dockerSchema2.Manifest:
		ll := make([]ociv1.Descriptor, len(orig.Layers))
		for i, l := range orig.Layers {
			ll[i] = *d2oDescriptor(l)
		}
		ociM.Config = *d2oDescriptor(orig.Config)
		ociM.Layers = ll
	case ociv1.Manifest:
		ociM = orig
	default:
		// TODO: consider supporting Docker schema v1 media types
		return ociM, fmt.Errorf("unable to convert %T to OCI image", orig)
	}
	return ociM, nil
}

func OCIManifestToAny(ociM ociv1.Manifest, origP interface{}) error {
	// reflect is used to handle both *interface{} and *Manifest
	rv := reflect.ValueOf(origP)
	for rv.IsValid() && rv.Type().Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return fmt.Errorf("invalid manifest output parameter: %T", origP)
	}
	if !rv.CanSet() {
		return fmt.Errorf("manifest output must be a pointer: %T", origP)
	}
	origR := rv.Interface()
	switch orig := (origR).(type) {
	case dockerSchema2.Manifest:
		ll := make([]dockerDistribution.Descriptor, len(ociM.Layers))
		for i, l := range ociM.Layers {
			ll[i] = dockerDistribution.Descriptor{
				MediaType:   l.MediaType,
				Size:        l.Size,
				Digest:      l.Digest,
				URLs:        l.URLs,
				Annotations: l.Annotations,
				Platform:    l.Platform,
			}
		}
		orig.Layers = ll
		orig.Config = dockerDistribution.Descriptor{
			MediaType:   ociM.Config.MediaType,
			Size:        ociM.Config.Size,
			Digest:      ociM.Config.Digest,
			URLs:        ociM.Config.URLs,
			Annotations: ociM.Config.Annotations,
			Platform:    ociM.Config.Platform,
		}
		orig.Versioned.MediaType = types.MediaTypeDocker2Manifest
		orig.Versioned.SchemaVersion = 2
		rv.Set(reflect.ValueOf(orig))
	case ociv1.Manifest:
		rv.Set(reflect.ValueOf(ociM))
	default:
		// Docker schema v1 will not be supported, can't resign, and no need for unsigned
		return fmt.Errorf("unable to convert OCI image to %T", origR)
	}
	return nil
}

// FromOrig creates a new manifest from the original upstream manifest type.
// This method should be used if you are creating a new manifest rather than pulling one from a registry.
func fromOrig(c common, orig interface{}) (Manifest, error) {
	var mt string
	var m Manifest
	origDigest := c.desc.Digest

	mj, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}
	c.manifSet = true
	if len(c.rawBody) == 0 {
		c.rawBody = mj
	}
	if _, ok := orig.(dockerSchema1.SignedManifest); !ok {
		c.desc.Digest = digest.FromBytes(mj)
	}
	if c.desc.Size == 0 {
		c.desc.Size = int64(len(mj))
	}
	// create manifest based on type
	switch mOrig := orig.(type) {
	case dockerSchema1.Manifest:
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker1Manifest
		m = &docker1Manifest{
			common:   c,
			Manifest: mOrig,
		}
	case dockerSchema1.SignedManifest:
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker1ManifestSigned
		// recompute digest on the canonical data
		c.desc.Digest = digest.FromBytes(mOrig.Canonical)
		m = &docker1SignedManifest{
			common:         c,
			SignedManifest: mOrig,
		}
	case dockerSchema2.Manifest:
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker2Manifest
		m = &docker2Manifest{
			common:   c,
			Manifest: mOrig,
		}
	case dockerManifestList.ManifestList:
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeDocker2ManifestList
		m = &docker2ManifestList{
			common:       c,
			ManifestList: mOrig,
		}
	case ociv1.Manifest:
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeOCI1Manifest
		m = &oci1Manifest{
			common:   c,
			Manifest: mOrig,
		}
	case ociv1.Index:
		mt = mOrig.MediaType
		c.desc.MediaType = types.MediaTypeOCI1ManifestList
		m = &oci1Index{
			common: c,
			Index:  orig.(ociv1.Index),
		}
	default:
		return nil, fmt.Errorf("unsupported type to convert to a manifest: %T", orig)
	}
	// verify media type
	err = verifyMT(c.desc.MediaType, mt)
	if err != nil {
		return nil, err
	}
	// verify digest didn't change
	if origDigest != "" && origDigest != c.desc.Digest {
		return nil, fmt.Errorf("manifest digest mismatch, expected %s, computed %s", origDigest, c.desc.Digest)
	}
	return m, nil
}

func fromCommon(c common) (Manifest, error) {
	var err error
	var m Manifest
	var mt string
	origDigest := c.desc.Digest
	// compute/verify digest
	if len(c.rawBody) > 0 {
		c.manifSet = true
		if c.desc.MediaType != MediaTypeDocker1ManifestSigned {
			d := digest.FromBytes(c.rawBody)
			c.desc.Digest = d
			c.desc.Size = int64(len(c.rawBody))
		}
	}
	// extract media type from body if needed
	if c.desc.MediaType == "" && len(c.rawBody) > 0 {
		mt := struct {
			MediaType     string        `json:"mediaType,omitempty"`
			SchemaVersion int           `json:"schemaVersion,omitempty"`
			Signatures    []interface{} `json:"signatures,omitempty"`
		}{}
		err = json.Unmarshal(c.rawBody, &mt)
		if mt.MediaType != "" {
			c.desc.MediaType = mt.MediaType
		} else if mt.SchemaVersion == 1 && len(mt.Signatures) > 0 {
			c.desc.MediaType = types.MediaTypeDocker1ManifestSigned
		} else if mt.SchemaVersion == 1 {
			c.desc.MediaType = types.MediaTypeDocker1Manifest
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
			c.desc.Digest = d
			c.desc.Size = int64(len(mOrig.Canonical))
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
	// verify digest didn't change
	if origDigest != "" && origDigest != c.desc.Digest {
		return nil, fmt.Errorf("manifest digest mismatch, expected %s, computed %s", origDigest, c.desc.Digest)
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
	return nil, wraperr.New(fmt.Errorf("platform not found: %s", platforms.Format(*p)), types.ErrNotFound)
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
