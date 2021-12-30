package regclient

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func (rc *RegClient) BlobCopy(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d digest.Digest) error {
	// for the same repository, there's nothing to copy
	if refSrc.Registry == refTgt.Registry && refSrc.Repository == refTgt.Repository {
		rc.log.WithFields(logrus.Fields{
			"src":    refTgt.Reference,
			"tgt":    refTgt.Reference,
			"digest": d,
		}).Debug("Blob copy skipped, same repo")
		return nil
	}
	// check if layer already exists
	if _, err := rc.BlobHead(ctx, refTgt, d); err == nil {
		rc.log.WithFields(logrus.Fields{
			"tgt":    refTgt.Reference,
			"digest": d,
		}).Debug("Blob copy skipped, already exists")
		return nil
	}
	// try mounting blob from the source repo is the registry is the same
	if refSrc.Registry == refTgt.Registry {
		err := rc.BlobMount(ctx, refSrc, refTgt, d)
		if err == nil {
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
	defer blobIO.Close()
	if _, _, err := rc.BlobPut(ctx, refTgt, d, blobIO, blobIO.Response().ContentLength); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
			"src": refSrc.Reference,
			"tgt": refTgt.Reference,
		}).Warn("Failed to push blob")
		return err
	}
	return nil
}

func (rc *RegClient) BlobDelete(ctx context.Context, r ref.Ref, d digest.Digest) error {
	schemeAPI, err := rc.getScheme(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.BlobDelete(ctx, r, d)
}

func (rc *RegClient) BlobGet(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error) {
	schemeAPI, err := rc.getScheme(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.BlobGet(ctx, r, d)
}

func (rc *RegClient) BlobGetOCIConfig(ctx context.Context, ref ref.Ref, d digest.Digest) (blob.OCIConfig, error) {
	b, err := rc.BlobGet(ctx, ref, d)
	if err != nil {
		return nil, err
	}
	return b.ToOCIConfig()
}

// BlobHead is used to verify if a blob exists and is accessible
func (rc *RegClient) BlobHead(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error) {
	schemeAPI, err := rc.getScheme(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.BlobHead(ctx, r, d)
}

// BlobMount attempts to perform a server side copy/mount of the blob between repositories
func (rc *RegClient) BlobMount(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d digest.Digest) error {
	schemeAPI, err := rc.getScheme(refSrc.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.BlobMount(ctx, refSrc, refTgt, d)
}

// BlobPut uploads a blob to a repository.
// This will attempt an anonymous blob mount first which some registries may support.
// It will then try doing a full put of the blob without chunking (most widely supported).
// If the full put fails, it will fall back to a chunked upload (useful for flaky networks).
func (rc *RegClient) BlobPut(ctx context.Context, ref ref.Ref, d digest.Digest, rdr io.Reader, cl int64) (digest.Digest, int64, error) {
	schemeAPI, err := rc.getScheme(ref.Scheme)
	if err != nil {
		return "", 0, err
	}
	return schemeAPI.BlobPut(ctx, ref, d, rdr, cl)
}
