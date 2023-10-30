package regclient

import (
	"context"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

type repoPinger interface {
	Ping(ctx context.Context, r ref.Ref) error
}

// Ping accesses the registry's root endpoint, which allows to determine whether
// the registry implements the OCI Distribution Specification and whether the
// registry accepts the supplied credentials.
func (rc *RegClient) Ping(ctx context.Context, r ref.Ref) error {
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return err
	}

	rp, ok := schemeAPI.(repoPinger)
	if !ok {
		return types.ErrNotImplemented
	}

	return rp.Ping(ctx, r)
}
