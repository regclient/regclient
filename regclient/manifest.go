package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/containerd/containerd/platforms"
	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
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

type manifestCommon struct {
	ref       Ref
	digest    digest.Digest
	mt        string
	manifSet  bool
	orig      interface{}
	ratelimit RateLimit
	rawHeader http.Header
	rawBody   []byte
}
type manifestDocker1M struct {
	manifestCommon
	dockerSchema1.Manifest
}
type manifestDocker1MS struct {
	manifestCommon
	dockerSchema1.SignedManifest
}
type manifestDockerM struct {
	manifestCommon
	dockerSchema2.Manifest
}
type manifestDockerML struct {
	manifestCommon
	dockerManifestList.ManifestList
}
type manifestOCIM struct {
	manifestCommon
	ociv1.Manifest
}
type manifestOCIML struct {
	manifestCommon
	ociv1.Index
}

// Manifest abstracts the various types of manifests that are supported
type Manifest interface {
	GetConfigDigest() (digest.Digest, error)
	GetDigest() digest.Digest
	GetDescriptorList() ([]ociv1.Descriptor, error)
	GetLayers() ([]ociv1.Descriptor, error)
	GetMediaType() string
	GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error)
	GetPlatformList() ([]*ociv1.Platform, error)
	GetOrigManifest() interface{}
	GetRateLimit() RateLimit
	GetRef() Ref
	HasRateLimit() bool
	IsList() bool
	IsSet() bool
	MarshalJSON() ([]byte, error)
	RawBody() ([]byte, error)
	RawHeaders() (http.Header, error)
}

func (m *manifestCommon) GetDigest() digest.Digest {
	return m.digest
}

func (m *manifestCommon) GetMediaType() string {
	return m.mt
}

func (m *manifestCommon) GetOrigManifest() interface{} {
	return m.orig
}

func (m *manifestCommon) GetRateLimit() RateLimit {
	return m.ratelimit
}

func (m *manifestCommon) GetRef() Ref {
	return m.ref
}

func (m *manifestCommon) HasRateLimit() bool {
	return m.ratelimit.Set
}

func (m *manifestCommon) IsList() bool {
	switch m.mt {
	case MediaTypeDocker2ManifestList, MediaTypeOCI1ManifestList:
		return true
	default:
		return false
	}
}

func (m *manifestCommon) IsSet() bool {
	return m.manifSet
}

func (m *manifestCommon) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.rawBody) > 0 {
		return m.rawBody, nil
	}

	if m.orig != nil {
		return json.Marshal((m.orig))
	}
	return []byte{}, wraperr.New(fmt.Errorf("Json marshalling not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *manifestCommon) RawBody() ([]byte, error) {
	return m.rawBody, nil
}

func (m *manifestCommon) RawHeaders() (http.Header, error) {
	return m.rawHeader, nil
}

func (m *manifestDocker1M) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDocker1MS) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerM) GetConfigDigest() (digest.Digest, error) {
	return m.Config.Digest, nil
}
func (m *manifestOCIM) GetConfigDigest() (digest.Digest, error) {
	return m.Config.Digest, nil
}
func (m *manifestDockerML) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestOCIML) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *manifestDocker1M) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDocker1MS) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerM) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestOCIM) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerML) GetDescriptorList() ([]ociv1.Descriptor, error) {
	dl := []ociv1.Descriptor{}
	for _, d := range m.Manifests {
		dl = append(dl, *dl2oDescriptor(d))
	}
	return dl, nil
}
func (m *manifestOCIML) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return m.Manifests, nil
}

func (m *manifestDocker1M) GetLayers() ([]ociv1.Descriptor, error) {
	var dl []ociv1.Descriptor
	for _, sd := range m.FSLayers {
		dl = append(dl, ociv1.Descriptor{
			Digest: sd.BlobSum,
		})
	}
	return dl, nil
}
func (m *manifestDocker1MS) GetLayers() ([]ociv1.Descriptor, error) {
	var dl []ociv1.Descriptor
	for _, sd := range m.FSLayers {
		dl = append(dl, ociv1.Descriptor{
			Digest: sd.BlobSum,
		})
	}
	return dl, nil
}
func (m *manifestDockerM) GetLayers() ([]ociv1.Descriptor, error) {
	var dl []ociv1.Descriptor
	for _, sd := range m.Layers {
		dl = append(dl, *d2oDescriptor(sd))
	}
	return dl, nil
}
func (m *manifestOCIM) GetLayers() ([]ociv1.Descriptor, error) {
	return m.Layers, nil
}
func (m *manifestDockerML) GetLayers() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Layers are not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestOCIML) GetLayers() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Layers are not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

// GetPlatformDesc returns the descriptor for the platform from the manifest list or OCI index
func (m *manifestDocker1M) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDocker1MS) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerM) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestOCIM) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerML) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformDesc(p, dl)
}
func (m *manifestOCIML) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformDesc(p, dl)
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

// GetPlatformList returns the list of platforms in a manifest list
func (m *manifestDocker1M) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDocker1MS) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerM) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestOCIM) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *manifestDockerML) GetPlatformList() ([]*ociv1.Platform, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformList(dl)
}
func (m *manifestOCIML) GetPlatformList() ([]*ociv1.Platform, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformList(dl)
}
func getPlatformList(dl []ociv1.Descriptor) ([]*ociv1.Platform, error) {
	var l []*ociv1.Platform
	for _, d := range dl {
		l = append(l, d.Platform)
	}
	return l, nil
}

func (m *manifestDocker1MS) MarshalJSON() ([]byte, error) {
	return m.SignedManifest.MarshalJSON()
}

// MarshalPretty is used for printPretty template formatting
func (m *manifestDocker1M) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.orig)
	return buf.Bytes(), nil
}
func (m *manifestDocker1MS) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.orig)
	return buf.Bytes(), nil
}
func (m *manifestDockerM) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.orig)
	return buf.Bytes(), nil
}
func (m *manifestOCIM) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.orig)
	return buf.Bytes(), nil
}
func (m *manifestDockerML) MarshalPretty() ([]byte, error) {
	if m == nil {
		return []byte{}, nil
	}
	buf := &bytes.Buffer{}
	tw := tabwriter.NewWriter(buf, 0, 0, 1, ' ', 0)
	if m.ref.Reference != "" {
		fmt.Fprintf(tw, "Name:\t%s\n", m.ref.Reference)
	}
	fmt.Fprintf(tw, "MediaType:\t%s\n", m.mt)
	fmt.Fprintf(tw, "Digest:\t%s\n", m.digest.String())
	fmt.Fprintf(tw, "\t\n")
	fmt.Fprintf(tw, "Manifests:\t\n")
	for _, d := range m.Manifests {
		fmt.Fprintf(tw, "\t\n")
		dRef := m.ref
		if dRef.Reference != "" {
			dRef.Digest = d.Digest.String()
			fmt.Fprintf(tw, "  Name:\t%s\n", dRef.CommonName())
		} else {
			fmt.Fprintf(tw, "  Digest:\t%s\n", string(d.Digest))
		}
		fmt.Fprintf(tw, "  MediaType:\t%s\n", d.MediaType)
		if p := d.Platform; p.OS != "" {
			fmt.Fprintf(tw, "  Platform:\t%s\n", platforms.Format(*dlp2Platform(p)))
			if p.OSVersion != "" {
				fmt.Fprintf(tw, "  OSVersion:\t%s\n", p.OSVersion)
			}
			if len(p.OSFeatures) > 0 {
				fmt.Fprintf(tw, "  OSFeatures:\t%s\n", strings.Join(p.OSFeatures, ", "))
			}
		}
		if len(d.URLs) > 0 {
			fmt.Fprintf(tw, "  URLs:\t%s\n", strings.Join(d.URLs, ", "))
		}
		if d.Annotations != nil {
			fmt.Fprintf(tw, "  Annotations:\t\n")
			for k, v := range d.Annotations {
				fmt.Fprintf(tw, "    %s:\t%s\n", k, v)
			}
		}
	}
	tw.Flush()
	return buf.Bytes(), nil
}
func (m *manifestOCIML) MarshalPretty() ([]byte, error) {
	if m == nil {
		return []byte{}, nil
	}
	buf := &bytes.Buffer{}
	tw := tabwriter.NewWriter(buf, 0, 0, 1, ' ', 0)
	if m.ref.Reference != "" {
		fmt.Fprintf(tw, "Name:\t%s\n", m.ref.Reference)
	}
	fmt.Fprintf(tw, "MediaType:\t%s\n", m.mt)
	fmt.Fprintf(tw, "Digest:\t%s\n", m.digest.String())
	fmt.Fprintf(tw, "\t\n")
	fmt.Fprintf(tw, "Manifests:\t\n")
	for _, d := range m.Manifests {
		fmt.Fprintf(tw, "\t\n")
		dRef := m.ref
		if dRef.Reference != "" {
			dRef.Digest = d.Digest.String()
			fmt.Fprintf(tw, "  Name:\t%s\n", dRef.CommonName())
		} else {
			fmt.Fprintf(tw, "  Digest:\t%s\n", string(d.Digest))
		}
		fmt.Fprintf(tw, "  MediaType:\t%s\n", d.MediaType)
		if d.Platform != nil {
			if p := d.Platform; p.OS != "" {
				fmt.Fprintf(tw, "  Platform:\t%s\n", platforms.Format(*p))
				if p.OSVersion != "" {
					fmt.Fprintf(tw, "  OSVersion:\t%s\n", p.OSVersion)
				}
				if len(p.OSFeatures) > 0 {
					fmt.Fprintf(tw, "  OSFeatures:\t%s\n", strings.Join(p.OSFeatures, ", "))
				}
			}
		}
		if len(d.URLs) > 0 {
			fmt.Fprintf(tw, "  URLs:\t%s\n", strings.Join(d.URLs, ", "))
		}
		if d.Annotations != nil {
			fmt.Fprintf(tw, "  Annotations:\t\n")
			for k, v := range d.Annotations {
				fmt.Fprintf(tw, "    %s:\t%s\n", k, v)
			}
		}
	}
	tw.Flush()
	return buf.Bytes(), nil
}

func (rc *regClient) ManifestDelete(ctx context.Context, ref Ref) error {
	if ref.Digest == "" {
		return wraperr.New(fmt.Errorf("Digest required to delete manifest, reference %s", ref.CommonName()), ErrMissingDigest)
	}

	// build/send request
	headers := http.Header{
		"Accept": []string{
			MediaTypeDocker1Manifest,
			MediaTypeDocker1ManifestSigned,
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
	var m Manifest
	mc := manifestCommon{
		ref:      ref,
		manifSet: true,
	}

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
			MediaTypeDocker1Manifest,
			MediaTypeDocker1ManifestSigned,
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

	mc.rawHeader = resp.HTTPResponse().Header
	rc.ratelimitHeader(&mc, resp.HTTPResponse())

	// read manifest and compute digest
	digester := digest.Canonical.Digester()
	reader := io.TeeReader(resp, digester.Hash())
	mc.rawBody, err = ioutil.ReadAll(reader)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.Reference,
		}).Warn("Failed to read manifest")
		return nil, fmt.Errorf("Error reading manifest for %s: %w", ref.CommonName(), err)
	}
	mc.digest = digester.Digest()

	// parse body into variable according to media type
	mc.mt = resp.HTTPResponse().Header.Get("Content-Type")

	headDigest := resp.HTTPResponse().Header.Get("OCI-Content-Digest")
	if headDigest == "" {
		headDigest = resp.HTTPResponse().Header.Get("Docker-Content-Digest")
	}
	if headDigest != "" && headDigest != mc.digest.String() && mc.mt != MediaTypeDocker1Manifest && mc.mt != MediaTypeDocker1ManifestSigned {
		rc.log.WithFields(logrus.Fields{
			"computed": mc.digest.String(),
			"returned": headDigest,
		}).Warn("Computed digest does not match header from registry")
	}

	switch mc.mt {
	case MediaTypeDocker1Manifest:
		dm := dockerSchema1.Manifest{}
		err = json.Unmarshal(mc.rawBody, &dm)
		mc.orig = dm
		m = &manifestDocker1M{manifestCommon: mc, Manifest: dm}
	case MediaTypeDocker1ManifestSigned:
		dm := dockerSchema1.SignedManifest{}
		err = json.Unmarshal(mc.rawBody, &dm)
		mc.orig = dm
		m = &manifestDocker1MS{manifestCommon: mc, SignedManifest: dm}
	case MediaTypeDocker2Manifest:
		dm := dockerSchema2.Manifest{}
		err = json.Unmarshal(mc.rawBody, &dm)
		mc.orig = dm
		m = &manifestDockerM{manifestCommon: mc, Manifest: dm}
	case MediaTypeDocker2ManifestList:
		dml := dockerManifestList.ManifestList{}
		err = json.Unmarshal(mc.rawBody, &dml)
		mc.orig = dml
		m = &manifestDockerML{manifestCommon: mc, ManifestList: dml}
	case MediaTypeOCI1Manifest:
		om := ociv1.Manifest{}
		err = json.Unmarshal(mc.rawBody, &om)
		mc.orig = om
		m = &manifestOCIM{manifestCommon: mc, Manifest: om}
	case MediaTypeOCI1ManifestList:
		oi := ociv1.Index{}
		err = json.Unmarshal(mc.rawBody, &oi)
		mc.orig = oi
		m = &manifestOCIML{manifestCommon: mc, Index: oi}
	default:
		rc.log.WithFields(logrus.Fields{
			"mediatype": mc.mt,
			"ref":       ref.Reference,
		}).Warn("Unsupported media type for manifest")
		return nil, wraperr.New(fmt.Errorf("Unsupported media type: %s, reference: %s", mc.mt, ref.CommonName()), ErrUnsupportedMediaType)
	}
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":       err,
			"mediatype": mc.mt,
			"ref":       ref.Reference,
		}).Warn("Failed to unmarshal manifest")
		return nil, fmt.Errorf("Error unmarshalling manifest for %s: %w", ref.CommonName(), err)
	}

	return m, nil
}

func (rc *regClient) ManifestHead(ctx context.Context, ref Ref) (Manifest, error) {
	var m Manifest
	mc := manifestCommon{
		ref:      ref,
		manifSet: false,
	}

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
			MediaTypeDocker1Manifest,
			MediaTypeDocker1ManifestSigned,
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
				method:  "HEAD",
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

	rc.ratelimitHeader(&mc, resp.HTTPResponse())

	// extract header data
	mc.rawHeader = resp.HTTPResponse().Header
	mc.mt = resp.HTTPResponse().Header.Get("Content-Type")
	mc.digest, err = digest.Parse(resp.HTTPResponse().Header.Get("Docker-Content-Digest"))
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
		}).Warn("No header found for Docker-Content-Digest")
	}

	switch mc.mt {
	case MediaTypeDocker1Manifest:
		m = &manifestDocker1M{manifestCommon: mc}
	case MediaTypeDocker1ManifestSigned:
		m = &manifestDocker1MS{manifestCommon: mc}
	case MediaTypeDocker2Manifest:
		m = &manifestDockerM{manifestCommon: mc}
	case MediaTypeDocker2ManifestList:
		m = &manifestDockerML{manifestCommon: mc}
	case MediaTypeOCI1Manifest:
		m = &manifestOCIM{manifestCommon: mc}
	case MediaTypeOCI1ManifestList:
		m = &manifestOCIML{manifestCommon: mc}
	default:
		rc.log.WithFields(logrus.Fields{
			"mediatype": mc.mt,
			"ref":       ref.Reference,
		}).Warn("Unsupported media type for manifest")
		return nil, wraperr.New(fmt.Errorf("Unsupported media type: %s, reference: %s", mc.mt, ref.CommonName()), ErrUnsupportedMediaType)
	}

	return m, nil
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

func (rc *regClient) ratelimitHeader(m *manifestCommon, r *http.Response) {
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
