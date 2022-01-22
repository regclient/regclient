package reg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	dockerDistribution "github.com/docker/distribution"
	dockerManifest "github.com/docker/distribution/manifest"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	ociv1Specs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/tag"
	"github.com/sirupsen/logrus"
)

// TagDelete removes a tag from a repository.
// It first attempts the newer OCI API to delete by tag name (not widely supported).
// If the OCI API fails, it falls back to pushing a unique empty manifest and deleting that.
func (reg *Reg) TagDelete(ctx context.Context, r ref.Ref) error {
	var tempManifest manifest.Manifest
	if r.Tag == "" {
		return types.ErrMissingTag
	}

	// attempt to delete the tag directly, available in OCI distribution-spec, and Hub API
	req := &reghttp.Req{
		Host:      r.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "DELETE",
				Repository: r.Repository,
				Path:       "manifests/" + r.Tag,
				IgnoreErr:  true, // do not trigger backoffs if this fails
			},
			"hub": {
				Method: "DELETE",
				Path:   "repositories/" + r.Repository + "/tags/" + r.Tag + "/",
			},
		},
	}

	resp, err := reg.reghttp.Do(ctx, req)
	if resp != nil {
		defer resp.Close()
	}
	// TODO: Hub may return a different status
	if err == nil && resp != nil && resp.HTTPResponse().StatusCode == 202 {
		return nil
	}
	// ignore errors, fallback to creating a temporary manifest to replace the tag and deleting that manifest

	// lookup the current manifest media type
	curManifest, err := reg.ManifestHead(ctx, r)
	if err != nil && errors.Is(err, types.ErrUnsupportedAPI) {
		curManifest, err = reg.ManifestGet(ctx, r)
	}
	if err != nil {
		return err
	}

	// create empty image config with single label
	// Note, this should be MediaType specific, but it appears that docker uses OCI for the config
	now := time.Now()
	conf := ociv1.Image{
		Created: &now,
		Config: ociv1.ImageConfig{
			Labels: map[string]string{
				"delete-tag":  r.Tag,
				"delete-date": now.String(),
			},
		},
		OS:           "linux",
		Architecture: "amd64",
		RootFS: ociv1.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{},
		},
	}
	confB, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	digester := digest.Canonical.Digester()
	confBuf := bytes.NewBuffer(confB)
	_, err = confBuf.WriteTo(digester.Hash())
	if err != nil {
		return err
	}
	confDigest := digester.Digest()

	// create manifest with config, matching the original tag manifest type
	switch curManifest.GetMediaType() {
	case types.MediaTypeOCI1Manifest, types.MediaTypeOCI1ManifestList:
		tempManifest, err = manifest.New(manifest.WithOrig(ociv1.Manifest{
			Versioned: ociv1Specs.Versioned{
				SchemaVersion: 2,
			},
			MediaType: types.MediaTypeOCI1Manifest,
			Config: ociv1.Descriptor{
				MediaType: types.MediaTypeOCI1ImageConfig,
				Digest:    confDigest,
				Size:      int64(len(confB)),
			},
			Layers: []ociv1.Descriptor{},
		}))
		if err != nil {
			return err
		}
	default: // default to the docker v2 schema
		tempManifest, err = manifest.New(manifest.WithOrig(dockerSchema2.Manifest{
			Versioned: dockerManifest.Versioned{
				SchemaVersion: 2,
				MediaType:     types.MediaTypeDocker2Manifest,
			},
			Config: dockerDistribution.Descriptor{
				MediaType: types.MediaTypeDocker2ImageConfig,
				Digest:    confDigest,
				Size:      int64(len(confB)),
			},
			Layers: []dockerDistribution.Descriptor{},
		}))
		if err != nil {
			return err
		}
	}
	reg.log.WithFields(logrus.Fields{
		"ref": r.Reference,
	}).Debug("Sending dummy manifest to replace tag")

	// push config
	_, _, err = reg.BlobPut(ctx, r, confDigest, bytes.NewReader(confB), int64(len(confB)))
	if err != nil {
		return fmt.Errorf("Failed sending dummy config to delete %s: %w", r.CommonName(), err)
	}

	// push manifest to tag
	err = reg.ManifestPut(ctx, r, tempManifest)
	if err != nil {
		return fmt.Errorf("Failed sending dummy manifest to delete %s: %w", r.CommonName(), err)
	}

	r.Digest = tempManifest.GetDigest().String()

	// delete manifest by digest
	reg.log.WithFields(logrus.Fields{
		"ref":    r.Reference,
		"digest": r.Digest,
	}).Debug("Deleting dummy manifest")
	err = reg.ManifestDelete(ctx, r)
	if err != nil {
		return fmt.Errorf("Failed deleting dummy manifest for %s: %w", r.CommonName(), err)
	}

	return nil
}

// TagList returns a listing to tags from the repository
func (reg *Reg) TagList(ctx context.Context, r ref.Ref, opts ...scheme.TagOpts) (*tag.TagList, error) {
	var config scheme.TagConfig
	for _, opt := range opts {
		opt(&config)
	}

	query := url.Values{}
	if config.Last != "" {
		query.Set("last", config.Last)
	}
	if config.Limit > 0 {
		query.Set("n", strconv.Itoa(config.Limit))
	}
	headers := http.Header{
		"Accept": []string{"application/json"},
	}
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "GET",
				Repository: r.Repository,
				Path:       "tags/list",
				Query:      query,
				Headers:    headers,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to list tags for %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to list tags for %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		reg.log.WithFields(logrus.Fields{
			"err": err,
			"ref": r.CommonName(),
		}).Warn("Failed to read tag list")
		return nil, fmt.Errorf("Failed to read tags for %s: %w", r.CommonName(), err)
	}
	mt := resp.HTTPResponse().Header.Get("Content-Type")
	tl, err := tag.New(
		tag.WithMT(mt),
		tag.WithRaw(respBody),
		tag.WithRef(r),
		tag.WithHeaders(resp.HTTPResponse().Header),
	)
	if err != nil {
		reg.log.WithFields(logrus.Fields{
			"err":  err,
			"body": respBody,
			"ref":  r.CommonName(),
		}).Warn("Failed to unmarshal tag list")
		return tl, fmt.Errorf("Failed to unmarshal tag list for %s: %w", r.CommonName(), err)
	}

	return tl, nil
}
