package blob

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/types/ref"
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
	origRdr   io.ReadCloser
	digester  digest.Digester
	// io.ReadCloser
}

// NewReader creates a new reader
func NewReader(rdr io.ReadCloser) Reader {
	digester := digest.Canonical.Digester()
	digestRdr := io.TeeReader(rdr, digester.Hash())
	bc := common{
		r: ref.Ref{},
	}
	if rdr != nil {
		bc.blobSet = true
	}
	br := reader{
		common:   bc,
		reader:   digestRdr,
		origRdr:  rdr,
		digester: digester,
	}
	return &br
}

func (b *reader) Close() error {
	if b.origRdr == nil {
		return nil
	}
	return b.origRdr.Close()
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

// Seek passes through the seek operation, reseting or invalidating the digest
func (b *reader) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == io.SeekCurrent {
		return b.readBytes, nil
	}
	// cannot do an arbitrary seek and still digest without a lot more complication
	if offset != 0 || whence != io.SeekStart {
		return b.readBytes, fmt.Errorf("Unable to seek to arbitrary position")
	}
	rdrSeek, ok := b.origRdr.(io.Seeker)
	if !ok {
		return b.readBytes, fmt.Errorf("Seek unsupported")
	}
	o, err := rdrSeek.Seek(offset, whence)
	if err != nil || o != 0 {
		return b.readBytes, err
	}
	// reset internal offset and digest calculation
	digester := digest.Canonical.Digester()
	digestRdr := io.TeeReader(b.origRdr, digester.Hash())
	b.digester = digester
	b.readBytes = 0
	b.reader = digestRdr

	return 0, nil
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
		return nil, fmt.Errorf("Error reading image config for %s: %w", b.r.CommonName(), err)
	}
	var ociImage ociv1.Image
	err = json.Unmarshal(blobBody, &ociImage)
	if err != nil {
		return nil, fmt.Errorf("Error parsing image config for %s: %w", b.r.CommonName(), err)
	}
	// return the resulting blobOCIConfig, reuse blobCommon, setting rawBody read above, and the unmarshaled OCI image config
	return &ociConfig{common: b.common, rawBody: blobBody, Image: ociImage}, nil
}
