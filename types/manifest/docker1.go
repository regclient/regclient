package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"

	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/wraperr"
)

const (
	// MediaTypeDocker1Manifest deprecated media type for docker schema1 manifests
	MediaTypeDocker1Manifest = "application/vnd.docker.distribution.manifest.v1+json"
	// MediaTypeDocker1ManifestSigned is a deprecated schema1 manifest with jws signing
	MediaTypeDocker1ManifestSigned = "application/vnd.docker.distribution.manifest.v1+prettyjws"
)

type docker1Manifest struct {
	common
	dockerSchema1.Manifest
}
type docker1SignedManifest struct {
	common
	dockerSchema1.SignedManifest
}

func (m *docker1Manifest) GetConfigDescriptor() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker1Manifest) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetConfigDescriptor() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *docker1Manifest) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *docker1Manifest) GetLayers() ([]ociv1.Descriptor, error) {
	var dl []ociv1.Descriptor
	for _, sd := range m.FSLayers {
		dl = append(dl, ociv1.Descriptor{
			Digest: sd.BlobSum,
		})
	}
	return dl, nil
}
func (m *docker1SignedManifest) GetLayers() ([]ociv1.Descriptor, error) {
	var dl []ociv1.Descriptor
	for _, sd := range m.FSLayers {
		dl = append(dl, ociv1.Descriptor{
			Digest: sd.BlobSum,
		})
	}
	return dl, nil
}

func (m *docker1Manifest) GetOrigManifest() interface{} {
	return m.Manifest
}
func (m *docker1SignedManifest) GetOrigManifest() interface{} {
	return m.SignedManifest
}

func (m *docker1Manifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *docker1Manifest) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *docker1Manifest) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.rawBody) > 0 {
		return m.rawBody, nil
	}

	return json.Marshal((m.Manifest))
}

func (m *docker1SignedManifest) MarshalJSON() ([]byte, error) {
	return m.SignedManifest.MarshalJSON()
}

func (m *docker1Manifest) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.Manifest)
	return buf.Bytes(), nil
}
func (m *docker1SignedManifest) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.SignedManifest)
	return buf.Bytes(), nil
}
