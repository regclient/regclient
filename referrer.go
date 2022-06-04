package regclient

import (
	"context"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
)

// ReferrerList retrieves a manifest
func (rc *RegClient) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return referrer.ReferrerList{}, err
	}
	return schemeAPI.ReferrerList(ctx, r, opts...)
}

// ReferrerPut pushes a manifest
// Any descriptors referenced by the manifest typically need to be pushed first
func (rc *RegClient) ReferrerPut(ctx context.Context, r ref.Ref, m manifest.Manifest) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}
	return schemeAPI.ReferrerPut(ctx, r, m)
}
