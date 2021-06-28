package regclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/regclient/regclient/regclient/blob"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

// BlobClient provides registry client requests to Blobs
type BlobClient interface {
	BlobCopy(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error
	BlobGet(ctx context.Context, ref types.Ref, d digest.Digest, accepts []string) (blob.Reader, error)
	BlobGetOCIConfig(ctx context.Context, ref types.Ref, d digest.Digest) (blob.OCIConfig, error)
	BlobHead(ctx context.Context, ref types.Ref, d digest.Digest) (blob.Reader, error)
	BlobMount(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error
	BlobPut(ctx context.Context, ref types.Ref, d digest.Digest, rdr io.Reader, ct string, cl int64) (digest.Digest, int64, error)
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
	if _, err := rc.BlobHead(ctx, refTgt, d); err == nil {
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
	if _, _, err := rc.BlobPut(ctx, refTgt, d, blobIO, blobIO.MediaType(), blobIO.Response().ContentLength); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to push blob")
		return err
	}
	return nil
}

func (rc *regClient) BlobGet(ctx context.Context, ref types.Ref, d digest.Digest, accepts []string) (blob.Reader, error) {
	// build/send request
	headers := http.Header{}
	if len(accepts) > 0 {
		headers["Accept"] = accepts
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:     "GET",
				repository: ref.Repository,
				path:       "blobs/" + d.String(),
				headers:    headers,
				digest:     d,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	b := blob.NewReader(resp)
	b.SetMeta(ref, d, 0)
	b.SetResp(resp.HTTPResponse())
	return b, nil
}

func (rc *regClient) BlobGetOCIConfig(ctx context.Context, ref types.Ref, d digest.Digest) (blob.OCIConfig, error) {
	b, err := rc.BlobGet(ctx, ref, d, []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return nil, err
	}
	return b.ToOCIConfig()
}

// BlobHead is used to verify if a blob exists and is accessible
func (rc *regClient) BlobHead(ctx context.Context, ref types.Ref, d digest.Digest) (blob.Reader, error) {
	// build/send request
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:     "HEAD",
				repository: ref.Repository,
				path:       "blobs/" + d.String(),
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	b := blob.NewReader(nil)
	b.SetMeta(ref, d, 0)
	b.SetResp(resp.HTTPResponse())
	return b, nil
}

func (rc *regClient) BlobMount(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error {
	_, err := rc.blobMount(ctx, refTgt, d, refSrc)
	return err
}

func (rc *regClient) BlobPut(ctx context.Context, ref types.Ref, d digest.Digest, rdr io.Reader, ct string, cl int64) (digest.Digest, int64, error) {
	var putURL *url.URL
	var err error
	// defaults for content-type and length
	if ct == "" {
		ct = "application/octet-stream"
	}
	if cl == 0 {
		cl = -1
	}

	// attempt an anonymous blob mount
	if d != "" && cl > 0 {
		putURL, err = rc.blobMount(ctx, ref, d, types.Ref{})
		if err == nil {
			return digest.Digest(d), cl, nil
		}
		if err != ErrMountReturnedLocation {
			putURL = nil
		}
	}
	// fallback to requesting upload URL
	if putURL == nil {
		putURL, err = rc.blobGetUploadURL(ctx, ref)
		if err != nil {
			return "", 0, err
		}
	}

	// send upload as one-chunk
	if d != "" && cl > 0 {
		err = rc.blobPutUploadFull(ctx, ref, d, putURL, rdr, ct, cl)
		return digest.Digest(d), cl, err
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
				method:     "POST",
				repository: ref.Repository,
				path:       "blobs/uploads/",
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
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

func (rc *regClient) blobMount(ctx context.Context, refTgt types.Ref, d digest.Digest, refSrc types.Ref) (*url.URL, error) {
	// build/send request
	query := url.Values{}
	query.Set("mount", d.String())
	if refSrc.Registry == refTgt.Registry && refSrc.Repository != "" {
		query.Set("from", refSrc.Repository)
	}

	req := httpReq{
		host:      refTgt.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method:     "POST",
				repository: refTgt.Repository,
				path:       "blobs/uploads/",
				query:      query,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, refTgt.CommonName(), err)
	}
	defer resp.Close()
	// 201 indicates the blob mount succeeded
	if resp.HTTPResponse().StatusCode == 201 {
		return nil, nil
	}
	// 202 indicates blob mount failed but server ready to receive an upload at location
	location := resp.HTTPResponse().Header.Get("Location")
	if resp.HTTPResponse().StatusCode == 202 && location != "" {
		postURL := resp.HTTPResponse().Request.URL
		putURL, err := postURL.Parse(location)
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"digest":   d,
				"target":   refTgt.CommonName(),
				"location": location,
				"err":      err,
			}).Warn("Mount location header failed to parse")
		} else {
			return putURL, ErrMountReturnedLocation
		}
	}
	// all other responses unhandled
	return nil, fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, refTgt.CommonName(), httpError(resp.HTTPResponse().StatusCode))
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
	opts = append(opts, retryable.WithHeader("Content-Length", []string{fmt.Sprintf("%d", cl)}))
	opts = append(opts, retryable.WithScope(ref.Repository, true))
	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "PUT", []url.URL{*putURL}, opts...)
	if err != nil {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

func (rc *regClient) blobPutUploadChunked(ctx context.Context, ref types.Ref, putURL *url.URL, rdr io.Reader, ct string) (digest.Digest, int64, error) {
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
	chunkURL := *putURL

	for !finalChunk {
		// read a chunk into an input buffer, computing the digest
		chunkSize, err := io.CopyN(chunkBuf, digestRdr, bufSize)
		if err == io.EOF {
			finalChunk = true
		} else if err != nil {
			return "", 0, fmt.Errorf("Failed to send blob chunk, ref %s: %w", ref.CommonName(), err)
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
			opts = append(opts, retryable.WithContentLen(chunkSize))
			opts = append(opts, retryable.WithHeader("Content-Type", []string{ct}))
			opts = append(opts, retryable.WithHeader("Content-Length", []string{fmt.Sprintf("%d", chunkSize)}))
			opts = append(opts, retryable.WithHeader("Content-Range", []string{fmt.Sprintf("%d-%d", chunkStart, chunkStart+chunkSize)}))
			opts = append(opts, retryable.WithScope(ref.Repository, true))

			rty := rc.getRetryable(host)
			resp, err := rty.DoRequest(ctx, "PATCH", []url.URL{chunkURL}, opts...)
			if err != nil {
				return "", 0, fmt.Errorf("Failed to send blob (chunk), ref %s: %w", ref.CommonName(), err)
			}
			resp.Close()
			if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
				return "", 0, fmt.Errorf("Failed to send blob (chunk), ref %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
			}
			chunkStart += chunkSize
			if chunkBuf.Len() != 0 {
				rc.log.WithFields(logrus.Fields{
					"buf-size":   chunkBuf.Len(),
					"chunk-size": chunkSize,
				}).Debug("Buffer was not read")
			}
			location := resp.HTTPResponse().Header.Get("Location")
			if location != "" {
				rc.log.WithFields(logrus.Fields{
					"location": location,
				}).Debug("Next chunk upload location received")
				prevURL := resp.HTTPResponse().Request.URL
				parseURL, err := prevURL.Parse(location)
				if err != nil {
					return "", 0, fmt.Errorf("Failed to send blob (parse next chunk location), ref %s: %w", ref.CommonName(), err)
				}
				chunkURL = *parseURL
			}
		}
	}

	// write digest to complete request
	d := digester.Digest()
	// append digest to request to use the monolithic upload option
	if chunkURL.RawQuery != "" {
		chunkURL.RawQuery = chunkURL.RawQuery + "&digest=" + d.String()
	} else {
		chunkURL.RawQuery = "digest=" + d.String()
	}

	// send the blob
	opts := []retryable.OptsReq{}
	// opts = append(opts, retryable.WithContentLen(0))
	opts = append(opts, retryable.WithHeader("Content-Length", []string{"0"}))
	opts = append(opts, retryable.WithHeader("Content-Range", []string{fmt.Sprintf("%d-%d", chunkStart, chunkStart)}))
	opts = append(opts, retryable.WithHeader("Content-Type", []string{ct}))
	opts = append(opts, retryable.WithScope(ref.Repository, true))
	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "PUT", []url.URL{chunkURL}, opts...)
	if err != nil {
		return "", 0, fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode < 200 || resp.HTTPResponse().StatusCode > 299 {
		return "", 0, fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return d, chunkStart, nil
}
