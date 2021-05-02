package blob

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Reader is an unprocessed Blob with an available ReadCloser for reading the Blob
type Reader interface {
	Blob
	io.ReadCloser
	ToOCIConfig() (OCIConfig, error)
}

// reader is the internal struct implementing BlobReader
type reader struct {
	common
	io.ReadCloser
}

// TODO: wrap the ReadCloser, running all reads through a digester
// on final EOF, either save the digest and length, or report an error if the length and/or digest doesn't match

// NewReader creates a new reader
func NewReader(rc io.ReadCloser) Reader {
	bc := common{}
	if rc != nil {
		bc.blobSet = true
	}
	br := reader{
		common:     bc,
		ReadCloser: rc,
	}
	return &br
}

// RawBody returns the original body from the request
func (b *reader) RawBody() ([]byte, error) {
	return ioutil.ReadAll(b)
}

// ToOCIConfig converts a blobReader to a BlobOCIConfig
func (b *reader) ToOCIConfig() (OCIConfig, error) {
	if !b.blobSet {
		return nil, fmt.Errorf("Blob is not defined")
	}
	// TODO: error if blob read has already been called
	blobBody, err := ioutil.ReadAll(b)
	if err != nil {
		return nil, fmt.Errorf("Error reading image config for %s: %w", b.ref.CommonName(), err)
	}
	var ociImage ociv1.Image
	err = json.Unmarshal(blobBody, &ociImage)
	if err != nil {
		return nil, fmt.Errorf("Error parsing image config for %s: %w", b.ref.CommonName(), err)
	}
	// return the resulting blobOCIConfig, reuse blobCommon, setting rawBody read above, and the unmarshaled OCI image config
	return &ociConfig{common: b.common, rawBody: blobBody, Image: ociImage}, nil
}
