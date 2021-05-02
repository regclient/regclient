package blob

import (
	"net/http"
	"strconv"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/regclient/types"
)

// Common interface is provided by all Blob implementations
type Common interface {
	Digest() digest.Digest
	Length() int64
	MediaType() string
	Response() *http.Response
	RawHeaders() http.Header
	SetMeta(ref types.Ref, d digest.Digest, cl int64)
	SetResp(resp *http.Response)
}

type common struct {
	ref       types.Ref
	digest    digest.Digest
	cl        int64
	mt        string
	blobSet   bool
	rawHeader http.Header
	resp      *http.Response
}

// Digest returns the provided or calculated digest of the blob
func (b *common) Digest() digest.Digest {
	return b.digest
}

// Length returns the provided or calculated length of the blob
func (b *common) Length() int64 {
	return b.cl
}

// MediaType returns the Content-Type header received from the registry
func (b *common) MediaType() string {
	return b.mt
}

// RawHeaders returns the headers received from the registry
func (b *common) RawHeaders() http.Header {
	return b.rawHeader
}

// Response returns the response associated with the blob
func (b *common) Response() *http.Response {
	return b.resp
}

// SetMeta sets the various blob metadata (reference, digest, and content length)
func (b *common) SetMeta(ref types.Ref, d digest.Digest, cl int64) {
	b.ref = ref
	b.digest = d
	b.cl = cl
}

// SetResp sets the response header data when pulling from a registry
func (b *common) SetResp(resp *http.Response) {
	if resp == nil {
		return
	}
	b.resp = resp
	b.rawHeader = resp.Header
	cl, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	b.cl = int64(cl)
	b.mt = resp.Header.Get("Content-Type")
}
