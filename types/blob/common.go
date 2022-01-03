package blob

import (
	"net/http"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/types/ref"
)

// Common interface is provided by all Blob implementations
type Common interface {
	Digest() digest.Digest
	Length() int64
	MediaType() string
	Response() *http.Response
	RawHeaders() http.Header
}

type common struct {
	r         ref.Ref
	desc      ociv1.Descriptor
	blobSet   bool
	rawHeader http.Header
	resp      *http.Response
}

// Digest returns the provided or calculated digest of the blob
func (b *common) Digest() digest.Digest {
	return b.desc.Digest
}

// Length returns the provided or calculated length of the blob
func (b *common) Length() int64 {
	return b.desc.Size
}

// MediaType returns the Content-Type header received from the registry
func (b *common) MediaType() string {
	return b.desc.MediaType
}

// RawHeaders returns the headers received from the registry
func (b *common) RawHeaders() http.Header {
	return b.rawHeader
}

// Response returns the response associated with the blob
func (b *common) Response() *http.Response {
	return b.resp
}
