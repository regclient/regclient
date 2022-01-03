package blob

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	"github.com/opencontainers/go-digest"
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
	readBytes int64
	reader    io.Reader
	origRdr   io.ReadCloser
	digester  digest.Digester
}

// NewReader creates a new reader
func NewReader(opts ...Opts) Reader {
	bc := BlobConfig{}
	for _, opt := range opts {
		opt(&bc)
	}
	if bc.resp != nil {
		// extract headers and reader if other fields not passed
		if bc.desc.MediaType == "" {
			bc.desc.MediaType = bc.resp.Header.Get("Content-Type")
		}
		if bc.desc.Size == 0 {
			cl, _ := strconv.Atoi(bc.resp.Header.Get("Content-Length"))
			bc.desc.Size = int64(cl)
		}
		if bc.desc.Digest == "" {
			bc.desc.Digest = digest.FromString(bc.resp.Header.Get("Docker-Content-Digest"))
		}
	}
	c := common{
		r:         bc.r,
		desc:      bc.desc,
		rawHeader: bc.header,
		resp:      bc.resp,
	}
	br := reader{
		common:  c,
		origRdr: bc.rc,
	}
	if bc.rc != nil {
		br.blobSet = true
		br.digester = digest.Canonical.Digester()
		br.reader = io.TeeReader(bc.rc, br.digester.Hash())
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
		if b.desc.Size == 0 {
			b.desc.Size = b.readBytes
		} else if b.readBytes != b.desc.Size {
			err = fmt.Errorf("Expected size mismatch [expected %d, received %d]: %w", b.desc.Size, b.readBytes, err)
		}
		// check/save digest
		if b.desc.Digest == "" {
			b.desc.Digest = b.digester.Digest()
		} else if b.desc.Digest != b.digester.Digest() {
			err = fmt.Errorf("Expected digest mismatch [expected %s, calculated %s]: %w", b.desc.Digest.String(), b.digester.Digest().String(), err)
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
