package blob

import (
	"encoding/json"
	"fmt"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// OCIConfig wraps an OCI Config struct extracted from a Blob
type OCIConfig interface {
	Blob
	GetConfig() ociv1.Image
}

// ociConfig includes an OCI Config struct extracted from a Blob
// Image is included as an anonymous field to facilitate json and templating calls transparently
type ociConfig struct {
	common
	rawBody []byte
	ociv1.Image
}

// NewOCIConfig creates a new BlobOCIConfig from an OCI Image
func NewOCIConfig(opts ...Opts) OCIConfig {
	bc := BlobConfig{}
	for _, opt := range opts {
		opt(&bc)
	}
	c := common{
		blobSet:   true,
		desc:      bc.desc,
		r:         bc.r,
		rawHeader: bc.header,
		resp:      bc.resp,
	}
	b := ociConfig{
		common: c,
		Image:  bc.image,
	}
	return &b
}

// GetConfig returns the original body from the request
func (b *ociConfig) GetConfig() ociv1.Image {
	return b.Image
}

// RawBody returns the original body from the request
func (b *ociConfig) RawBody() ([]byte, error) {
	var err error
	if !b.blobSet {
		return []byte{}, fmt.Errorf("Blob is not defined")
	}
	if len(b.rawBody) == 0 {
		b.rawBody, err = json.Marshal(b.Image)
	}
	return b.rawBody, err
}
