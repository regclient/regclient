package reg

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/internal/wraperr"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

// ManifestDelete removes a manifest by reference (digest) from a registry.
// This will implicitly delete all tags pointing to that manifest.
func (reg *Reg) ManifestDelete(ctx context.Context, r ref.Ref) error {
	if r.Digest == "" {
		return wraperr.New(fmt.Errorf("Digest required to delete manifest, reference %s", r.CommonName()), types.ErrMissingDigest)
	}

	// build/send request
	req := &reghttp.Req{
		Host:      r.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "DELETE",
				Repository: r.Repository,
				Path:       "manifests/" + r.Digest,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("Failed to delete manifest %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("Failed to delete manifest %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}

// ManifestGet retrieves a manifest from the registry
func (reg *Reg) ManifestGet(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	var tagOrDigest string
	if r.Digest != "" {
		tagOrDigest = r.Digest
	} else if r.Tag != "" {
		tagOrDigest = r.Tag
	} else {
		return nil, wraperr.New(fmt.Errorf("Reference missing tag and digest: %s", r.CommonName()), types.ErrMissingTagOrDigest)
	}

	// build/send request
	headers := http.Header{
		"Accept": []string{
			types.MediaTypeDocker1Manifest,
			types.MediaTypeDocker1ManifestSigned,
			types.MediaTypeDocker2Manifest,
			types.MediaTypeDocker2ManifestList,
			types.MediaTypeOCI1Manifest,
			types.MediaTypeOCI1ManifestList,
		},
	}
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "GET",
				Repository: r.Repository,
				Path:       "manifests/" + tagOrDigest,
				Headers:    headers,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to get manifest %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to get manifest %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	// read manifest
	rawBody, err := io.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("Error reading manifest for %s: %w", r.CommonName(), err)
	}

	return manifest.New(
		manifest.WithRef(r),
		manifest.WithHeader(resp.HTTPResponse().Header),
		manifest.WithRaw(rawBody),
	)
}

// ManifestHead returns metadata on the manifest from the registry
func (reg *Reg) ManifestHead(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	// build the request
	var tagOrDigest string
	if r.Digest != "" {
		tagOrDigest = r.Digest
	} else if r.Tag != "" {
		tagOrDigest = r.Tag
	} else {
		return nil, wraperr.New(fmt.Errorf("Reference missing tag and digest: %s", r.CommonName()), types.ErrMissingTagOrDigest)
	}

	// build/send request
	headers := http.Header{
		"Accept": []string{
			types.MediaTypeDocker1Manifest,
			types.MediaTypeDocker1ManifestSigned,
			types.MediaTypeDocker2Manifest,
			types.MediaTypeDocker2ManifestList,
			types.MediaTypeOCI1Manifest,
			types.MediaTypeOCI1ManifestList,
		},
	}
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "HEAD",
				Repository: r.Repository,
				Path:       "manifests/" + tagOrDigest,
				Headers:    headers,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to request manifest head %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to request manifest head %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	return manifest.New(
		manifest.WithRef(r),
		manifest.WithHeader(resp.HTTPResponse().Header),
	)
}

// ManifestPut uploads a manifest to a registry
func (reg *Reg) ManifestPut(ctx context.Context, r ref.Ref, m manifest.Manifest) error {
	var tagOrDigest string
	if r.Digest != "" {
		tagOrDigest = r.Digest
	} else if r.Tag != "" {
		tagOrDigest = r.Tag
	} else {
		reg.log.WithFields(logrus.Fields{
			"ref": r.Reference,
		}).Warn("Manifest put requires a tag")
		return types.ErrMissingTag
	}

	// create the request body
	mj, err := m.MarshalJSON()
	if err != nil {
		reg.log.WithFields(logrus.Fields{
			"ref": r.Reference,
			"err": err,
		}).Warn("Error marshaling manifest")
		return fmt.Errorf("Error marshalling manifest for %s: %w", r.CommonName(), err)
	}

	// build/send request
	headers := http.Header{
		"Content-Type": []string{m.GetMediaType()},
	}
	req := &reghttp.Req{
		Host:      r.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "PUT",
				Repository: r.Repository,
				Path:       "manifests/" + tagOrDigest,
				Headers:    headers,
				BodyLen:    int64(len(mj)),
				BodyBytes:  mj,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("Failed to put manifest %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 201 {
		return fmt.Errorf("Failed to put manifest %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}
