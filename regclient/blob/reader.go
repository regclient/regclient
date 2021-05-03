package blob

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/regclient/types"
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
	readBytes int64
	reader    io.Reader
	closer    io.Closer
	digester  digest.Digester
	// io.ReadCloser
}

// NewReader creates a new reader
func NewReader(rdr io.ReadCloser) Reader {
	digester := digest.Canonical.Digester()
	digestRdr := io.TeeReader(rdr, digester.Hash())
	bc := common{
		ref: types.Ref{},
	}
	if rdr != nil {
		bc.blobSet = true
	}
	br := reader{
		common:   bc,
		reader:   digestRdr,
		closer:   rdr,
		digester: digester,
	}
	return &br
}

func (b *reader) Close() error {
	return b.closer.Close()
}

// RawBody returns the original body from the request
func (b *reader) RawBody() ([]byte, error) {
	return ioutil.ReadAll(b)
}

// Read passes through the read operation while computing the digest and tracking the size
func (b *reader) Read(p []byte) (int, error) {
	size, err := b.reader.Read(p)
	b.readBytes = b.readBytes + int64(size)
	if err == io.EOF {
		// check/save size
		if b.cl == 0 {
			b.cl = b.readBytes
		} else if b.readBytes != b.cl {
			err = fmt.Errorf("Expected size mismatch [expected %d, received %d]: %w", b.cl, b.readBytes, err)
		}
		// check/save digest
		if b.digest == "" {
			b.digest = b.digester.Digest()
		} else if b.digest != b.digester.Digest() {
			err = fmt.Errorf("Expected digest mismatch [expected %s, calculated %s]: %w", b.digest.String(), b.digester.Digest().String(), err)
		}
	}
	return size, err
}

// ToOCIConfig converts a blobReader to a BlobOCIConfig
func (b *reader) ToOCIConfig() (OCIConfig, error) {
	if !b.blobSet {
		return nil, fmt.Errorf("Blob is not defined")
	}
	if b.readBytes != 0 {
		return nil, fmt.Errorf("Unable to convert after read has been performed")
	}
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
