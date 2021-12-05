package regclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/retryable"
	"github.com/regclient/regclient/regclient/blob"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

type ociBlobAPI interface {
	BlobDelete(ctx context.Context, ref types.Ref, d digest.Digest) error
	BlobGet(ctx context.Context, ref types.Ref, d digest.Digest) (blob.Reader, error)
	BlobHead(ctx context.Context, ref types.Ref, d digest.Digest) (blob.Reader, error)
	BlobMount(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error
	BlobPut(ctx context.Context, ref types.Ref, d digest.Digest, rdr io.Reader, cl int64) (digest.Digest, int64, error)
}

func (rc *Client) BlobCopy(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error {
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
	blobIO, err := rc.BlobGet(ctx, refSrc, d)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"src":    refSrc.Reference,
			"digest": d,
		}).Warn("Failed to retrieve blob")
		return err
	}
	defer blobIO.Close()
	if _, _, err := rc.BlobPut(ctx, refTgt, d, blobIO, blobIO.Response().ContentLength); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to push blob")
		return err
	}
	return nil
}

// blobGetRetryable tracks read bytes, and resends a blobGetRange on errors to get a new pass through reader
type blobGetRetryable struct {
	rc         *Client
	r          io.ReadCloser
	offset     int64
	start, end int64
	offsetErr  int64
	ctx        context.Context
	ref        types.Ref
	d          digest.Digest
}

func (bgr *blobGetRetryable) newReader() error {
	// request new reader for range
	r, err := bgr.rc.blobGetRange(bgr.ctx, bgr.ref, bgr.d, bgr.start+bgr.offset, bgr.end)
	if err != nil {
		return err
	}
	// close previously erroring reader, ignoring errors
	_ = bgr.r.Close()
	// save new reader
	bgr.r = r
	return nil
}

func (bgr *blobGetRetryable) setEnd() error {
	if bgr.end == 0 {
		br, err := bgr.rc.BlobHead(bgr.ctx, bgr.ref, bgr.d)
		if err != nil {
			return err
		}
		if br.Response().Header["Accept-Ranges"] == nil {
			return fmt.Errorf("Registry does not support range requests")
		}
		bgr.end, err = strconv.ParseInt(br.Response().Header.Get("Content-Length"), 10, 64)
		if bgr.end == 0 {
			return fmt.Errorf("Registry reported empty blob")
		}
	}
	return nil
}

func (bgr *blobGetRetryable) Close() error {
	return bgr.r.Close()
}

func (bgr *blobGetRetryable) Read(p []byte) (int, error) {
	i, err := bgr.r.Read(p)
	if err != nil && err != io.EOF {
		// on repeat failures, give up and error
		if bgr.offsetErr == bgr.offset {
			return i, err
		}
		bgr.offsetErr = bgr.offset
		// recreate reader with a range request
		err = bgr.setEnd()
		if err != nil {
			return 0, err
		}
		err = bgr.newReader()
		if err != nil {
			return 0, err
		}
		i, err = bgr.r.Read(p)
	}
	bgr.offset += int64(i)
	return i, err
}

func (bgr *blobGetRetryable) Seek(offset int64, whence int) (int64, error) {
	// noop to return current offset
	if offset == 0 && whence == io.SeekCurrent {
		return bgr.offset, nil
	}

	// set end sends a head request to verify registry support for range requests
	if offset != 0 || whence != io.SeekStart {
		err := bgr.setEnd()
		if err != nil {
			return 0, err
		}
	}

	// update offset based on whence and provided offset
	switch whence {
	case io.SeekCurrent:
		bgr.offset += offset
	case io.SeekStart:
		bgr.offset = offset
	case io.SeekEnd:
		bgr.offset = bgr.end + offset
	}
	if bgr.offset < 0 {
		return bgr.offset, fmt.Errorf("Seek before start not allowed")
	}

	// submit a new request
	err := bgr.newReader()
	if err != nil {
		return 0, err
	}

	return bgr.offset, nil
}

func (rc *Client) BlobDelete(ctx context.Context, ref types.Ref, d digest.Digest) error {
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:     "DELETE",
				repository: ref.Repository,
				path:       "blobs/" + d.String(),
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete blob, digest %s, ref %s: %w", d, ref.CommonName(), err)
	}
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("failed to delete blob, digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

// TODO: remove accepts argument
func (rc *Client) BlobGet(ctx context.Context, ref types.Ref, d digest.Digest) (blob.Reader, error) {
	// build/send request
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:     "GET",
				repository: ref.Repository,
				path:       "blobs/" + d.String(),
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

	bgr := blobGetRetryable{
		r:   resp,
		rc:  rc,
		ctx: ctx,
		ref: ref,
		d:   d,
	}
	b := blob.NewReader(&bgr)
	b.SetMeta(ref, d, 0)
	b.SetResp(resp.HTTPResponse())
	return b, nil
}

// TODO: consider adding a BlobGetRange that returns a blob.Reader

func (rc *Client) blobGetRange(ctx context.Context, ref types.Ref, d digest.Digest, start, end int64) (io.ReadCloser, error) {
	// check for valid range
	if start < 0 || end < 0 || start > end {
		return nil, fmt.Errorf("Invalid range, start %d, end %d", start, end)
	}
	// build/send request
	headers := http.Header{}
	// if start and end are both 0, skip the range, return the full blob
	if start > 0 || (start == 0 && end > 0) {
		headers["Range"] = []string{fmt.Sprintf("bytes=%d-%d", start, end)}
	}
	req := httpReq{
		host: ref.Registry,
		apis: map[string]httpReqAPI{
			"": {
				method:     "GET",
				repository: ref.Repository,
				path:       "blobs/" + d.String(),
				headers:    headers,
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

	return resp, nil
}

func (rc *Client) BlobGetOCIConfig(ctx context.Context, ref types.Ref, d digest.Digest) (blob.OCIConfig, error) {
	b, err := rc.BlobGet(ctx, ref, d)
	if err != nil {
		return nil, err
	}
	return b.ToOCIConfig()
}

// BlobHead is used to verify if a blob exists and is accessible
func (rc *Client) BlobHead(ctx context.Context, ref types.Ref, d digest.Digest) (blob.Reader, error) {
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

func (rc *Client) BlobMount(ctx context.Context, refSrc types.Ref, refTgt types.Ref, d digest.Digest) error {
	_, uuid, err := rc.blobMount(ctx, refTgt, d, refSrc)
	// if mount fails and returns an upload location, cancel that upload
	if err != nil {
		rc.blobUploadCancel(ctx, refTgt, uuid)
	}
	return err
}

// TODO: remove content-type arg
func (rc *Client) BlobPut(ctx context.Context, ref types.Ref, d digest.Digest, rdr io.Reader, cl int64) (digest.Digest, int64, error) {
	var putURL *url.URL
	var err error
	// defaults for content-type and length
	if cl == 0 {
		cl = -1
	}

	// attempt an anonymous blob mount
	if d != "" && cl > 0 {
		putURL, _, err = rc.blobMount(ctx, ref, d, types.Ref{})
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
	tryPut := bool(d != "" && cl > 0)
	if tryPut {
		host := rc.hostGet(ref.Registry)
		maxPut := host.BlobMax
		if maxPut == 0 {
			maxPut = rc.blobMaxPut
		}
		if maxPut > 0 && cl > maxPut {
			tryPut = false
		}
	}
	if tryPut {
		err = rc.blobPutUploadFull(ctx, ref, d, putURL, rdr, cl)
		if err == nil {
			return digest.Digest(d), cl, nil
		}
		// on failure, attempt to seek back to start to perform a chunked upload
		rdrSeek, ok := rdr.(io.ReadSeeker)
		if !ok {
			return digest.Digest(d), cl, err
		}
		offset, errR := rdrSeek.Seek(0, io.SeekStart)
		if errR != nil || offset != 0 {
			return digest.Digest(d), cl, err
		}
	}

	// send a chunked upload if full upload not possible or too large
	return rc.blobPutUploadChunked(ctx, ref, putURL, rdr)
}

func (rc *Client) blobGetUploadURL(ctx context.Context, ref types.Ref) (*url.URL, error) {
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
	if resp.HTTPResponse().StatusCode != 202 {
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	// Extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme.
	location := resp.HTTPResponse().Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", ref.CommonName(), ErrMissingLocation)
	}
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

func (rc *Client) blobMount(ctx context.Context, refTgt types.Ref, d digest.Digest, refSrc types.Ref) (*url.URL, string, error) {
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
		return nil, "", fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, refTgt.CommonName(), err)
	}
	defer resp.Close()
	// 201 indicates the blob mount succeeded
	if resp.HTTPResponse().StatusCode == 201 {
		return nil, "", nil
	}
	// 202 indicates blob mount failed but server ready to receive an upload at location
	location := resp.HTTPResponse().Header.Get("Location")
	uuid := resp.HTTPResponse().Header.Get("Docker-Upload-UUID")
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
			return putURL, uuid, ErrMountReturnedLocation
		}
	}
	// all other responses unhandled
	return nil, "", fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, refTgt.CommonName(), httpError(resp.HTTPResponse().StatusCode))
}

func (rc *Client) blobPutUploadFull(ctx context.Context, ref types.Ref, d digest.Digest, putURL *url.URL, rdr io.Reader, cl int64) error {
	host := rc.hostGet(ref.Registry)
	ct := "application/octet-stream"

	// append digest to request to use the monolithic upload option
	if putURL.RawQuery != "" {
		putURL.RawQuery = putURL.RawQuery + "&digest=" + url.QueryEscape(d.String())
	} else {
		putURL.RawQuery = "digest=" + url.QueryEscape(d.String())
	}

	// send the blob
	opts := []retryable.OptsReq{}
	readOnce := false
	bodyFunc := func() (io.ReadCloser, error) {
		// if reader is reused,
		if readOnce {
			rdrSeek, ok := rdr.(io.ReadSeeker)
			if !ok {
				return nil, fmt.Errorf("Unable to reuse reader")
			}
			_, err := rdrSeek.Seek(0, io.SeekStart)
			if err != nil {
				return nil, fmt.Errorf("Unable to reuse reader")
			}
		}
		readOnce = true
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
	// 201 follows distribution-spec, 204 is listed as possible in the Docker registry spec
	if resp.HTTPResponse().StatusCode != 201 && resp.HTTPResponse().StatusCode != 204 {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

func (rc *Client) blobPutUploadChunked(ctx context.Context, ref types.Ref, putURL *url.URL, rdr io.Reader) (digest.Digest, int64, error) {
	host := rc.hostGet(ref.Registry)
	bufSize := host.BlobChunk
	if bufSize <= 0 {
		bufSize = rc.blobChunkSize
	}
	bufBytes := make([]byte, bufSize)
	bufRdr := bytes.NewReader(bufBytes)
	lenChange := false
	ct := "application/octet-stream"

	// setup buffer and digest pipe
	digester := digest.Canonical.Digester()
	digestRdr := io.TeeReader(rdr, digester.Hash())
	finalChunk := false
	chunkStart := int64(0)
	bodyFunc := func() (io.ReadCloser, error) {
		// reset to the start on every new read
		_, err := bufRdr.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		return ioutil.NopCloser(bufRdr), nil
	}
	chunkURL := *putURL

	for !finalChunk {
		lenChange = false
		// reset length if previous read was short
		if cap(bufBytes) != len(bufBytes) {
			bufBytes = bufBytes[:cap(bufBytes)]
			lenChange = true
		}
		// read a chunk into an input buffer, computing the digest
		chunkSize, err := io.ReadFull(digestRdr, bufBytes)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			finalChunk = true
		} else if err != nil {
			return "", 0, fmt.Errorf("Failed to send blob chunk, ref %s: %w", ref.CommonName(), err)
		}
		// update length on partial read
		if chunkSize != len(bufBytes) {
			bufBytes = bufBytes[:chunkSize]
			lenChange = true
		}
		if lenChange {
			// need to recreate the reader on a change to the slice length,
			// old reader is looking at the old slice metadata
			bufRdr = bytes.NewReader(bufBytes)
		}

		if chunkSize > 0 {
			// write chunk
			opts := []retryable.OptsReq{}
			opts = append(opts, retryable.WithBodyFunc(bodyFunc))
			opts = append(opts, retryable.WithContentLen(int64(chunkSize)))
			opts = append(opts, retryable.WithHeader("Content-Type", []string{ct}))
			opts = append(opts, retryable.WithHeader("Content-Length", []string{fmt.Sprintf("%d", chunkSize)}))
			opts = append(opts, retryable.WithHeader("Content-Range", []string{fmt.Sprintf("%d-%d", chunkStart, chunkStart+int64(chunkSize))}))
			opts = append(opts, retryable.WithScope(ref.Repository, true))

			rty := rc.getRetryable(host)
			resp, err := rty.DoRequest(ctx, "PATCH", []url.URL{chunkURL}, opts...)
			if err != nil {
				return "", 0, fmt.Errorf("Failed to send blob (chunk), ref %s: %w", ref.CommonName(), err)
			}
			resp.Close()

			// distribution-spec is 202, AWS ECR returns a 201 and rejects the put
			if resp.HTTPResponse().StatusCode == 201 {
				rc.log.WithFields(logrus.Fields{
					"ref":        ref.CommonName(),
					"chunkStart": chunkStart,
					"chunkSize":  chunkSize,
				}).Debug("Early accept of chunk in PATCH before PUT request")
			} else if resp.HTTPResponse().StatusCode != 202 {
				return "", 0, fmt.Errorf("Failed to send blob (chunk), ref %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
			}
			chunkStart += int64(chunkSize)
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

	// compute digest
	d := digester.Digest()

	// send the final put
	// append digest to request to use the monolithic upload option
	if chunkURL.RawQuery != "" {
		chunkURL.RawQuery = chunkURL.RawQuery + "&digest=" + url.QueryEscape(d.String())
	} else {
		chunkURL.RawQuery = "digest=" + url.QueryEscape(d.String())
	}

	opts := []retryable.OptsReq{}
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
	// 201 follows distribution-spec, 204 is listed as possible in the Docker registry spec
	if resp.HTTPResponse().StatusCode != 201 && resp.HTTPResponse().StatusCode != 204 {
		return "", 0, fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}

	return d, chunkStart, nil
}

// TODO: just take a putURL rather than the uuid and call a delete on that url
func (rc *Client) blobUploadCancel(ctx context.Context, ref types.Ref, uuid string) error {
	if uuid == "" {
		return fmt.Errorf("Failed to cancel upload %s: uuid undefined", ref.CommonName())
	}
	req := httpReq{
		host:      ref.Registry,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method:     "DELETE",
				repository: ref.Repository,
				path:       "blobs/uploads/" + uuid,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil {
		return fmt.Errorf("Failed to cancel upload %s: %w", ref.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("Failed to cancel upload %s: %w", ref.CommonName(), httpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

// blobUploadStatus provides a response with headers indicating the progress of an upload
func (rc *Client) blobUploadStatus(ctx context.Context, ref types.Ref, putURL *url.URL) (*http.Response, error) {
	host := rc.hostGet(ref.Registry)
	rty := rc.getRetryable(host)
	opts := []retryable.OptsReq{}
	opts = append(opts, retryable.WithScope(ref.Repository, true))
	resp, err := rty.DoRequest(ctx, "GET", []url.URL{*putURL}, opts...)
	if err != nil {
		return nil, fmt.Errorf("Failed to get upload status: %v", err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 204 {
		return resp.HTTPResponse(), fmt.Errorf("Failed to get upload status: %v", httpError(resp.HTTPResponse().StatusCode))
	}
	return resp.HTTPResponse(), nil
}

func blobUploadCurBytes(resp *http.Response) (int64, error) {
	if resp == nil {
		return 0, fmt.Errorf("Missing response")
	}
	r := resp.Header.Get("Range")
	if r == "" {
		return 0, fmt.Errorf("Missing range header")
	}
	rSplit := strings.SplitN(r, "-", 2)
	if len(rSplit) < 2 {
		return 0, fmt.Errorf("Missing offset in range header")
	}
	return strconv.ParseInt(rSplit[2], 10, 64)
}
