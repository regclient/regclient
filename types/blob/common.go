package blob

import (
	"net/http"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

// Common interface is provided by all Blob implementations
type Common interface {
	GetDescriptor() types.Descriptor
	Response() *http.Response
	RawHeaders() http.Header

	Digest() digest.Digest // TODO: deprecate
	Length() int64         // TODO: deprecate
	MediaType() string     // TODO: deprecate
}

type common struct {
	r         ref.Ref
	desc      types.Descriptor
	blobSet   bool
	rawHeader http.Header
	resp      *http.Response
}

// GetDescriptor returns the descriptor associated with the blob
func (b *common) GetDescriptor() types.Descriptor {
	return b.desc
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
