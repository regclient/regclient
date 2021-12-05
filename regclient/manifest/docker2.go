package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/containerd/containerd/platforms"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/wraperr"
)

const (
	// MediaTypeDocker2Manifest is the media type when pulling manifests from a v2 registry
	MediaTypeDocker2Manifest = dockerSchema2.MediaTypeManifest
	// MediaTypeDocker2ManifestList is the media type when pulling a manifest list from a v2 registry
	MediaTypeDocker2ManifestList = dockerManifestList.MediaTypeManifestList
)

type docker2Manifest struct {
	common
	dockerSchema2.Manifest
}
type docker2ManifestList struct {
	common
	dockerManifestList.ManifestList
}

func (m *docker2Manifest) GetConfigDescriptor() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{
		MediaType:   m.Config.MediaType,
		Digest:      m.Config.Digest,
		Size:        m.Config.Size,
		URLs:        m.Config.URLs,
		Annotations: m.Config.Annotations,
		Platform:    m.Config.Platform,
	}, nil
}
func (m *docker2Manifest) GetConfigDigest() (digest.Digest, error) {
	return m.Config.Digest, nil
}
func (m *docker2ManifestList) GetConfigDescriptor() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker2ManifestList) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *docker2Manifest) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker2ManifestList) GetDescriptorList() ([]ociv1.Descriptor, error) {
	dl := []ociv1.Descriptor{}
	for _, d := range m.Manifests {
		dl = append(dl, *dl2oDescriptor(d))
	}
	return dl, nil
}

func (m *docker2Manifest) GetLayers() ([]ociv1.Descriptor, error) {
	var dl []ociv1.Descriptor
	for _, sd := range m.Layers {
		dl = append(dl, *d2oDescriptor(sd))
	}
	return dl, nil
}
func (m *docker2ManifestList) GetLayers() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Layers are not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *docker2Manifest) GetOrigManifest() interface{} {
	return m.Manifest
}
func (m *docker2ManifestList) GetOrigManifest() interface{} {
	return m.ManifestList
}

func (m *docker2Manifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker2ManifestList) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformDesc(p, dl)
}

func (m *docker2Manifest) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *docker2ManifestList) GetPlatformList() ([]*ociv1.Platform, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformList(dl)
}

func (m *docker2Manifest) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.rawBody) > 0 {
		return m.rawBody, nil
	}

	return json.Marshal((m.Manifest))
}
func (m *docker2ManifestList) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.rawBody) > 0 {
		return m.rawBody, nil
	}

	return json.Marshal((m.ManifestList))
}

func (m *docker2Manifest) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.Manifest)
	return buf.Bytes(), nil
}
func (m *docker2ManifestList) MarshalPretty() ([]byte, error) {
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
