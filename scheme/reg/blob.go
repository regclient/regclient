package reg

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
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

// BlobDelete removes a blob from the repository
func (reg *Reg) BlobDelete(ctx context.Context, r ref.Ref, d digest.Digest) error {
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "DELETE",
				Repository: r.Repository,
				Path:       "blobs/" + d.String(),
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete blob, digest %s, ref %s: %w", d, r.CommonName(), err)
	}
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("failed to delete blob, digest %s, ref %s: %w", d, r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

// BlobGet retrieves a blob from the repository, returning a blob reader
func (reg *Reg) BlobGet(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error) {
	// build/send request
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "GET",
				Repository: r.Repository,
				Path:       "blobs/" + d.String(),
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, r.CommonName(), err)
	}
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to get blob, digest %s, ref %s: %w", d, r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	b := blob.NewReader(
		blob.WithRef(r),
		blob.WithReadCloser(resp),
		blob.WithDesc(ociv1.Descriptor{
			Digest: d,
		}),
		blob.WithResp(resp.HTTPResponse()),
	)
	return b, nil
}

// BlobHead is used to verify if a blob exists and is accessible
func (reg *Reg) BlobHead(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error) {
	// build/send request
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "HEAD",
				Repository: r.Repository,
				Path:       "blobs/" + d.String(),
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to request blob head, digest %s, ref %s: %w", d, r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	b := blob.NewReader(
		blob.WithRef(r),
		blob.WithDesc(ociv1.Descriptor{
			Digest: d,
		}),
		blob.WithResp(resp.HTTPResponse()),
	)
	return b, nil
}

// BlobMount attempts to perform a server side copy/mount of the blob between repositories
func (reg *Reg) BlobMount(ctx context.Context, rSrc ref.Ref, rTgt ref.Ref, d digest.Digest) error {
	_, uuid, err := reg.blobMount(ctx, rTgt, d, rSrc)
	// if mount fails and returns an upload location, cancel that upload
	if err != nil {
		reg.blobUploadCancel(ctx, rTgt, uuid)
	}
	return err
}

// BlobPut uploads a blob to a repository.
// This will attempt an anonymous blob mount first which some registries may support.
// It will then try doing a full put of the blob without chunking (most widely supported).
// If the full put fails, it will fall back to a chunked upload (useful for flaky networks).
func (reg *Reg) BlobPut(ctx context.Context, r ref.Ref, d digest.Digest, rdr io.Reader, cl int64) (digest.Digest, int64, error) {
	var putURL *url.URL
	var err error
	// defaults for content-type and length
	if cl == 0 {
		cl = -1
	}

	// attempt an anonymous blob mount
	if d != "" && cl > 0 {
		putURL, _, err = reg.blobMount(ctx, r, d, ref.Ref{})
		if err == nil {
			return digest.Digest(d), cl, nil
		}
		if err != types.ErrMountReturnedLocation {
			putURL = nil
		}
	}
	// fallback to requesting upload URL
	if putURL == nil {
		putURL, err = reg.blobGetUploadURL(ctx, r)
		if err != nil {
			return "", 0, err
		}
	}

	// send upload as one-chunk
	tryPut := bool(d != "" && cl > 0)
	if tryPut {
		host := reg.hostGet(r.Registry)
		maxPut := host.BlobMax
		if maxPut == 0 {
			maxPut = reg.blobMaxPut
		}
		if maxPut > 0 && cl > maxPut {
			tryPut = false
		}
	}
	if tryPut {
		err = reg.blobPutUploadFull(ctx, r, d, putURL, rdr, cl)
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
	return reg.blobPutUploadChunked(ctx, r, putURL, rdr)
}

func (reg *Reg) blobGetUploadURL(ctx context.Context, r ref.Ref) (*url.URL, error) {
	// request an upload location
	req := &reghttp.Req{
		Host:      r.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "POST",
				Repository: r.Repository,
				Path:       "blobs/uploads/",
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 202 {
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	// Extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme.
	location := resp.HTTPResponse().Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("Failed to send blob post, ref %s: %w", r.CommonName(), types.ErrMissingLocation)
	}
	reg.log.WithFields(logrus.Fields{
		"location": location,
	}).Debug("Upload location received")
	// put url may be relative to the above post URL, so parse in that context
	postURL := resp.HTTPResponse().Request.URL
	putURL, err := postURL.Parse(location)
	if err != nil {
		reg.log.WithFields(logrus.Fields{
			"location": location,
			"err":      err,
		}).Warn("Location url failed to parse")
		return nil, fmt.Errorf("Blob upload url invalid, ref %s: %w", r.CommonName(), err)
	}
	return putURL, nil
}

func (reg *Reg) blobMount(ctx context.Context, rTgt ref.Ref, d digest.Digest, rSrc ref.Ref) (*url.URL, string, error) {
	// build/send request
	query := url.Values{}
	query.Set("mount", d.String())
	if rSrc.Registry == rTgt.Registry && rSrc.Repository != "" {
		query.Set("from", rSrc.Repository)
	}

	req := &reghttp.Req{
		Host:      rTgt.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "POST",
				Repository: rTgt.Repository,
				Path:       "blobs/uploads/",
				Query:      query,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, rTgt.CommonName(), err)
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
			reg.log.WithFields(logrus.Fields{
				"digest":   d,
				"target":   rTgt.CommonName(),
				"location": location,
				"err":      err,
			}).Warn("Mount location header failed to parse")
		} else {
			return putURL, uuid, types.ErrMountReturnedLocation
		}
	}
	// all other responses unhandled
	return nil, "", fmt.Errorf("Failed to mount blob, digest %s, ref %s: %w", d, rTgt.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
}

func (reg *Reg) blobPutUploadFull(ctx context.Context, r ref.Ref, d digest.Digest, putURL *url.URL, rdr io.Reader, cl int64) error {
	// append digest to request to use the monolithic upload option
	if putURL.RawQuery != "" {
		putURL.RawQuery = putURL.RawQuery + "&digest=" + url.QueryEscape(d.String())
	} else {
		putURL.RawQuery = "digest=" + url.QueryEscape(d.String())
	}

	// make a reader function for the blob
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

	// build/send request
	header := http.Header{
		"Content-Type": {"application/octet-stream"},
	}
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "PUT",
				Repository: r.Repository,
				DirectURL:  putURL,
				BodyFunc:   bodyFunc,
				BodyLen:    cl,
				Headers:    header,
			},
		},
		NoMirrors: true,
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, r.CommonName(), err)
	}
	defer resp.Close()
	// 201 follows distribution-spec, 204 is listed as possible in the Docker registry spec
	if resp.HTTPResponse().StatusCode != 201 && resp.HTTPResponse().StatusCode != 204 {
		return fmt.Errorf("Failed to send blob (put), digest %s, ref %s: %w", d, r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

func (reg *Reg) blobPutUploadChunked(ctx context.Context, r ref.Ref, putURL *url.URL, rdr io.Reader) (digest.Digest, int64, error) {
	host := reg.hostGet(r.Registry)
	bufSize := host.BlobChunk
	if bufSize <= 0 {
		bufSize = reg.blobChunkSize
	}
	bufBytes := make([]byte, bufSize)
	bufRdr := bytes.NewReader(bufBytes)
	lenChange := false

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
			return "", 0, fmt.Errorf("Failed to send blob chunk, ref %s: %w", r.CommonName(), err)
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
			header := http.Header{
				"Content-Type":  {"application/octet-stream"},
				"Content-Range": {fmt.Sprintf("%d-%d", chunkStart, chunkStart+int64(chunkSize))},
			}
			req := &reghttp.Req{
				Host: r.Registry,
				APIs: map[string]reghttp.ReqAPI{
					"": {
						Method:     "PATCH",
						Repository: r.Repository,
						DirectURL:  &chunkURL,
						BodyFunc:   bodyFunc,
						BodyLen:    int64(chunkSize),
						Headers:    header,
					},
				},
				NoMirrors: true,
			}
			resp, err := reg.reghttp.Do(ctx, req)
			if err != nil {
				return "", 0, fmt.Errorf("Failed to send blob (chunk), ref %s: %w", r.CommonName(), err)
			}
			resp.Close()

			// distribution-spec is 202, AWS ECR returns a 201 and rejects the put
			if resp.HTTPResponse().StatusCode == 201 {
				reg.log.WithFields(logrus.Fields{
					"ref":        r.CommonName(),
					"chunkStart": chunkStart,
					"chunkSize":  chunkSize,
				}).Debug("Early accept of chunk in PATCH before PUT request")
			} else if resp.HTTPResponse().StatusCode != 202 {
				return "", 0, fmt.Errorf("Failed to send blob (chunk), ref %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
			}
			chunkStart += int64(chunkSize)
			location := resp.HTTPResponse().Header.Get("Location")
			if location != "" {
				reg.log.WithFields(logrus.Fields{
					"location": location,
				}).Debug("Next chunk upload location received")
				prevURL := resp.HTTPResponse().Request.URL
				parseURL, err := prevURL.Parse(location)
				if err != nil {
					return "", 0, fmt.Errorf("Failed to send blob (parse next chunk location), ref %s: %w", r.CommonName(), err)
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

	header := http.Header{
		"Content-Type":  {"application/octet-stream"},
		"Content-Range": {fmt.Sprintf("%d-%d", chunkStart, chunkStart)},
	}
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "PUT",
				Repository: r.Repository,
				DirectURL:  &chunkURL,
				BodyLen:    int64(0),
				Headers:    header,
			},
		},
		NoMirrors: true,
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, r.CommonName(), err)
	}
	defer resp.Close()
	// 201 follows distribution-spec, 204 is listed as possible in the Docker registry spec
	if resp.HTTPResponse().StatusCode != 201 && resp.HTTPResponse().StatusCode != 204 {
		return "", 0, fmt.Errorf("Failed to send blob (chunk digest), digest %s, ref %s: %w", d, r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	return d, chunkStart, nil
}

// TODO: just take a putURL rather than the uuid and call a delete on that url
func (reg *Reg) blobUploadCancel(ctx context.Context, r ref.Ref, uuid string) error {
	if uuid == "" {
		return fmt.Errorf("Failed to cancel upload %s: uuid undefined", r.CommonName())
	}
	req := &reghttp.Req{
		Host:      r.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "DELETE",
				Repository: r.Repository,
				Path:       "blobs/uploads/" + uuid,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("Failed to cancel upload %s: %w", r.CommonName(), err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 202 {
		return fmt.Errorf("Failed to cancel upload %s: %w", r.CommonName(), reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}
	return nil
}

// blobUploadStatus provides a response with headers indicating the progress of an upload
func (reg *Reg) blobUploadStatus(ctx context.Context, r ref.Ref, putURL *url.URL) (*http.Response, error) {
	req := &reghttp.Req{
		Host: r.Registry,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:     "GET",
				Repository: r.Repository,
				DirectURL:  putURL,
			},
		},
		NoMirrors: true,
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to get upload status: %v", err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 204 {
		return resp.HTTPResponse(), fmt.Errorf("Failed to get upload status: %v", reghttp.HttpError(resp.HTTPResponse().StatusCode))
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
