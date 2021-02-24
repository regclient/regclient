package regclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

// BlobClient provides registry client requests to Blobs
type BlobClient interface {
	BlobCopy(ctx context.Context, refSrc Ref, refTgt Ref, d string) error
	BlobGet(ctx context.Context, ref Ref, d string, accepts []string) (io.ReadCloser, *http.Response, error)
	BlobMount(ctx context.Context, refSrc Ref, refTgt Ref, d string) error
	BlobPut(ctx context.Context, ref Ref, d string, rdr io.ReadCloser, ct string, cl int64) error
}

func (rc *regClient) BlobCopy(ctx context.Context, refSrc Ref, refTgt Ref, d string) error {
	// for the same repository, there's nothing to copy
	if refSrc.Registry == refTgt.Registry && refSrc.Repository == refTgt.Repository {
		rc.log.WithFields(logrus.Fields{
			"src":    refTgt.Reference,
			"tgt":    refTgt.Reference,
			"digest": d,
		}).Debug("Blob copy skipped, same repo")
		return nil
	}
	// check if layer already exists
	if err := rc.BlobHead(ctx, refTgt, d); err == nil {
		rc.log.WithFields(logrus.Fields{
			"tgt":    refTgt.Reference,
			"digest": d,
		}).Debug("Blob copy skipped, already exists")
		return nil
	}
	// try mounting blob from the source repo is the registry is the same
	if refSrc.Registry == refTgt.Registry {
		err := rc.BlobMount(ctx, refSrc, refTgt, d)
		if err == nil {
			rc.log.WithFields(logrus.Fields{
				"src":    refTgt.Reference,
				"tgt":    refTgt.Reference,
				"digest": d,
			}).Debug("Blob copy performed server side with registry mount")
			return nil
		}
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to mount blob")
	}
	// fast options failed, download layer from source and push to target
	blobIO, layerResp, err := rc.BlobGet(ctx, refSrc, d, []string{})
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"src":    refSrc.Reference,
			"digest": d,
		}).Warn("Failed to retrieve blob")
		return err
	}
	defer blobIO.Close()
	if err := rc.BlobPut(ctx, refTgt, d, blobIO, layerResp.Header.Get("Content-Type"), layerResp.ContentLength); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to push blob")
		return err
	}
	return nil
}

func (rc *regClient) BlobGet(ctx context.Context, ref Ref, d string, accepts []string) (io.ReadCloser, *http.Response, error) {
	dp, err := digest.Parse(d)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"digest": d,
		}).Warn("Failed to parse digest")
		return nil, nil, fmt.Errorf("Failed to parse blob digest %s, ref %s: %w", d, ref.CommonName(), err)
	}

	// build/send request
	headers := http.Header{
		"Accept": accepts,
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:  "GET",
				path:    ref.Repository + "/blobs/" + d,
				headers: headers,
				digest:  dp,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return nil, nil, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, nil, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return resp, resp.HTTPResponse(), nil
}

// BlobHead is used to verify if a blob exists and is accessible
func (rc *regClient) BlobHead(ctx context.Context, ref Ref, d string) error {
	// build/send request
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method: "HEAD",
				path:   ref.Repository + "/blobs/" + d,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}

func (rc *regClient) BlobMount(ctx context.Context, refSrc Ref, refTgt Ref, d string) error {
	if refSrc.Registry != refTgt.Registry {
		return fmt.Errorf("Registry must match for blob mount")
	}

	// build/send request
	query := url.Values{}
	query.Set("mount", d)
	query.Set("from", refSrc.Repository)

	req := httpReq{
		host:      refTgt.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method: "POST",
				path:   refTgt.Repository + "/blobs/uploads/",
				query:  query,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, refTgt.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, refTgt.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}

// TODO: use BlobPut to wrap 3 types of uploads: PUT with chunks, PUT without chunks (implemented here), single POST
func (rc *regClient) BlobPut(ctx context.Context, ref Ref, d string, rdr io.ReadCloser, ct string, cl int64) error {
	// defaults for content-type and length
	if ct == "" {
		ct = "application/octet-stream"
	}
	if cl == 0 {
		cl = -1
	}

	// request an upload location
	req := httpReq{
		host:      ref.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method: "POST",
				path:   ref.Repository + "/blobs/uploads/",
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to send blob post, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Failed to send blob post, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	// Extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme.
	// This doesn't use the httpDo method since location could point to any url, negating the API expansion, mirror handling, and similar features.
	host := rc.hostGet(ref.Registry)
	scheme := "https"
	if host.TLS == TLSDisabled {
		scheme = "http"
	}
	location := resp.HTTPResponse().Header.Get("Location")
	rc.log.WithFields(logrus.Fields{
		"location": location,
	}).Debug("Upload location received")
	var putURL *url.URL
	if strings.HasPrefix(location, "/") {
		location = scheme + "://" + host.DNS[0] + location
	} else if !strings.Contains(location, "://") {
		location = scheme + "://" + location
	}
	putURL, err = url.Parse(location)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"location": location,
			"err":      err,
		}).Warn("Location url failed to parse")
		return fmt.Errorf("Blob upload url invalid, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}

	// append digest to request to use the monolithic upload option
	if putURL.RawQuery != "" {
		putURL.RawQuery = putURL.RawQuery + "&digest=" + d
	} else {
		putURL.RawQuery = "digest=" + d
	}

	// send the blob
	opts := []retryable.OptsReq{}
	bodyFunc := func() (io.ReadCloser, error) {
		return ioutil.NopCloser(rdr), nil
	}
	opts = append(opts, retryable.WithBodyFunc(bodyFunc))
	opts = append(opts, retryable.WithContentLen(cl))
	opts = append(opts, retryable.WithHeader("Content-Type", []string{ct}))
	rty := rc.getRetryable(host)
	resp, err = rty.DoRequest(ctx, "PUT", []url.URL{*putURL}, opts...)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to send blob put, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Failed to send blob put, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return nil
}
