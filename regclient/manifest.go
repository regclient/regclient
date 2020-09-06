package regclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"

	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/sudo-bmitch/regcli/pkg/retryable"
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

func (rc *regClient) ManifestDigest(ctx context.Context, ref Ref) (digest.Digest, error) {
	host := rc.getHost(ref.Registry)
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
		}).Warn("Manifest requires a tag or digest")
		return "", ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	opts := []retryable.OptsReq{}
	opts = append(opts, retryable.WithHeader("Accept", []string{
		MediaTypeDocker2Manifest,
		MediaTypeDocker2ManifestList,
		MediaTypeOCI1Manifest,
		MediaTypeOCI1ManifestList,
	}))

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", manfURL, opts...)
	if err != nil {
		return "", err
	}
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.Reference,
		}).Warn("Failed to read manifest")
		return "", err
	}
	return digest.FromBytes(respBody), nil
}

func (rc *regClient) ManifestGet(ctx context.Context, ref Ref) (Manifest, error) {
	m := manifest{}

	host := rc.getHost(ref.Registry)
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
		}).Warn("Manifest requires a tag or digest")
		return nil, ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	opts := []retryable.OptsReq{}
	opts = append(opts, retryable.WithHeader("Accept", []string{
		MediaTypeDocker2Manifest,
		MediaTypeDocker2ManifestList,
		MediaTypeOCI1Manifest,
		MediaTypeOCI1ManifestList,
	}))

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", manfURL, opts...)
	if err != nil {
		return nil, err
	}
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.Reference,
		}).Warn("Failed to read manifest")
		return nil, err
	}
	m.mt = resp.HTTPResponse().Header.Get("Content-Type")
	switch m.mt {
	case MediaTypeDocker2Manifest:
		err = json.Unmarshal(respBody, &m.dockerM)
	case MediaTypeDocker2ManifestList:
		err = json.Unmarshal(respBody, &m.dockerML)
	case MediaTypeOCI1Manifest:
		err = json.Unmarshal(respBody, &m.ociM)
	case MediaTypeOCI1ManifestList:
		err = json.Unmarshal(respBody, &m.ociML)
	default:
		rc.log.WithFields(logrus.Fields{
			"mediatype": m.mt,
			"ref":       ref.Reference,
		}).Warn("Unsupported media type for manifest")
		return nil, fmt.Errorf("Unknown manifest media type %s", m.mt)
	}
	// TODO: consider making a manifest Unmarshal method that detects which mediatype from the json
	// err = json.Unmarshal(respBody, &m)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":       err,
			"mediatype": m.mt,
			"ref":       ref.Reference,
		}).Warn("Failed to unmarshal manifest")
		return nil, err
	}

	return &m, nil
}

func (rc *regClient) ManifestPut(ctx context.Context, ref Ref, m Manifest) error {
	host := rc.getHost(ref.Registry)
	if ref.Tag == "" {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
		}).Warn("Manifest put requires a tag")
		return ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + ref.Tag,
	}

	// add body to request
	opts := []retryable.OptsReq{}
	opts = append(opts, retryable.WithHeader("Content-Type", []string{m.GetMediaType()}))

	var mj []byte
	mj, err := json.Marshal(m)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
			"err": err,
		}).Warn("Error marshaling manifest")
		return err
	}
	opts = append(opts, retryable.WithBodyBytes(mj))
	opts = append(opts, retryable.WithContentLen(int64(len(mj))))

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "PUT", manfURL, opts...)
	if err != nil {
		return fmt.Errorf("Error calling manifest put request: %w\nResponse object: %v", err, resp)
	}

	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		body, _ := ioutil.ReadAll(resp)
		rc.log.WithFields(logrus.Fields{
			"ref":    ref.Reference,
			"status": resp.HTTPResponse().StatusCode,
			"body":   body,
		}).Warn("Unexpected status code for manifest")
		return fmt.Errorf("Unexpected status code on manifest put %d\nResponse object: %v\nBody: %s", resp.HTTPResponse().StatusCode, resp, body)
	}

	return nil
}
