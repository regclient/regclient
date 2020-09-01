package regclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/sudo-bmitch/regcli/pkg/retryable"
)

func (rc *regClient) BlobCopy(ctx context.Context, refSrc Ref, refTgt Ref, d string) error {
	// for the same repository, there's nothing to copy
	if refSrc.Repository == refTgt.Repository {
		return nil
	}
	// check if layer already exists
	if err := rc.BlobHead(ctx, refTgt, d); err == nil {
		return nil
	}
	// try mounting blob from the source repo is the registry is the same
	if refSrc.Registry == refTgt.Registry {
		err := rc.BlobMount(ctx, refSrc, refTgt, d)
		if err == nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "Failed to mount blob: %s\n", err)
	}
	// fast options failed, download layer from source and push to target
	blobIO, layerResp, err := rc.BlobGet(ctx, refSrc, d, []string{})
	if err != nil {
		return err
	}
	if err := rc.BlobPut(ctx, refTgt, d, blobIO, layerResp.Header.Get("Content-Type"), layerResp.ContentLength); err != nil {
		blobIO.Close()
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

	headers := http.Header{}
	for _, accept := range accepts {
		headers.Add("Accept", accept)
	}

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", blobURL, retryable.WithHeaders(headers))
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
	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "POST", uploadURL)
	if err != nil {
		return err
	}
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("Blob upload request did not return a 202 status, status code: %d\nResponse object: %v", resp.HTTPResponse().StatusCode, resp)
	}

	// extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme
	location := resp.HTTPResponse().Header.Get("Location")
	fmt.Fprintf(os.Stderr, "Upload location received: %s", location)
	var putURL *url.URL
	if strings.HasPrefix(location, "/") {
		location = host.Scheme + "://" + host.DNS[0] + location
	} else if !strings.Contains(location, "://") {
		location = host.Scheme + "://" + location
	}
	putURL, err = url.Parse(location)
	if err != nil {
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
		return err
	}
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Blob put request status code: %d\nRequest object: %v\nResponse object: %v", resp.HTTPResponse().StatusCode, resp)
	}

	return nil
}
