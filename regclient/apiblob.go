package regclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/sudo-bmitch/regcli/pkg/retryable"
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
		return nil, nil, err
	}

	headers := http.Header{}
	for _, accept := range accepts {
		headers.Add("Accept", accept)
	}

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", blobURL, retryable.WithHeaders(headers), retryable.WithDigest(dp))
	if err != nil {
		return nil, nil, err
	}
	return resp, resp.HTTPResponse(), nil
}

func (rc *regClient) BlobHead(ctx context.Context, ref Ref, d string) error {
	host := rc.getHost(ref.Registry)

	blobURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/blobs/" + d,
	}

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "HEAD", blobURL)
	if err != nil {
		return err
	}
	defer resp.Close()

	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return ErrNotFound
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
	if err != nil {
		return fmt.Errorf("Error calling blob mount request: %w\nResponse object: %v", err, resp)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 201 {
		return fmt.Errorf("Blob mount did not return a 201 status, status code: %d\nResponse object: %v", resp.HTTPResponse().StatusCode, resp)
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
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"ref": ref.Reference,
		}).Warn("Error calling BlobPut")
		return err
	}
	if resp.HTTPResponse().StatusCode != 202 {
		rc.log.WithFields(logrus.Fields{
			"statusCode": resp.HTTPResponse().StatusCode,
			"url":        uploadURL.String(),
		}).Warn("Unexpected status code on BlobPut")
		return fmt.Errorf("Blob upload request did not return a 202 status, status code: %d\nResponse object: %v", resp.HTTPResponse().StatusCode, resp)
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
		return err
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
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"url":    putURL.String(),
			"ref":    ref.Reference,
			"digest": d,
		}).Warn("Failed to upload blob")
		return err
	}
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		rc.log.WithFields(logrus.Fields{
			"statusCode": resp.HTTPResponse().StatusCode,
			"url":        putURL.String(),
			"ref":        ref.Reference,
			"digest":     d,
		}).Warn("Unexpected status code while uploading blob")
		return fmt.Errorf("Blob put request status code: %d\nResponse object: %v", resp.HTTPResponse().StatusCode, resp)
	}

	return nil
}
