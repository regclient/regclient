package blob

import (
	"io"
	"net/http"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/types/ref"
)

// Blob interface is used for returning blobs
type Blob interface {
	Common
	RawBody() ([]byte, error)
}

type BlobConfig struct {
	desc   ociv1.Descriptor
	header http.Header
	image  ociv1.Image
	r      ref.Ref
	rdr    io.Reader
	resp   *http.Response
}

type Opts func(*BlobConfig)

func WithDesc(d ociv1.Descriptor) Opts {
	return func(bc *BlobConfig) {
		bc.desc = d
	}
}
func WithHeader(header http.Header) Opts {
	return func(bc *BlobConfig) {
		bc.header = header
	}

}
func WithImage(image ociv1.Image) Opts {
	return func(bc *BlobConfig) {
		bc.image = image
	}
}
func WithReader(rc io.Reader) Opts {
	return func(bc *BlobConfig) {
		bc.rdr = rc
	}
}
func WithRef(r ref.Ref) Opts {
	return func(bc *BlobConfig) {
		bc.r = r
	}
}
func WithResp(resp *http.Response) Opts {
	return func(bc *BlobConfig) {
		bc.resp = resp
		if bc.header == nil {
			bc.header = resp.Header
		}
	}
}
