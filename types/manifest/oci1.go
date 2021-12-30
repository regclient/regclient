package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/containerd/containerd/platforms"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/wraperr"
)

const (
	// MediaTypeOCI1Manifest OCI v1 manifest media type
	MediaTypeOCI1Manifest = ociv1.MediaTypeImageManifest
	// MediaTypeOCI1ManifestList OCI v1 manifest list media type
	MediaTypeOCI1ManifestList = ociv1.MediaTypeImageIndex
)

type oci1Manifest struct {
	common
	ociv1.Manifest
}
type oci1Index struct {
	common
	ociv1.Index
}

func (m *oci1Manifest) GetConfigDescriptor() (ociv1.Descriptor, error) {
	return m.Config, nil
}
func (m *oci1Manifest) GetConfigDigest() (digest.Digest, error) {
	return m.Config.Digest, nil
}
func (m *oci1Index) GetConfigDescriptor() (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *oci1Index) GetConfigDigest() (digest.Digest, error) {
	return "", wraperr.New(fmt.Errorf("Config digest not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *oci1Manifest) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Platform descriptor list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *oci1Index) GetDescriptorList() ([]ociv1.Descriptor, error) {
	return m.Manifests, nil
}

func (m *oci1Manifest) GetLayers() ([]ociv1.Descriptor, error) {
	return m.Layers, nil
}
func (m *oci1Index) GetLayers() ([]ociv1.Descriptor, error) {
	return []ociv1.Descriptor{}, wraperr.New(fmt.Errorf("Layers are not available for media type %s", m.mt), ErrUnsupportedMediaType)
}

func (m *oci1Manifest) GetOrigManifest() interface{} {
	return m.Manifest
}
func (m *oci1Index) GetOrigManifest() interface{} {
	return m.Index
}

func (m *oci1Manifest) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	return nil, wraperr.New(fmt.Errorf("Platform lookup not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *oci1Index) GetPlatformDesc(p *ociv1.Platform) (*ociv1.Descriptor, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformDesc(p, dl)
}

func (m *oci1Manifest) GetPlatformList() ([]*ociv1.Platform, error) {
	return nil, wraperr.New(fmt.Errorf("Platform list not available for media type %s", m.mt), ErrUnsupportedMediaType)
}
func (m *oci1Index) GetPlatformList() ([]*ociv1.Platform, error) {
	dl, err := m.GetDescriptorList()
	if err != nil {
		return nil, err
	}
	return getPlatformList(dl)
}

func (m *oci1Manifest) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.rawBody) > 0 {
		return m.rawBody, nil
	}

	return json.Marshal((m.Manifest))
}
func (m *oci1Index) MarshalJSON() ([]byte, error) {
	if !m.manifSet {
		return []byte{}, wraperr.New(fmt.Errorf("Manifest unavailable, perform a ManifestGet first"), ErrUnavailable)
	}

	if len(m.rawBody) > 0 {
		return m.rawBody, nil
	}

	return json.Marshal((m.Index))
}

func (m *oci1Manifest) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(m.Manifest)
	return buf.Bytes(), nil
}
func (m *oci1Index) MarshalPretty() ([]byte, error) {
	if m == nil {
		return []byte{}, nil
	}
	buf := &bytes.Buffer{}
	tw := tabwriter.NewWriter(buf, 0, 0, 1, ' ', 0)
	if m.r.Reference != "" {
		fmt.Fprintf(tw, "Name:\t%s\n", m.r.Reference)
	}
	fmt.Fprintf(tw, "MediaType:\t%s\n", m.mt)
	fmt.Fprintf(tw, "Digest:\t%s\n", m.digest.String())
	fmt.Fprintf(tw, "\t\n")
	fmt.Fprintf(tw, "Manifests:\t\n")
	for _, d := range m.Manifests {
		fmt.Fprintf(tw, "\t\n")
		dRef := m.r
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
