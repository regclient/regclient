package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

// BlobClient provides registry client requests to Blobs
type BlobClient interface {
	BlobCopy(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error
	BlobGet(ctx context.Context, ref types.Ref, d digest.Digest, accepts []string) (BlobReader, error)
	BlobGetOCIConfig(ctx context.Context, ref types.Ref, d digest.Digest) (BlobOCIConfig, error)
	BlobMount(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error
	BlobPut(ctx context.Context, ref types.Ref, d digest.Digest, rdr io.Reader, ct string, cl int64) (digest.Digest, error)
}

// Blob interface is used for returning blobs
type Blob interface {
	GetOrig() interface{}
	MediaType() string
	Response() *http.Response
	RawHeaders() (http.Header, error)
	RawBody() ([]byte, error)
}

// BlobReader is an unprocessed Blob with an available ReadCloser for reading the Blob
type BlobReader interface {
	Blob
	io.ReadCloser
}

// BlobOCIConfig wraps an OCI Config struct extracted from a Blob
type BlobOCIConfig interface {
	Blob
	GetConfig() ociv1.Image
}

type blobCommon struct {
	ref       types.Ref
	digest    string
	mt        string
	orig      interface{}
	rawHeader http.Header
	resp      *http.Response
}

// BlobReader is an unprocessed Blob with an available ReadCloser for reading the Blob
type blobReader struct {
	blobCommon
	io.ReadCloser
}

// blobOCIConfig includes an OCI Config struct extracted from a Blob
// Image is included as an anonymous field to facilitate json and templating calls transparently
type blobOCIConfig struct {
	blobCommon
	rawBody []byte
	ociv1.Image
}

func (rc *regClient) BlobCopy(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error {
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
	blobIO, err := rc.BlobGet(ctx, refSrc, d, []string{})
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"src":    refSrc.Reference,
			"digest": d,
		}).Warn("Failed to retrieve blob")
		return err
	}
	defer blobIO.Close()
	if _, err := rc.BlobPut(ctx, refTgt, d, blobIO, blobIO.MediaType(), blobIO.Response().ContentLength); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to push blob")
		return err
	}
	return nil
}

func (rc *regClient) BlobGet(ctx context.Context, ref types.Ref, d digest.Digest, accepts []string) (BlobReader, error) {
	return rc.blobGet(ctx, ref, d, accepts)
}

func (rc *regClient) blobGet(ctx context.Context, ref types.Ref, d digest.Digest, accepts []string) (blobReader, error) {
	var b blobReader
	bc := blobCommon{
		ref:    ref,
		digest: d.String(),
	}

	// build/send request
	headers := http.Header{}
	if len(accepts) > 0 {
		headers["Accept"] = accepts
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:  "GET",
				path:    ref.Repository + "/blobs/" + d.String(),
				headers: headers,
				digest:  d,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return b, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	if resp.HTTPResponse().StatusCode != 200 {
		return b, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	bc.resp = resp.HTTPResponse()
	bc.rawHeader = resp.HTTPResponse().Header
	bc.mt = resp.HTTPResponse().Header.Get("Content-Type")
	b = blobReader{
		blobCommon: bc,
		ReadCloser: resp,
	}
	return b, nil
}

func (rc *regClient) BlobGetOCIConfig(ctx context.Context, ref types.Ref, d digest.Digest) (BlobOCIConfig, error) {
	b, err := rc.blobGet(ctx, ref, d, []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return blobOCIConfig{}, err
	}
	return b.toOCIConfig()
}

// BlobHead is used to verify if a blob exists and is accessible
// TODO: on success, return a Blob with non-content data configured
func (rc *regClient) BlobHead(ctx context.Context, ref types.Ref, d digest.Digest) error {
	// build/send request
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method: "HEAD",
				path:   ref.Repository + "/blobs/" + d.String(),
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

func (rc *regClient) BlobMount(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error {
	if refSrc.Registry != refTgt.Registry {
		return fmt.Errorf("Registry must match for blob mount")
	}

	// build/send request
	query := url.Values{}
	query.Set("mount", d.String())
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

func (rc *regClient) BlobPut(ctx context.Context, ref types.Ref, d digest.Digest, rdr io.Reader, ct string, cl int64) (digest.Digest, error) {
	// defaults for content-type and length
	if ct == "" {
		ct = "application/octet-stream"
	}
	if cl == 0 {
		cl = -1
	}

	// get the upload URL
	putURL, err := rc.blobGetUploadURL(ctx, ref)
	if err != nil {
		return "", err
	}

	// send upload as one-chunk
	if d != "" {
		err = rc.blobPutUploadFull(ctx, ref, d, putURL, rdr, ct, cl)
		if err == nil {
			return digest.Digest(d), nil
		}
	}

	// send a chunked upload if full upload not possible or failed
	return rc.blobPutUploadChunked(ctx, ref, putURL, rdr, ct)
}

func (rc *regClient) blobGetUploadURL(ctx context.Context, ref types.Ref) (*url.URL, error) {
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
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	// Extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme.
	// This doesn't use the httpDo method since location could point to any url, negating the API expansion, mirror handling, and similar features.
	location := resp.HTTPResponse().Header.Get("Location")
	rc.log.WithFields(logrus.Fields{
		"location": location,
	}).Debug("Upload location received")
	// put url may be relative to the above post URL, so parse in that context
	postURL := resp.HTTPResponse().Request.URL
	putURL, err := postURL.Parse(location)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"location": location,
			"err":      err,
		}).Warn("Location url failed to parse")
		return nil, fmt.Errorf("Blob upload url invalid, ref %s: %w", ref.CommonName(), err)
	}
	return putURL, nil
}

func (rc *regClient) blobPutUploadFull(ctx context.Context, ref types.Ref, d digest.Digest, putURL *url.URL, rdr io.Reader, ct string, cl int64) error {
	host := rc.hostGet(ref.Registry)

	// append digest to request to use the monolithic upload option
	if putURL.RawQuery != "" {
		putURL.RawQuery = putURL.RawQuery + "&digest=" + d.String()
	} else {
		putURL.RawQuery = "digest=" + d.String()
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
	resp, err := rty.DoRequest(ctx, "PUT", []url.URL{*putURL}, opts...)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

func (rc *regClient) blobPutUploadChunked(ctx context.Context, ref types.Ref, putURL *url.URL, rdr io.Reader, ct string) (digest.Digest, error) {
	host := rc.hostGet(ref.Registry)
	bufSize := int64(512 * 1024) // 512k

	// setup buffer and digest pipe
	// read manifest and compute digest
	digester := digest.Canonical.Digester()
	digestRdr := io.TeeReader(rdr, digester.Hash())
	chunkBuf := new(bytes.Buffer)
	chunkBuf.Grow(int(bufSize))
	finalChunk := false
	chunkStart := int64(0)
	bodyFunc := func() (io.ReadCloser, error) {
		return ioutil.NopCloser(chunkBuf), nil
	}

	for !finalChunk {
		chunkURL := *putURL
		// read a chunk into an input buffer, computing the digest
		chunkSize, err := io.CopyN(chunkBuf, digestRdr, bufSize)
		if err == io.EOF {
			finalChunk = true
		} else if err != nil {
			return "", fmt.Errorf("Failed to send blob chunk, ref %s: %w", ref.CommonName(), err)
		}

		if int64(chunkBuf.Len()) != chunkSize {
			rc.log.WithFields(logrus.Fields{
				"buf-size":   chunkBuf.Len(),
				"chunk-size": chunkSize,
			}).Debug("Buffer/chunk size mismatch")
		}
		if chunkSize > 0 {
			// write chunk
			opts := []retryable.OptsReq{}
			opts = append(opts, retryable.WithBodyFunc(bodyFunc))
			// opts = append(opts, retryable.WithContentLen(chunkSize))
			opts = append(opts, retryable.WithHeader("Content-Type", []string{ct}))
			opts = append(opts, retryable.WithHeader("Content-Length", []string{fmt.Sprintf("%d", chunkSize)}))
			opts = append(opts, retryable.WithHeader("Content-Range", []string{fmt.Sprintf("%d-%d", chunkStart, chunkStart+chunkSize)}))

			rty := rc.getRetryable(host)
			resp, err := rty.DoRequest(ctx, "PATCH", []url.URL{chunkURL}, opts...)
			if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
				return "", fmt.Errorf("Failed to send blob (chunk), ref %s: %w", ref.CommonName(), err)
			}
			resp.Close()
			if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
				return "", fmt.Errorf("Failed to send blob (chunk), ref %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
			}
			chunkStart += chunkSize
			if chunkBuf.Len() != 0 {
				rc.log.WithFields(logrus.Fields{
					"buf-size":   chunkBuf.Len(),
					"chunk-size": chunkSize,
				}).Debug("Buffer was not read")
			}
			location := resp.HTTPResponse().Header.Get("Location")
			rc.log.WithFields(logrus.Fields{
				"location": location,
			}).Debug("Next chunk upload location received")
			prevURL := resp.HTTPResponse().Request.URL
			putURL, err = prevURL.Parse(location)
			if err != nil {
				return "", fmt.Errorf("Failed to send blob (parse next chunk location), ref %s: %w", ref.CommonName(), err)
			}
		}
	}

	// write digest to complete request
	d := digester.Digest()
	// append digest to request to use the monolithic upload option
	if putURL.RawQuery != "" {
		putURL.RawQuery = putURL.RawQuery + "&digest=" + d.String()
	} else {
		putURL.RawQuery = "digest=" + d.String()
	}

	// send the blob
	opts := []retryable.OptsReq{}
	// opts = append(opts, retryable.WithContentLen(0))
	opts = append(opts, retryable.WithHeader("Content-Length", []string{"0"}))
	opts = append(opts, retryable.WithHeader("Content-Range", []string{fmt.Sprintf("%d-%d", chunkStart, chunkStart)}))
	opts = append(opts, retryable.WithHeader("Content-Type", []string{ct}))
	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "PUT", []url.URL{*putURL}, opts...)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return "", fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return "", fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return d, nil
}

func (b blobCommon) GetOrig() interface{} {
	return b.orig
}

func (b blobCommon) MediaType() string {
	return b.mt
}

func (b blobCommon) RawHeaders() (http.Header, error) {
	return b.rawHeader, nil
}

func (b blobCommon) Response() *http.Response {
	return b.resp
}

// RawBody returns the original body from the request
func (b blobReader) RawBody() ([]byte, error) {
	return ioutil.ReadAll(b)
}

func (b blobReader) toOCIConfig() (BlobOCIConfig, error) {
	blobBody, err := ioutil.ReadAll(b)
	if err != nil {
		return blobOCIConfig{}, fmt.Errorf("Error reading image config for %s: %w", b.ref.CommonName(), err)
	}
	var ociImage ociv1.Image
	err = json.Unmarshal(blobBody, &ociImage)
	if err != nil {
		return blobOCIConfig{}, fmt.Errorf("Error parsing image config for %s: %w", b.ref.CommonName(), err)
	}
	b.orig = &ociImage
	return blobOCIConfig{blobCommon: b.blobCommon, rawBody: blobBody, Image: ociImage}, nil
}

// GetConfig returns the original body from the request
func (b blobOCIConfig) GetConfig() ociv1.Image {
	return b.Image
}

// RawBody returns the original body from the request
func (b blobOCIConfig) RawBody() ([]byte, error) {
	return b.rawBody, nil
}
