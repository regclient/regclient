package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	dockerDistribution "github.com/docker/distribution"
	dockerManifest "github.com/docker/distribution/manifest"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	ociv1Specs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/regclient/manifest"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

type ociTagAPI interface {
	TagDelete(ctx context.Context, ref types.Ref) error
	TagList(ctx context.Context, ref types.Ref, opts ...TagOpts) (TagList, error)
}

// TODO: consider a tag interface for future uses
// type Tag interface {
// 	GetOrig() interface{}
// 	MarshalJSON() ([]byte, error)
// 	RawBody() ([]byte, error)
// 	RawHeaders() (http.Header, error)
// }

// TagList interface is used for listing tags
type TagList = types.TagList

type tagCommon struct {
	ref       types.Ref
	mt        string
	orig      interface{}
	rawHeader http.Header
	rawBody   []byte
}

type tagDockerList struct {
	tagCommon
	TagDockerList
}

// TagDockerList is returned from registry/2.0 API's
type TagDockerList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// TagOpts is used for options to the tag functions
type tagOpts struct {
	Limit int
	Last  string
}
type TagOpts func(*tagOpts)

func TagOptLimit(limit int) TagOpts {
	return func(t *tagOpts) {
		t.Limit = limit
	}
}
func TagOptLast(last string) TagOpts {
	return func(t *tagOpts) {
		t.Last = last
	}
}

// TagDelete deletes a tag from the registry. Since there's no API for this,
// you'd want to normally just delete the manifest. However multiple tags may
// point to the same manifest, so instead you must:
// 1. Make a manifest, for this we put a few labels and timestamps to be unique.
// 2. Push that manifest to the tag.
// 3. Delete the digest for that new manifest that is only used by that tag.
func (rc *Client) TagDelete(ctx context.Context, ref types.Ref) error {
	var tempManifest manifest.Manifest
	if ref.Tag == "" {
		return ErrMissingTag
	}

	// attempt to delete the tag directly, available in OCI distribution-spec, and Hub API
	req := httpReq{
		host:      ref.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method:     "DELETE",
				repository: ref.Repository,
				path:       "manifests/" + ref.Tag,
				ignoreErr:  true, // do not trigger backoffs if this fails
			},
			"hub": {
				method: "DELETE",
				path:   "repositories/" + ref.Repository + "/tags/" + ref.Tag + "/",
			},
		},
	}

	resp, err := rc.httpDo(ctx, req)
	if resp != nil {
		defer resp.Close()
	}
	// TODO: Hub may return a different status
	if err == nil && resp != nil && resp.HTTPResponse().StatusCode == 202 {
		return nil
	}
	// ignore errors, fallback to creating a temporary manifest to replace the tag and deleting that manifest

	// lookup the current manifest media type
	curManifest, err := rc.ManifestHead(ctx, ref)
	if err != nil && errors.Is(err, ErrUnsupportedAPI) {
		curManifest, err = rc.ManifestGet(ctx, ref)
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
				"delete-tag":  ref.Tag,
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
	case MediaTypeOCI1Manifest, MediaTypeOCI1ManifestList:
		tempManifest, err = manifest.FromOrig(ociv1.Manifest{
			Versioned: ociv1Specs.Versioned{
				SchemaVersion: 2,
			},
			MediaType: MediaTypeOCI1Manifest,
			Config: ociv1.Descriptor{
				MediaType: MediaTypeOCI1ImageConfig,
				Digest:    confDigest,
				Size:      int64(len(confB)),
			},
			Layers: []ociv1.Descriptor{},
		})
		if err != nil {
			return err
		}
	default: // default to the docker v2 schema
		tempManifest, err = manifest.FromOrig(dockerSchema2.Manifest{
			Versioned: dockerManifest.Versioned{
				SchemaVersion: 2,
				MediaType:     MediaTypeDocker2Manifest,
			},
			Config: dockerDistribution.Descriptor{
				MediaType: MediaTypeDocker2ImageConfig,
				Digest:    confDigest,
				Size:      int64(len(confB)),
			},
			Layers: []dockerDistribution.Descriptor{},
		})
		if err != nil {
			return err
		}
	}

	rc.log.WithFields(logrus.Fields{
		"ref": ref.Reference,
	}).Debug("Sending dummy manifest to replace tag")

	// push config
	_, _, err = rc.BlobPut(ctx, ref, confDigest, ioutil.NopCloser(bytes.NewReader(confB)), int64(len(confB)))
	if err != nil {
		return fmt.Errorf("Failed sending dummy config to delete %s: %w", ref.CommonName(), err)
	}

	// push manifest to tag
	err = rc.ManifestPut(ctx, ref, tempManifest)
	if err != nil {
		return fmt.Errorf("Failed sending dummy manifest to delete %s: %w", ref.CommonName(), err)
	}

	ref.Digest = tempManifest.GetDigest().String()

	// delete manifest by digest
	rc.log.WithFields(logrus.Fields{
		"ref":    ref.Reference,
		"digest": ref.Digest,
	}).Debug("Deleting dummy manifest")
	err = rc.ManifestDelete(ctx, ref)
	if err != nil {
		return fmt.Errorf("Failed deleting dummy manifest for %s: %w", ref.CommonName(), err)
	}

	return nil
}

func (rc *Client) TagList(ctx context.Context, ref types.Ref, opts ...TagOpts) (TagList, error) {
	var tl TagList
	var tOpts tagOpts
	for _, opt := range opts {
		opt(&tOpts)
	}

	tc := tagCommon{
		ref: ref,
	}
	query := url.Values{}
	if tOpts.Last != "" {
		query.Set("last", tOpts.Last)
	}
	if tOpts.Limit > 0 {
		query.Set("n", strconv.Itoa(tOpts.Limit))
	}
	headers := http.Header{
		"Accept": []string{"application/json"},
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:     "GET",
				repository: ref.Repository,
				path:       "tags/list",
				query:      query,
				headers:    headers,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
		return tl, fmt.Errorf("Failed to list tags for %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return tl, fmt.Errorf("Failed to list tags for %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	tc.rawHeader = resp.HTTPResponse().Header
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.CommonName(),
		}).Warn("Failed to read tag list")
		return tl, fmt.Errorf("Failed to read tags for %s: %w", ref.CommonName(), err)
	}
	tc.rawBody = respBody
	tc.mt = resp.HTTPResponse().Header.Get("Content-Type")
	mt := strings.Split(tc.mt, ";")[0] // "application/json; charset=utf-8" -> "application/json"
	switch mt {
	case "application/json", "text/plain":
		var tdl TagDockerList
		err = json.Unmarshal(respBody, &tdl)
		tStruct := tagDockerList{
			tagCommon:     tc,
			TagDockerList: tdl,
		}
		tStruct.orig = &tStruct.TagDockerList
		tl = tStruct
	default:
		return tl, fmt.Errorf("%w: media type: %s, reference: %s", ErrUnsupportedMediaType, mt, ref.CommonName())
	}
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":  err,
			"body": respBody,
			"ref":  ref.CommonName(),
		}).Warn("Failed to unmarshal tag list")
		return tl, fmt.Errorf("Failed to unmarshal tag list for %s: %w", ref.CommonName(), err)
	}

	return tl, nil
}

func (t tagCommon) GetOrig() interface{} {
	return t.orig
}

func (t tagCommon) MarshalJSON() ([]byte, error) {
	if len(t.rawBody) > 0 {
		return t.rawBody, nil
	}

	if t.orig != nil {
		return json.Marshal((t.orig))
	}
	return []byte{}, fmt.Errorf("Json marshalling failed: %w", ErrNotFound)
}

func (t tagCommon) RawBody() ([]byte, error) {
	return t.rawBody, nil
}

func (t tagCommon) RawHeaders() (http.Header, error) {
	return t.rawHeader, nil
}

// GetTags returns the tags from a list
func (tl TagDockerList) GetTags() ([]string, error) {
	return tl.Tags, nil
}

// MarshalPretty is used for printPretty template formatting
func (tl TagDockerList) MarshalPretty() ([]byte, error) {
	sort.Slice(tl.Tags, func(i, j int) bool {
		if strings.Compare(tl.Tags[i], tl.Tags[j]) < 0 {
			return true
		}
		return false
	})
	buf := &bytes.Buffer{}
	for _, tag := range tl.Tags {
		fmt.Fprintf(buf, "%s\n", tag)
	}
	return buf.Bytes(), nil
}
