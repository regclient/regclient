package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
	"time"

	dockerDistribution "github.com/docker/distribution"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/regclient/regclient/pkg/wraperr"
	"github.com/sirupsen/logrus"
)

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
	if ref.Tag == "" {
		return ErrMissingTag
	}
	// host := rc.getHost(ref.Registry)

	// create empty image config with single label
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

	// create manifest with config
	manf := dockerSchema2.Manifest{
		Config: dockerDistribution.Descriptor{
			MediaType: MediaTypeDocker2ImageConfig,
			Digest:    confDigest,
			Size:      int64(len(confB)),
		},
		Layers: []dockerDistribution.Descriptor{},
	}
	manf.SchemaVersion = 2
	manf.MediaType = MediaTypeDocker2Manifest
	manfB, err := json.Marshal(manf)
	if err != nil {
		return err
	}
	digester = digest.Canonical.Digester()
	manfBuf := bytes.NewBuffer(manfB)
	_, err = manfBuf.WriteTo(digester.Hash())
	if err != nil {
		return err
	}
	manfDigest := digester.Digest()
	m := manifest{
		digest:   manfDigest,
		dockerM:  manf,
		manifSet: true,
		mt:       MediaTypeDocker2Manifest,
		origByte: manfB,
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
	err = rc.ManifestPut(ctx, ref, &m)
	if err != nil {
		return fmt.Errorf("Failed sending dummy manifest to delete %s: %w", ref.CommonName(), err)
	}

	ref.Digest = manfDigest.String()

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
	host := rc.getHost(ref.Registry)
	repoURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/tags/list",
	}
	query := url.Values{}
	if opts.Last != "" {
		query.Set("last", opts.Last)
	}
	if opts.Limit > 0 {
		query.Set("n", strconv.Itoa(opts.Limit))
	}
	repoURL.RawQuery = query.Encode()

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", repoURL)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.CommonName(),
		}).Warn("Failed to list tags")
		return tl, fmt.Errorf("Failed to list tags for %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	switch resp.HTTPResponse().StatusCode {
	case 200: // success
	case 401:
		return tl, wraperr.New(fmt.Errorf("Unauthorized request for tags %s", ref.CommonName()), ErrUnauthorized)
	case 403:
		return tl, wraperr.New(fmt.Errorf("Forbidden request for tags %s", ref.CommonName()), ErrUnauthorized)
	case 404:
		return tl, wraperr.New(fmt.Errorf("Repository not found: %s", ref.CommonName()), ErrNotFound)
	case 429:
		return tl, wraperr.New(fmt.Errorf("Rate limit exceeded pulling tags %s", ref.CommonName()), ErrRateLimit)
	default:
		return tl, fmt.Errorf("Error getting tags for %s: http response code %d != 200", ref.CommonName(), resp.HTTPResponse().StatusCode)
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
