package regclient

import (
	"context"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
)

// ReferrerList retrieves a list of referrers to a manifest.
// The descriptor list should contain manifests that each have a subject field matching the requested ref.
func (rc *RegClient) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return referrer.ReferrerList{}, err
	}
	return schemeAPI.ReferrerList(ctx, r, opts...)
}
