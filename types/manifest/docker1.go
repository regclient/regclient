package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"

	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/wraperr"
	"github.com/regclient/regclient/types"
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

func (m *docker1Manifest) GetConfig() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, wraperr.New(fmt.Errorf("config digest not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}
func (m *docker1Manifest) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("config digest not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetConfig() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, wraperr.New(fmt.Errorf("config digest not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("config digest not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}

func (m *docker1Manifest) GetManifestList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("platform descriptor list not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetManifestList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("platform descriptor list not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
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

func (m *docker1Manifest) GetOrig() interface{} {
	return m.Manifest
}
func (m *docker1SignedManifest) GetOrig() interface{} {
	return m.SignedManifest
}

func (m *docker1Manifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("platform lookup not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("platform lookup not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}

func (m *docker1Manifest) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("platform list not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}
func (m *docker1SignedManifest) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("platform list not available for media type %s", m.desc.MediaType), types.ErrUnsupportedMediaType)
}

func (m *docker1Manifest) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), types.ErrUnavailable)
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

func (m *docker1Manifest) SetOrig(origIn interface{}) error {
	orig, ok := origIn.(dockerSchema1.Manifest)
	if !ok {
		return types.ErrUnsupportedMediaType
	}
	if orig.MediaType != types.MediaTypeDocker1Manifest {
		// TODO: error?
		orig.MediaType = types.MediaTypeDocker1Manifest
	}
	mj, err := json.Marshal(orig)
	if err != nil {
		return err
	}
	m.manifSet = true
	m.rawBody = mj
	m.desc = ociv1.Descriptor{
		MediaType: types.MediaTypeDocker1Manifest,
		Digest:    digest.FromBytes(mj),
		Size:      int64(len(mj)),
	}
	m.Manifest = orig

	return nil
}

func (m *docker1SignedManifest) SetOrig(origIn interface{}) error {
	orig, ok := origIn.(dockerSchema1.SignedManifest)
	if !ok {
		return types.ErrUnsupportedMediaType
	}
	if orig.MediaType != types.MediaTypeDocker1ManifestSigned {
		// TODO: error?
		orig.MediaType = types.MediaTypeDocker1ManifestSigned
	}
	mj, err := json.Marshal(orig)
	if err != nil {
		return err
	}
	m.manifSet = true
	m.rawBody = mj
	m.desc = ociv1.Descriptor{
		MediaType: types.MediaTypeDocker1ManifestSigned,
		Digest:    digest.FromBytes(mj),
		Size:      int64(len(mj)),
	}
	m.SignedManifest = orig

	return nil
}
