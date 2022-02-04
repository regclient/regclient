package regclient

import (
	"context"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

// ManifestDelete removes a manifest, including all tags pointing to that registry
// The reference must include the digest to delete (see TagDelete for deleting a tag)
// All tags pointing to the manifest will be deleted
func (rc *RegClient) ManifestDelete(ctx context.Context, r ref.Ref) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.ManifestDelete(ctx, r)
}

// ManifestGet retrieves a manifest
func (rc *RegClient) ManifestGet(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.ManifestGet(ctx, r)
}

// ManifestHead queries for the existence of a manifest and returns metadata (digest, media-type, size)
func (rc *RegClient) ManifestHead(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.ManifestHead(ctx, r)
}

// ManifestPut pushes a manifest
// Any descriptors referenced by the manifest typically need to be pushed first
func (rc *RegClient) ManifestPut(ctx context.Context, r ref.Ref, m manifest.Manifest, opts ...scheme.ManifestOpts) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.ManifestPut(ctx, r, m, opts...)
}
