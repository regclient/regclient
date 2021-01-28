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
	"github.com/regclient/regclient/pkg/wraperr"
	"github.com/sirupsen/logrus"
)

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
	host := rc.getHost(ref.Registry)

	blobURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/blobs/" + d,
	}

	dp, err := digest.Parse(d)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"digest": d,
		}).Warn("Failed to parse digest")
		return nil, nil, fmt.Errorf("Failed to parse blob digest %s, ref %s: %w", d, ref.CommonName(), err)
	}

	headers := http.Header{}
	for _, accept := range accepts {
		headers.Add("Accept", accept)
	}

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", blobURL, retryable.WithHeaders(headers), retryable.WithDigest(dp))
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return nil, nil, fmt.Errorf("Failed to retrieve blob, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	switch resp.HTTPResponse().StatusCode {
	case 200: // success
	case 401:
		return nil, nil, wraperr.New(fmt.Errorf("Unauthorized request for blob, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 403:
		return nil, nil, wraperr.New(fmt.Errorf("Forbidden request for blob, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 404:
		return nil, nil, wraperr.New(fmt.Errorf("Blob not found: digest %s, ref %s", d, ref.CommonName()), ErrNotFound)
	case 429:
		return nil, nil, wraperr.New(fmt.Errorf("Rate limit exceeded pulling blob, digest %s, ref %s", d, ref.CommonName()), ErrRateLimit)
	default:
		return nil, nil, fmt.Errorf("Request failed for blob, digest %s, ref %s, http status %d", d, ref.CommonName(), resp.HTTPResponse().StatusCode)
	}

	return resp, resp.HTTPResponse(), nil
}

// BlobHead is used to verify if a blob exists and is accessible
func (rc *regClient) BlobHead(ctx context.Context, ref Ref, d string) error {
	host := rc.getHost(ref.Registry)

	blobURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/blobs/" + d,
	}

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "HEAD", blobURL)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()

	switch resp.HTTPResponse().StatusCode {
	case 200: // success
	case 401:
		return wraperr.New(fmt.Errorf("Unauthorized request for blob head, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 403:
		return wraperr.New(fmt.Errorf("Forbidden request for blob head, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 404:
		return wraperr.New(fmt.Errorf("Blob not found: digest %s, ref %s", d, ref.CommonName()), ErrNotFound)
	case 429:
		return wraperr.New(fmt.Errorf("Rate limit exceeded pulling blob head, digest %s, ref %s", d, ref.CommonName()), ErrRateLimit)
	default:
		return fmt.Errorf("Request failed for blob, digest %s, ref %s, http status %d", d, ref.CommonName(), resp.HTTPResponse().StatusCode)
	}

	return nil
}

func (rc *regClient) BlobMount(ctx context.Context, refSrc Ref, refTgt Ref, d string) error {
	if refSrc.Registry != refTgt.Registry {
		return fmt.Errorf("Registry must match for blob mount")
	}

	host := rc.getHost(refTgt.Registry)
	mountURL := url.URL{
		Scheme:   host.Scheme,
		Host:     host.DNS[0],
		Path:     "/v2/" + refTgt.Repository + "/blobs/uploads/",
		RawQuery: "mount=" + d + "&from=" + refSrc.Repository,
	}

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "POST", mountURL)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Error calling blob mount request, digest %s, ref %s, error: %w, response: %v", d, refTgt.CommonName(), err, resp)
	}
	defer resp.Close()

	switch resp.HTTPResponse().StatusCode {
	case 201: // success: accepted
	case 202: // success: completed
	case 204: // success: no content
	case 401:
		return wraperr.New(fmt.Errorf("Unauthorized request for blob mount, digest %s, ref %s", d, refTgt.CommonName()), ErrUnauthorized)
	case 403:
		return wraperr.New(fmt.Errorf("Forbidden request for blob mount, digest %s, ref %s", d, refTgt.CommonName()), ErrUnauthorized)
	case 404:
		return wraperr.New(fmt.Errorf("Blob repo not found: digest %s, ref %s", d, refTgt.CommonName()), ErrNotFound)
	case 429:
		return wraperr.New(fmt.Errorf("Rate limit exceeded on blob mount, digest %s, ref %s", d, refTgt.CommonName()), ErrRateLimit)
	default:
		return fmt.Errorf("Blob mount status %d != 201, digest %s, ref %s, response: %v", resp.HTTPResponse().StatusCode, d, refTgt.CommonName(), resp)
	}

	return nil
}

func (rc *regClient) BlobPut(ctx context.Context, ref Ref, d string, rdr io.ReadCloser, ct string, cl int64) error {
	if ct == "" {
		ct = "application/octet-stream"
	}
	if cl == 0 {
		cl = -1
	}

	host := rc.getHost(ref.Registry)

	// request an upload location
	uploadURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/blobs/uploads/",
	}
	rc.log.WithFields(logrus.Fields{
		"url": uploadURL.String(),
	}).Debug("Requesting upload location")
	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "POST", uploadURL)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.Reference,
		}).Warn("Error calling BlobPut")
		return fmt.Errorf("Failed sending blob put, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	switch resp.HTTPResponse().StatusCode {
	case 201: // success: accepted
	case 202: // success: completed
	case 204: // success: no content
	case 401:
		return wraperr.New(fmt.Errorf("Unauthorized request for blob upload, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 403:
		return wraperr.New(fmt.Errorf("Forbidden request for blob upload, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 404:
		return wraperr.New(fmt.Errorf("Blob repo not found: digest %s, ref %s", d, ref.CommonName()), ErrNotFound)
	case 429:
		return wraperr.New(fmt.Errorf("Rate limit exceeded on blob upload, digest %s, ref %s", d, ref.CommonName()), ErrRateLimit)
	default:
		return fmt.Errorf("Blob upload status %d != 202, digest %s, ref %s, response: %v", resp.HTTPResponse().StatusCode, d, ref.CommonName(), resp)
	}

	// extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme
	location := resp.HTTPResponse().Header.Get("Location")
	rc.log.WithFields(logrus.Fields{
		"location": location,
	}).Debug("Upload location received")
	var putURL *url.URL
	if strings.HasPrefix(location, "/") {
		location = host.Scheme + "://" + host.DNS[0] + location
	} else if !strings.Contains(location, "://") {
		location = host.Scheme + "://" + location
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
	resp, err = rty.DoRequest(ctx, "PUT", *putURL, opts...)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"url":    putURL.String(),
			"ref":    ref.Reference,
			"digest": d,
		}).Warn("Failed to upload blob")
		return fmt.Errorf("Blob upload failed, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	switch resp.HTTPResponse().StatusCode {
	case 201: // success: accepted
	case 202: // success: completed
	case 204: // success: no content
	case 401:
		return wraperr.New(fmt.Errorf("Unauthorized request for blob upload, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 403:
		return wraperr.New(fmt.Errorf("Forbidden request for blob upload, digest %s, ref %s", d, ref.CommonName()), ErrUnauthorized)
	case 404:
		return wraperr.New(fmt.Errorf("Blob repo not found: digest %s, ref %s", d, ref.CommonName()), ErrNotFound)
	case 429:
		return wraperr.New(fmt.Errorf("Rate limit exceeded on blob upload, digest %s, ref %s", d, ref.CommonName()), ErrRateLimit)
	default:
		return fmt.Errorf("Blob upload status %d != 201, digest %s, ref %s, response: %v", resp.HTTPResponse().StatusCode, d, ref.CommonName(), resp)
	}

	return nil
}
