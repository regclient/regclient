package regclient

import (
	"context"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

func (rc *RegClient) ManifestDelete(ctx context.Context, r ref.Ref) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.ManifestDelete(ctx, r)
}

func (rc *RegClient) ManifestGet(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.ManifestGet(ctx, r)
}

func (rc *RegClient) ManifestHead(ctx context.Context, r ref.Ref) (manifest.Manifest, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return nil, err
	}
	return schemeAPI.ManifestHead(ctx, r)
}

func (rc *RegClient) ManifestPut(ctx context.Context, r ref.Ref, m manifest.Manifest, opts ...scheme.ManifestOpts) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.ManifestPut(ctx, r, m, opts...)
}
