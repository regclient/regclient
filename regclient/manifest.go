package regclient

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/regclient/regclient/pkg/retryable"
	"github.com/regclient/regclient/pkg/wraperr"
	"github.com/regclient/regclient/regclient/manifest"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

// ManifestClient provides registry client requests to manifests
type ManifestClient interface {
	ManifestDelete(ctx context.Context, ref types.Ref) error
	ManifestGet(ctx context.Context, ref types.Ref) (manifest.Manifest, error)
	ManifestHead(ctx context.Context, ref types.Ref) (manifest.Manifest, error)
	ManifestPut(ctx context.Context, ref types.Ref, m manifest.Manifest) error
}

func (rc *regClient) ManifestDelete(ctx context.Context, ref types.Ref) error {
	if ref.Digest == "" {
		return wraperr.New(fmt.Errorf("Digest required to delete manifest, reference %s", ref.CommonName()), ErrMissingDigest)
	}

	// build/send request
	headers := http.Header{
		// "Accept": []string{
		// 	MediaTypeDocker1Manifest,
		// 	MediaTypeDocker1ManifestSigned,
		// 	MediaTypeDocker2Manifest,
		// 	MediaTypeDocker2ManifestList,
		// 	MediaTypeOCI1Manifest,
		// 	MediaTypeOCI1ManifestList,
		// },
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

func (rc *regClient) ManifestGet(ctx context.Context, ref types.Ref) (manifest.Manifest, error) {
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

	// read manifest
	rawBody, err := ioutil.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("Error reading manifest for %s: %w", ref.CommonName(), err)
	}

	// parse body into variable according to media type
	mt := resp.HTTPResponse().Header.Get("Content-Type")

	return manifest.New(mt, rawBody, ref, resp.HTTPResponse().Header)
}

func (rc *regClient) ManifestHead(ctx context.Context, ref types.Ref) (manifest.Manifest, error) {
	// build the request
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

	// extract header data
	mt := resp.HTTPResponse().Header.Get("Content-Type")

	return manifest.New(mt, []byte{}, ref, resp.HTTPResponse().Header)
}

func (rc *regClient) ManifestPut(ctx context.Context, ref types.Ref, m manifest.Manifest) error {
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
