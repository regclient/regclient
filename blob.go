package regclient

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

const blobCBFreq = time.Millisecond * 100

type blobOpt struct {
	callback func(kind, instance, state string, cur, total int64)
}

// BlobOpts define options for the Image* commands
type BlobOpts func(*blobOpt)

// BlobWithCallback provides progress data to a callback function
func BlobWithCallback(callback func(kind, instance, state string, cur, total int64)) BlobOpts {
	return func(opts *blobOpt) {
		opts.callback = callback
	}
}

// BlobCopy copies a blob between two locations
// If the blob already exists in the target, the copy is skipped
// A server side cross repository blob mount is attempted
func (rc *RegClient) BlobCopy(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d types.Descriptor, opts ...BlobOpts) error {
	var opt blobOpt
	for _, optFn := range opts {
		optFn(&opt)
	}
	tDesc := d
	tDesc.URLs = []string{} // ignore URLs when pushing to target
	// for the same repository, there's nothing to copy
	if ref.EqualRepository(refSrc, refTgt) {
		if opt.callback != nil {
			opt.callback("blob", d.Digest.String(), "skipped", 0, d.Size)
		}
		rc.log.WithFields(logrus.Fields{
			"src":    refTgt.Reference,
			"tgt":    refTgt.Reference,
			"digest": d.Digest,
		}).Debug("Blob copy skipped, same repo")
		return nil
	}
	// check if layer already exists
	if _, err := rc.BlobHead(ctx, refTgt, tDesc); err == nil {
		if opt.callback != nil {
			opt.callback("blob", d.Digest.String(), "skipped", 0, d.Size)
		}
		rc.log.WithFields(logrus.Fields{
			"tgt":    refTgt.Reference,
			"digest": d,
		}).Debug("Blob copy skipped, already exists")
		return nil
	}
	// try mounting blob from the source repo is the registry is the same
	if ref.EqualRegistry(refSrc, refTgt) {
		err := rc.BlobMount(ctx, refSrc, refTgt, d)
		if err == nil {
			if opt.callback != nil {
				opt.callback("blob", d.Digest.String(), "skipped", 0, d.Size)
			}
			rc.log.WithFields(logrus.Fields{
				"src":    refTgt.Reference,
				"tgt":    refTgt.Reference,
				"digest": d,
			}).Debug("Blob copy performed server side with registry mount")
			return nil
		}
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to mount blob")
	}
	// fast options failed, download layer from source and push to target
	blobIO, err := rc.BlobGet(ctx, refSrc, d)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":    err,
			"src":    refSrc.Reference,
			"digest": d,
		}).Warn("Failed to retrieve blob")
		return err
	}
	if opt.callback != nil {
		opt.callback("blob", d.Digest.String(), "started", 0, d.Size)
		ticker := time.NewTicker(blobCBFreq)
		done := make(chan bool)
		defer func() {
			close(done)
			ticker.Stop()
			if ctx.Err() == nil {
				opt.callback("blob", d.Digest.String(), "finished", d.Size, d.Size)
			}
		}()
		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					offset, err := blobIO.Seek(0, io.SeekCurrent)
					if err == nil && offset > 0 {
						opt.callback("blob", d.Digest.String(), "active", offset, d.Size)
					}
				}
			}
		}()
	}
	defer blobIO.Close()
	if _, err := rc.BlobPut(ctx, refTgt, blobIO.GetDescriptor(), blobIO); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to push blob")
		return err
	}
	return nil
}

// BlobDelete removes a blob from the registry
// This method should only be used to repair a damaged registry
// Typically a server side garbage collection should be used to purge unused blobs
func (rc *RegClient) BlobDelete(ctx context.Context, r ref.Ref, d types.Descriptor) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.BlobDelete(ctx, r, d)
}

// BlobGet retrieves a blob, returning a reader
func (rc *RegClient) BlobGet(ctx context.Context, r ref.Ref, d types.Descriptor) (blob.Reader, error) {
	data, err := d.GetData()
	if err == nil {
		return blob.NewReader(blob.WithDesc(d), blob.WithRef(r), blob.WithReader(bytes.NewReader(data))), nil
	}
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.BlobGet(ctx, r, d)
}

// BlobGetOCIConfig retrieves an OCI config from a blob, automatically extracting the JSON
func (rc *RegClient) BlobGetOCIConfig(ctx context.Context, ref ref.Ref, d types.Descriptor) (blob.OCIConfig, error) {
	b, err := rc.BlobGet(ctx, ref, d)
	if err != nil {
		return nil, err
	}
	return b.ToOCIConfig()
}

// BlobHead is used to verify if a blob exists and is accessible
func (rc *RegClient) BlobHead(ctx context.Context, r ref.Ref, d types.Descriptor) (blob.Reader, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.BlobHead(ctx, r, d)
}

// BlobMount attempts to perform a server side copy/mount of the blob between repositories
func (rc *RegClient) BlobMount(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d types.Descriptor) error {
	schemeAPI, err := rc.schemeGet(refSrc.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.BlobMount(ctx, refSrc, refTgt, d)
}

// BlobPut uploads a blob to a repository.
// This will attempt an anonymous blob mount first which some registries may support.
// It will then try doing a full put of the blob without chunking (most widely supported).
// If the full put fails, it will fall back to a chunked upload (useful for flaky networks).
func (rc *RegClient) BlobPut(ctx context.Context, ref ref.Ref, d types.Descriptor, rdr io.Reader) (types.Descriptor, error) {
	schemeAPI, err := rc.schemeGet(ref.Scheme)
	if err != nil {
		return types.Descriptor{}, err
	}
	return schemeAPI.BlobPut(ctx, ref, d, rdr)
}
