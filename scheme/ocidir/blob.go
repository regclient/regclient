package ocidir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

// BlobDelete removes a blob from the repository
func (o *OCIDir) BlobDelete(ctx context.Context, r ref.Ref, d types.Descriptor) error {
	return types.ErrNotImplemented
}

// BlobGet retrieves a blob, returning a reader
func (o *OCIDir) BlobGet(ctx context.Context, r ref.Ref, d types.Descriptor) (blob.Reader, error) {
	file := path.Join(r.Path, "blobs", d.Digest.Algorithm().String(), d.Digest.Encoded())
	fd, err := o.fs.Open(file)
	if err != nil {
		return nil, err
	}
	if d.Size <= 0 {
		fi, err := fd.Stat()
		if err != nil {
			fd.Close()
			return nil, err
		}
		d.Size = fi.Size()
	}
	br := blob.NewReader(
		blob.WithRef(r),
		blob.WithReader(fd),
		blob.WithDesc(d),
	)
	o.log.WithFields(logrus.Fields{
		"ref":  r.CommonName(),
		"file": file,
	}).Debug("retrieved blob")
	return br, nil
}

// BlobHead verifies the existence of a blob, the reader contains the headers but no body to read
func (o *OCIDir) BlobHead(ctx context.Context, r ref.Ref, d types.Descriptor) (blob.Reader, error) {
	file := path.Join(r.Path, "blobs", d.Digest.Algorithm().String(), d.Digest.Encoded())
	fd, err := o.fs.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	if d.Size <= 0 {
		fi, err := fd.Stat()
		if err != nil {
			return nil, err
		}
		d.Size = fi.Size()
	}
	br := blob.NewReader(
		blob.WithRef(r),
		blob.WithDesc(d),
	)
	return br, nil
}

// BlobMount attempts to perform a server side copy of the blob
func (o *OCIDir) BlobMount(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d types.Descriptor) error {
	return types.ErrUnsupported
}

// BlobPut sends a blob to the repository, returns the digest and size when successful
func (o *OCIDir) BlobPut(ctx context.Context, r ref.Ref, d types.Descriptor, rdr io.Reader) (types.Descriptor, error) {
	err := o.throttleGet(r).Acquire(ctx)
	if err != nil {
		return d, err
	}
	defer o.throttleGet(r).Release(ctx)
	err = o.initIndex(r, false)
	if err != nil {
		return d, err
	}
	digester := digest.Canonical.Digester()
	rdr = io.TeeReader(rdr, digester.Hash())
	// write the blob to a tmp file
	var dir, tmpPattern string
	if d.Digest != "" && d.Size > 0 {
		dir = path.Join(r.Path, "blobs", d.Digest.Algorithm().String())
		tmpPattern = d.Digest.Encoded() + ".*.tmp"
	} else {
		dir = path.Join(r.Path, "blobs", digest.Canonical.String())
		tmpPattern = "*.tmp"
	}
	err = rwfs.MkdirAll(o.fs, dir, 0777)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return d, fmt.Errorf("failed creating %s: %w", dir, err)
	}
	tmpFile, err := rwfs.CreateTemp(o.fs, dir, tmpPattern)
	if err != nil {
		return d, fmt.Errorf("failed creating blob tmp file: %w", err)
	}
	fi, err := tmpFile.Stat()
	if err != nil {
		return d, fmt.Errorf("failed to stat blob tmpfile: %w", err)
	}
	tmpName := fi.Name()
	i, err := io.Copy(tmpFile, rdr)
	tmpFile.Close()
	if err != nil {
		return d, err
	}
	// validate result matches descriptor, or update descriptor if it wasn't defined
	if d.Digest == "" || d.Size <= 0 {
		d.Digest = digester.Digest()
	} else if d.Digest != digester.Digest() {
		return d, fmt.Errorf("unexpected digest, expected %s, computed %s", d.Digest, digester.Digest())
	}
	if d.Size <= 0 {
		d.Size = i
	} else if i != d.Size {
		return d, fmt.Errorf("unexpected blob length, expected %d, received %d", d.Size, i)
	}
	file := path.Join(r.Path, "blobs", d.Digest.Algorithm().String(), d.Digest.Encoded())
	err = o.fs.Rename(path.Join(dir, tmpName), file)
	if err != nil {
		return d, fmt.Errorf("failed to write blob (rename tmp file %s to %s): %w", path.Join(dir, tmpName), file, err)
	}
	o.log.WithFields(logrus.Fields{
		"ref":  r.CommonName(),
		"file": file,
	}).Debug("pushed blob")

	o.mu.Lock()
	o.refMod(r)
	o.mu.Unlock()
	return d, nil
}
