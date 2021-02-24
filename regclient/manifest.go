package regclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/containerd/containerd/platforms"
	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/regclient/regclient/pkg/wraperr"
	"github.com/sirupsen/logrus"
)

// ManifestClient provides registry client requests to manifests
type ManifestClient interface {
	ManifestDelete(ctx context.Context, ref Ref) error
	ManifestGet(ctx context.Context, ref Ref) (Manifest, error)
	ManifestHead(ctx context.Context, ref Ref) (Manifest, error)
	ManifestPut(ctx context.Context, ref Ref, m Manifest) error
}

type manifest struct {
	digest    digest.Digest
	dockerM   dockerSchema2.Manifest
	dockerML  dockerManifestList.ManifestList
	manifSet  bool
	mt        string
	ociM      ociv1.Manifest
	ociML     ociv1.Index
	origByte  []byte
	ratelimit RateLimit
}

// Manifest abstracts the various types of manifests that are supported
type Manifest interface {
	GetConfigDigest() (digest.Digest, error)
	GetDigest() digest.Digest
	GetDockerManifest() dockerSchema2.Manifest
	GetDockerManifestList() dockerManifestList.ManifestList
	GetLayers() ([]ociv1.Descriptor, error)
	GetMediaType() string
	GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error)
	GetPlatformList() ([]*ociv1.Platform, error)
	GetOCIManifest() ociv1.Manifest
	GetOCIManifestList() ociv1.Index
	GetOrigManifest() interface{}
	GetRateLimit() RateLimit
	HasRateLimit() bool
	IsList() bool
	MarshalJSON() ([]byte, error)
}

func (m *manifest) GetConfigDigest() (digest.Digest, error) {
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return m.dockerM.Config.Digest, nil
	case ociv1.MediaTypeImageManifest:
		return m.ociM.Config.Digest, nil
	}
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *manifest) GetDigest() digest.Digest {
	return m.digest
}

func (m *manifest) GetDockerManifest() dockerSchema2.Manifest {
	return m.dockerM
}

func (m *manifest) GetDockerManifestList() dockerManifestList.ManifestList {
	return m.dockerML
}

func (m *manifest) GetLayers() ([]ociv1.Descriptor, error) {
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return d2oDescriptorList(m.dockerM.Layers), nil
	case ociv1.MediaTypeImageManifest:
		return m.ociM.Layers, nil
	}
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Layers are not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *manifest) GetMediaType() string {
	return m.mt
}

// GetPlatformDesc returns the descriptor for the platform from the manifest list or OCI index
func (m *manifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	platformCmp := platforms.NewMatcher(*p)
	switch m.mt {
	case MediaTypeDocker2ManifestList:
		for _, d := range m.dockerML.Manifests {
			if platformCmp.Match(*dlp2Platform(d.Platform)) {
				return dl2oDescriptor(d), nil
			}
		}
	case MediaTypeOCI1ManifestList:
		for _, d := range m.ociML.Manifests {
			if platformCmp.Match(*d.Platform) {
				return &d, nil
			}
		}
	default:
		return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
	}
	return nil, wraperr.New(fmt.Errorf("Platform not found: %v", p), ErrNotFound)
}

// GetPlatformList returns the list of platforms in a manifest list
func (m *manifest) GetPlatformList() ([]*ociv1.Platform, error) {
	var l []*ociv1.Platform
	if !m.manifSet {
		return l, wraperr.New(fmt.Errorf("Platform list unavailable, perform a ManifestGet first"), ErrUnavailable)
	}
	switch m.mt {
	case MediaTypeDocker2ManifestList:
		for _, d := range m.dockerML.Manifests {
			l = append(l, dlp2Platform(d.Platform))
		}
	case MediaTypeOCI1ManifestList:
		for _, d := range m.ociML.Manifests {
			l = append(l, d.Platform)
		}
	default:
		return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
	}
	return l, nil
}

func (m *manifest) GetOCIManifest() ociv1.Manifest {
	return m.ociM
}

func (m *manifest) GetOCIManifestList() ociv1.Index {
	return m.ociML
}

func (m *manifest) GetOrigManifest() interface{} {
	if !m.manifSet {
		return nil
	}
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return m.dockerM
	case MediaTypeDocker2ManifestList:
		return m.dockerML
	case MediaTypeOCI1Manifest:
		return m.ociM
	case MediaTypeOCI1ManifestList:
		return m.ociML
	default:
		return nil
	}
}

func (m *manifest) GetRateLimit() RateLimit {
	return m.ratelimit
}

func (m *manifest) HasRateLimit() bool {
	return m.ratelimit.Set
}

func (m *manifest) IsList() bool {
	switch m.mt {
	case MediaTypeDocker2ManifestList, MediaTypeOCI1ManifestList:
		return true
	default:
		return false
	}
}

func (m *manifest) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.origByte) > 0 {
		return m.origByte, nil
	}

	switch m.mt {
	case MediaTypeDocker2Manifest:
		return json.Marshal(m.dockerM)
	case MediaTypeDocker2ManifestList:
		return json.Marshal(m.dockerML)
	case MediaTypeOCI1Manifest:
		return json.Marshal(m.ociM)
	case MediaTypeOCI1ManifestList:
		return json.Marshal(m.ociML)
	}
	return []byte{}, wraperr.New(fmt.Errorf("Json marshalling not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (rc *regClient) ManifestDelete(ctx context.Context, ref Ref) error {
	if ref.Digest == "" {
		return wraperr.New(fmt.Errorf("Digest required to delete manifest, reference %s", ref.CommonName()), ErrMissingDigest)
	}

	// build/send request
	headers := http.Header{
		"Accept": []string{
			MediaTypeDocker2Manifest,
			MediaTypeDocker2ManifestList,
			MediaTypeOCI1Manifest,
			MediaTypeOCI1ManifestList,
		},
	}
	req := httpReq{
		host:      ref.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method:  "DELETE",
				path:    ref.Repository + "/manifests/" + ref.Digest,
				headers: headers,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to delete manifest %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("Failed to delete manifest %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}

func (rc *regClient) ManifestGet(ctx context.Context, ref Ref) (Manifest, error) {
	m := manifest{}

	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		return nil, wraperr.New(fmt.Errorf("Reference missing tag and digest: %s", ref.CommonName()), ErrMissingTagOrDigest)
	}

	// build/send request
	headers := http.Header{
		"Accept": []string{
			MediaTypeDocker2Manifest,
			MediaTypeDocker2ManifestList,
			MediaTypeOCI1Manifest,
			MediaTypeOCI1ManifestList,
		},
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:  "GET",
				path:    ref.Repository + "/manifests/" + tagOrDigest,
				headers: headers,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return nil, fmt.Errorf("Failed to get manifest %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to get manifest %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	rc.ratelimitHeader(&m, resp.HTTPResponse())

	// read manifest and compute digest
	digester := digest.Canonical.Digester()
	reader := io.TeeReader(resp, digester.Hash())
	m.origByte, err = ioutil.ReadAll(reader)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.Reference,
		}).Warn("Failed to read manifest")
		return nil, fmt.Errorf("Error reading manifest for %s: %w", ref.CommonName(), err)
	}
	m.digest = digester.Digest()

	headDigest := resp.HTTPResponse().Header.Get("OCI-Content-Digest")
	if headDigest == "" {
		headDigest = resp.HTTPResponse().Header.Get("Docker-Content-Digest")
	}
	if headDigest != "" && headDigest != m.digest.String() {
		rc.log.WithFields(logrus.Fields{
			"computed": m.digest.String(),
			"returned": headDigest,
		}).Warn("Computed digest does not match header from registry")
	}

	// parse body into variable according to media type
	m.mt = resp.HTTPResponse().Header.Get("Content-Type")
	switch m.mt {
	case MediaTypeDocker2Manifest:
		err = json.Unmarshal(m.origByte, &m.dockerM)
	case MediaTypeDocker2ManifestList:
		err = json.Unmarshal(m.origByte, &m.dockerML)
	case MediaTypeOCI1Manifest:
		err = json.Unmarshal(m.origByte, &m.ociM)
	case MediaTypeOCI1ManifestList:
		err = json.Unmarshal(m.origByte, &m.ociML)
	default:
		rc.log.WithFields(logrus.Fields{
			"mediatype": m.mt,
			"ref":       ref.Reference,
		}).Warn("Unsupported media type for manifest")
		return nil, wraperr.New(fmt.Errorf("Unsupported media type: %s, reference: %s", m.mt, ref.CommonName()), ErrUnsupportedMediaType)
	}
	// TODO: consider making a manifest Unmarshal method that detects which mediatype from the json
	// err = json.Unmarshal(m.origByte, &m)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":       err,
			"mediatype": m.mt,
			"ref":       ref.Reference,
		}).Warn("Failed to unmarshal manifest")
		return nil, fmt.Errorf("Error unmarshalling manifest for %s: %w", ref.CommonName(), err)
	}
	m.manifSet = true

	return &m, nil
}

func (rc *regClient) ManifestHead(ctx context.Context, ref Ref) (Manifest, error) {
	m := manifest{}

	// build the request
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
		}).Warn("Manifest requires a tag or digest")
		return nil, wraperr.New(fmt.Errorf("Reference missing tag and digest: %s", ref.CommonName()), ErrMissingTagOrDigest)
	}

	// build/send request
	headers := http.Header{
		"Accept": []string{
			MediaTypeDocker2Manifest,
			MediaTypeDocker2ManifestList,
			MediaTypeOCI1Manifest,
			MediaTypeOCI1ManifestList,
		},
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:  "GET",
				path:    ref.Repository + "/manifests/" + tagOrDigest,
				headers: headers,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return nil, fmt.Errorf("Failed to request manifest head %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to request manifest head %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	rc.ratelimitHeader(&m, resp.HTTPResponse())

	// extract media type and digest from header
	m.mt = resp.HTTPResponse().Header.Get("Content-Type")
	m.digest, err = digest.Parse(resp.HTTPResponse().Header.Get("Docker-Content-Digest"))
	if err != nil {
		return nil, fmt.Errorf("Error getting digest for %s: %w", ref.CommonName(), err)
	}

	return &m, nil
}

func (rc *regClient) ManifestPut(ctx context.Context, ref Ref, m Manifest) error {
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
		}).Warn("Manifest put requires a tag")
		return ErrMissingTag
	}

	// create the request body
	mj, err := m.MarshalJSON()
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
			"err": err,
		}).Warn("Error marshaling manifest")
		return fmt.Errorf("Error marshalling manifest for %s: %w", ref.CommonName(), err)
	}

	// build/send request
	headers := http.Header{
		"Content-Type": []string{m.GetMediaType()},
	}
	req := httpReq{
		host:      ref.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method:    "PUT",
				path:      ref.Repository + "/manifests/" + tagOrDigest,
				headers:   headers,
				bodyLen:   int64(len(mj)),
				bodyBytes: mj,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to put manifest %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Failed to put manifest %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}

func (rc *regClient) ratelimitHeader(m *manifest, r *http.Response) {
	// check for rate limit headers
	rlLimit := r.Header.Get("RateLimit-Limit")
	rlRemain := r.Header.Get("RateLimit-Remaining")
	rlReset := r.Header.Get("RateLimit-Reset")
	if rlLimit != "" {
		lpSplit := strings.Split(rlLimit, ",")
		lSplit := strings.Split(lpSplit[0], ";")
		rlLimitI, err := strconv.Atoi(lSplit[0])
		if err != nil {
			m.ratelimit.Limit = 0
		} else {
			m.ratelimit.Limit = rlLimitI
		}
		if len(lSplit) > 1 {
			m.ratelimit.Policies = lpSplit
		} else if len(lpSplit) > 1 {
			m.ratelimit.Policies = lpSplit[1:]
		}
	}
	if rlRemain != "" {
		m.ratelimit.Set = true
		rSplit := strings.Split(rlRemain, ";")
		rlRemainI, err := strconv.Atoi(rSplit[0])
		if err != nil {
			m.ratelimit.Remain = 0
		} else {
			m.ratelimit.Remain = rlRemainI
			m.ratelimit.Set = true
		}
	}
	if rlReset != "" {
		rlResetI, err := strconv.Atoi(rlReset)
		if err != nil {
			m.ratelimit.Reset = 0
		} else {
			m.ratelimit.Reset = rlResetI
		}
	}
	if m.ratelimit.Set {
		rc.log.WithFields(logrus.Fields{
			"limit":  m.ratelimit.Limit,
			"remain": m.ratelimit.Remain,
			"reset":  m.ratelimit.Reset,
		}).Debug("Rate limit found")
	}
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

func d2oDescriptorList(src []dockerDistribution.Descriptor) []ociv1.Descriptor {
	var tgt []ociv1.Descriptor
	for _, sd := range src {
		tgt = append(tgt, *d2oDescriptor(sd))
	}
	return tgt
}
