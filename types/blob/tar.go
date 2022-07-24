package blob

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/archive"
)

// TarReader reads or writes to a blob with tar contents and optional compression
type TarReader interface {
	Blob
	io.Closer
	GetTarReader() (*tar.Reader, error)
}

type tarReader struct {
	common
	origRdr  io.Reader
	reader   io.Reader
	digester digest.Digester
	tr       *tar.Reader
}

// NewTarReader creates a TarReader
func NewTarReader(opts ...Opts) TarReader {
	bc := blobConfig{}
	for _, opt := range opts {
		opt(&bc)
	}
	c := common{
		desc:      bc.desc,
		r:         bc.r,
		rawHeader: bc.header,
		resp:      bc.resp,
	}
	tr := tarReader{
		common:  c,
		origRdr: bc.rdr,
	}
	if bc.rdr != nil {
		tr.blobSet = true
		tr.digester = digest.Canonical.Digester()
		tr.reader = io.TeeReader(bc.rdr, tr.digester.Hash())
	}
	return &tr
}

// Close attempts to close the reader and populates/validates the digest
func (tr *tarReader) Close() error {
	var err error
	if tr.digester != nil {
		dig := tr.digester.Digest()
		tr.digester = nil
		if tr.desc.Digest.String() != "" && dig != tr.desc.Digest {
			err = fmt.Errorf("digest mismatch, expected %s, received %s", tr.desc.Digest.String(), dig.String())
		}
		tr.desc.Digest = dig
	}
	if tr.origRdr == nil {
		return err
	}
	// attempt to close if available in original reader
	if trc, ok := tr.origRdr.(io.Closer); ok {
		return trc.Close()
	}
	return err
}

// GetTarReader returns the tar.Reader for the blob
func (tr *tarReader) GetTarReader() (*tar.Reader, error) {
	if tr.reader == nil {
		return nil, fmt.Errorf("blob has no reader defined")
	}
	if tr.tr == nil {
		dr, err := archive.Decompress(tr.reader)
		if err != nil {
			return nil, err
		}
		tr.tr = tar.NewReader(dr)
	}
	return tr.tr, nil
}

// RawBody returns the original body from the request
func (tr *tarReader) RawBody() ([]byte, error) {
	if !tr.blobSet {
		return []byte{}, fmt.Errorf("Blob is not defined")
	}
	if tr.tr != nil {
		return []byte{}, fmt.Errorf("RawBody cannot be returned after TarReader returned")
	}
	b, err := ioutil.ReadAll(tr.reader)
	if err != nil {
		return b, err
	}
	err = tr.Close()
	return b, err
}
