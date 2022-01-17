package ocidir

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/ref"
)

// BlobDelete removes a blob from the repository
func (o *OCIDir) BlobDelete(ctx context.Context, r ref.Ref, d digest.Digest) error {
	return types.ErrNotImplemented
}

// BlobGet retrieves a blob, returning a reader
func (o *OCIDir) BlobGet(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error) {
	file := path.Join(r.Path, "blobs", d.Algorithm().String(), d.Encoded())
	fd, err := o.fs.Open(file)
	if err != nil {
		return nil, err
	}
	fi, err := fd.Stat()
	if err != nil {
		fd.Close()
		return nil, err
	}
	br := blob.NewReader(
		blob.WithRef(r),
		blob.WithReader(fd),
		blob.WithDesc(ociv1.Descriptor{
			Digest: d,
			Size:   fi.Size(),
		}),
	)
	return br, nil
}

// BlobHead verifies the existence of a blob, the reader contains the headers but no body to read
func (o *OCIDir) BlobHead(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error) {
	file := path.Join(r.Path, "blobs", d.Algorithm().String(), d.Encoded())
	fd, err := o.fs.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	fi, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	br := blob.NewReader(
		blob.WithRef(r),
		blob.WithDesc(ociv1.Descriptor{
			Digest: d,
			Size:   fi.Size(),
		}),
	)
	return br, nil
}

// BlobMount attempts to perform a server side copy of the blob
func (o *OCIDir) BlobMount(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d digest.Digest) error {
	return types.ErrUnsupported
}

// BlobPut sends a blob to the repository, returns the digest and size when successful
func (o *OCIDir) BlobPut(ctx context.Context, r ref.Ref, d digest.Digest, rdr io.Reader, cl int64) (digest.Digest, int64, error) {
	digester := digest.Canonical.Digester()
	rdr = io.TeeReader(rdr, digester.Hash())
	// if digest unavailable, read into a []byte+digest, and replace rdr
	if d == "" {
		b, err := io.ReadAll(rdr)
		if err != nil {
			return "", 0, err
		}
		d = digester.Digest()
		cl = int64(len(b))
		rdr = bytes.NewReader(b)
		digester = nil // no need to recompute or validate digest
	}
	// write the blob to the CAS file
	dir := path.Join(r.Path, "blobs", d.Algorithm().String())
	err := rwfs.MkdirAll(o.fs, dir, 0777)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return "", 0, fmt.Errorf("failed creating %s: %w", dir, err)
	}
	file := path.Join(r.Path, "blobs", d.Algorithm().String(), d.Encoded())
	fd, err := o.fs.Create(file)
	if err != nil {
		return "", 0, fmt.Errorf("failed creating %s: %w", file, err)
	}
	defer fd.Close()
	i, err := io.Copy(fd, rdr)
	if err != nil {
		return "", 0, err
	}
	// validate result
	if digester != nil && d != digester.Digest() {
		return "", 0, fmt.Errorf("unexpected digest, expected %s, computed %s", d, digester.Digest())
	}
	if cl > 0 && i != cl {
		return "", 0, fmt.Errorf("unexpected blob length, expected %d, received %d", cl, i)
	}
	cl = i
	return d, cl, nil
}
