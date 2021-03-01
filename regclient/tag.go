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
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

// TagClient wraps calls to tag list and delete
type TagClient interface {
	TagDelete(ctx context.Context, ref Ref) error
	TagList(ctx context.Context, ref Ref) (TagList, error)
	TagListWithOpts(ctx context.Context, ref Ref, opts TagOpts) (TagList, error)
}

// TagList comes from github.com/opencontainers/distribution-spec
// TODO: switch to their implementation when it becomes stable
// TODO: rename to avoid confusion with (*regClient).TagList
type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// TagOpts is used for options to the tag functions
type TagOpts struct {
	Limit int
	Last  string
}

// TagDelete deletes a tag from the registry. Since there's no API for this,
// you'd want to normally just delete the manifest. However multiple tags may
// point to the same manifest, so instead you must:
// 1. Make a manifest, for this we put a few labels and timestamps to be unique.
// 2. Push that manifest to the tag.
// 3. Delete the digest for that new manifest that is only used by that tag.
func (rc *regClient) TagDelete(ctx context.Context, ref Ref) error {
	var tempManifest Manifest
	if ref.Tag == "" {
		return ErrMissingTag
	}

	// lookup the current manifest media type
	curManifest, _ := rc.ManifestHead(ctx, ref)

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
	switch curManifest.(type) {
	case *manifestOCIM, *manifestOCIML:
		om := ociv1.Manifest{
			Versioned: ociv1Specs.Versioned{
				SchemaVersion: 1,
			},
			Config: ociv1.Descriptor{
				MediaType: MediaTypeOCI1ImageConfig,
				Digest:    confDigest,
				Size:      int64(len(confB)),
			},
			Layers: []ociv1.Descriptor{},
		}
		omBytes, err := json.Marshal(om)
		if err != nil {
			return err
		}
		digester = digest.Canonical.Digester()
		manfBuf := bytes.NewBuffer(omBytes)
		_, err = manfBuf.WriteTo(digester.Hash())
		if err != nil {
			return err
		}
		tempManifest = &manifestOCIM{
			manifestCommon: manifestCommon{
				mt:       MediaTypeOCI1Manifest,
				ref:      ref,
				orig:     om,
				rawBody:  omBytes,
				digest:   digester.Digest(),
				manifSet: true,
			},
			Manifest: om,
		}
	default: // default to the docker v2 schema
		dm := dockerSchema2.Manifest{
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
		}
		dmBytes, err := json.Marshal(dm)
		if err != nil {
			return err
		}
		digester = digest.Canonical.Digester()
		manfBuf := bytes.NewBuffer(dmBytes)
		_, err = manfBuf.WriteTo(digester.Hash())
		if err != nil {
			return err
		}
		tempManifest = &manifestDockerM{
			manifestCommon: manifestCommon{
				mt:       MediaTypeOCI1Manifest,
				ref:      ref,
				orig:     dm,
				rawBody:  dmBytes,
				digest:   digester.Digest(),
				manifSet: true,
			},
			Manifest: dm,
		}
	}

	rc.log.WithFields(logrus.Fields{
		"ref": ref.Reference,
	}).Debug("Sending dummy manifest to replace tag")

	// push config
	err = rc.BlobPut(ctx, ref, confDigest.String(), ioutil.NopCloser(bytes.NewReader(confB)), MediaTypeDocker2ImageConfig, int64(len(confB)))
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

func (rc *regClient) TagList(ctx context.Context, ref Ref) (TagList, error) {
	return rc.TagListWithOpts(ctx, ref, TagOpts{})
}

func (rc *regClient) TagListWithOpts(ctx context.Context, ref Ref, opts TagOpts) (TagList, error) {
	tl := TagList{}
	query := url.Values{}
	if opts.Last != "" {
		query.Set("last", opts.Last)
	}
	if opts.Limit > 0 {
		query.Set("n", strconv.Itoa(opts.Limit))
	}
	headers := http.Header{
		"Accept": []string{"application/json"},
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:  "GET",
				path:    ref.Repository + "/tags/list",
				query:   query,
				headers: headers,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return tl, fmt.Errorf("Failed to list tags for %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return tl, fmt.Errorf("Failed to list tags for %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.CommonName(),
		}).Warn("Failed to read tag list")
		return tl, fmt.Errorf("Failed to read tags for %s: %w", ref.CommonName(), err)
	}
	err = json.Unmarshal(respBody, &tl)
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

// MarshalPretty is used for printPretty template formatting
func (tl TagList) MarshalPretty() ([]byte, error) {
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
