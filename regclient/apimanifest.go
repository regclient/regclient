package regclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"

	digest "github.com/opencontainers/go-digest"
	"github.com/sudo-bmitch/regcli/pkg/retryable"
)

func (rc *regClient) ManifestDigest(ctx context.Context, ref Ref) (digest.Digest, error) {
	host := rc.getHost(ref.Registry)
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
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
		return nil, fmt.Errorf("Unknown manifest media type %s", m.mt)
	}
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

func (rc *regClient) ManifestPut(ctx context.Context, ref Ref, m Manifest) error {
	host := rc.getHost(ref.Registry)
	if ref.Tag == "" {
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
		return fmt.Errorf("Unexpected status code on manifest put %d\nResponse object: %v\nBody: %s", resp.HTTPResponse().StatusCode, resp, body)
	}

	return nil
}
