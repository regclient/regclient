package blob

import (
	"fmt"
	"io"
	"strconv"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/limitread"
	"github.com/regclient/regclient/types"
)

// Reader is an unprocessed Blob with an available ReadCloser for reading the Blob
type Reader interface {
	Blob
	io.ReadSeekCloser
	ToOCIConfig() (OCIConfig, error)
	ToTarReader() (TarReader, error)
}

// reader is the internal struct implementing BlobReader
type reader struct {
	common
	readBytes int64
	reader    io.Reader
	origRdr   io.Reader
	digester  digest.Digester
}

// NewReader creates a new reader
func NewReader(opts ...Opts) Reader {
	bc := blobConfig{}
	for _, opt := range opts {
		opt(&bc)
	}
	if bc.resp != nil {
		// extract headers and reader if other fields not passed
		if bc.header == nil {
			bc.header = bc.resp.Header
		}
		if bc.rdr == nil {
			bc.rdr = bc.resp.Body
		}
	}
	if bc.header != nil {
		// extract fields from header if descriptor not passed
		if bc.desc.MediaType == "" {
			bc.desc.MediaType = types.MediaTypeBase(bc.header.Get("Content-Type"))
		}
		if bc.desc.Size == 0 {
			cl, _ := strconv.Atoi(bc.header.Get("Content-Length"))
			bc.desc.Size = int64(cl)
		}
		if bc.desc.Digest == "" {
			bc.desc.Digest, _ = digest.Parse(bc.header.Get("Docker-Content-Digest"))
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
		origRdr: bc.rdr,
	}
	if bc.rdr != nil {
		br.blobSet = true
		br.digester = digest.Canonical.Digester()
		rdr := bc.rdr
		if br.desc.Size > 0 {
			rdr = &limitread.LimitRead{
				Reader: rdr,
				Limit:  br.desc.Size,
			}
		}
		br.reader = io.TeeReader(rdr, br.digester.Hash())
	}
	return &br
}

func (b *reader) Close() error {
	if b.origRdr == nil {
		return nil
	}
	// attempt to close if available in original reader
	bc, ok := b.origRdr.(io.Closer)
	if !ok {
		return nil
	}
	return bc.Close()
}

// RawBody returns the original body from the request
func (b *reader) RawBody() ([]byte, error) {
	return io.ReadAll(b)
}

// Read passes through the read operation while computing the digest and tracking the size
func (b *reader) Read(p []byte) (int, error) {
	if b.reader == nil {
		return 0, fmt.Errorf("blob has no reader: %w", io.ErrUnexpectedEOF)
	}
	size, err := b.reader.Read(p)
	b.readBytes = b.readBytes + int64(size)
	if err == io.EOF {
		// check/save size
		if b.desc.Size == 0 {
			b.desc.Size = b.readBytes
		} else if b.readBytes < b.desc.Size {
			err = fmt.Errorf("%w [expected %d, received %d]: %v", types.ErrShortRead, b.desc.Size, b.readBytes, err)
		} else if b.readBytes > b.desc.Size {
			err = fmt.Errorf("%w [expected %d, received %d]: %v", types.ErrSizeLimitExceeded, b.desc.Size, b.readBytes, err)
		}
		// check/save digest
		if b.desc.Digest == "" {
			b.desc.Digest = b.digester.Digest()
		} else if b.desc.Digest != b.digester.Digest() {
			err = fmt.Errorf("%w [expected %s, calculated %s]: %v", types.ErrDigestMismatch, b.desc.Digest.String(), b.digester.Digest().String(), err)
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
		return b.readBytes, fmt.Errorf("unable to seek to arbitrary position")
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
	rdr := b.origRdr
	if b.desc.Size > 0 {
		rdr = &limitread.LimitRead{
			Reader: rdr,
			Limit:  b.desc.Size,
		}
	}
	digester := digest.Canonical.Digester()
	b.reader = io.TeeReader(rdr, digester.Hash())
	b.digester = digester
	b.readBytes = 0

	return 0, nil
}

// ToOCIConfig converts a blobReader to a BlobOCIConfig
func (b *reader) ToOCIConfig() (OCIConfig, error) {
	if !b.blobSet {
		return nil, fmt.Errorf("blob is not defined")
	}
	if b.readBytes != 0 {
		return nil, fmt.Errorf("unable to convert after read has been performed")
	}
	blobBody, err := io.ReadAll(b)
	errC := b.Close()
	if err != nil {
		return nil, fmt.Errorf("error reading image config for %s: %w", b.r.CommonName(), err)
	}
	if errC != nil {
		return nil, fmt.Errorf("error closing blob reader: %w", err)
	}
	return NewOCIConfig(
		WithDesc(b.desc),
		WithHeader(b.rawHeader),
		WithRawBody(blobBody),
		WithRef(b.r),
		WithResp(b.resp),
	), nil
}

func (b *reader) ToTarReader() (TarReader, error) {
	if !b.blobSet {
		return nil, fmt.Errorf("blob is not defined")
	}
	if b.readBytes != 0 {
		return nil, fmt.Errorf("unable to convert after read has been performed")
	}
	return NewTarReader(
		WithDesc(b.desc),
		WithHeader(b.rawHeader),
		WithRef(b.r),
		WithResp(b.resp),
		WithReader(b.reader),
	), nil
}
