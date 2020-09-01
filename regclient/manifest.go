package regclient

import (
	"encoding/json"
	"fmt"

	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type manifest struct {
	dockerM  dockerSchema2.Manifest
	dockerML dockerManifestList.ManifestList
	mt       string
	ociM     ociv1.Manifest
	ociML    ociv1.Index
}

// Manifest abstracts the various types of manifests that are supported
type Manifest interface {
	GetConfigDigest() (digest.Digest, error)
	GetDockerManifest() dockerSchema2.Manifest
	GetDockerManifestList() dockerManifestList.ManifestList
	GetLayers() ([]ociv1.Descriptor, error)
	GetMediaType() string
	GetOCIManifest() ociv1.Manifest
	GetOCIManifestList() ociv1.Index
	MarshalJSON() ([]byte, error)
}

func (m *manifest) GetConfigDigest() (digest.Digest, error) {
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return m.dockerM.Config.Digest, nil
	case ociv1.MediaTypeImageManifest:
		return m.ociM.Config.Digest, nil
	}
	// TODO: find config for current OS type?
	return "", fmt.Errorf("Unsupported manifest mediatype %s", m.mt)
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
	return []ociv1.Descriptor{}, fmt.Errorf("Unsupported manifest mediatype %s", m.mt)
}

func d2oDescriptorList(src []dockerDistribution.Descriptor) []ociv1.Descriptor {
	var tgt []ociv1.Descriptor
	for _, sd := range src {
		td := ociv1.Descriptor{
			MediaType:   sd.MediaType,
			Digest:      sd.Digest,
			Size:        sd.Size,
			URLs:        sd.URLs,
			Annotations: sd.Annotations,
			Platform:    sd.Platform,
		}
		tgt = append(tgt, td)
	}
	return tgt
}

func (m *manifest) GetMediaType() string {
	return m.mt
}

func (m *manifest) GetOCIManifest() ociv1.Manifest {
	return m.ociM
}

func (m *manifest) GetOCIManifestList() ociv1.Index {
	return m.ociML
}

func (m *manifest) MarshalJSON() ([]byte, error) {
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
	return []byte{}, ErrUnsupportedMediaType
}
