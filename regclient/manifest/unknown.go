package manifest

import (
	"fmt"

	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/wraperr"
)

type unknown struct {
	common
	UnknownData
}

type UnknownData struct {
	Data map[string]interface{}
}

func (m *unknown) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *unknown) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *unknown) GetLayers() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Layer list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *unknown) GetOrigManifest() interface{} {
	return m.UnknownData
}

func (m *unknown) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *unknown) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *unknown) MarshalJSON() ([]byte, error) {
	return m.rawBody, nil
}
