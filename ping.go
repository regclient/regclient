package regclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/regclient/regclient/types"
)

type repoPinger interface {
	Ping(ctx context.Context, hostname string) error
}

// Ping accesses the registry's root endpoint, which allows to determine whether
// the registry implements the OCI Distribution Specification and whether the
// registry accepts the supplied credentials.
func (rc *RegClient) Ping(ctx context.Context, hostname string) error {
	i := strings.Index(hostname, "/")
	if i > 0 {
		return fmt.Errorf("invalid hostname: %s%.0w", hostname, types.ErrParsingFailed)
	}
	schemeAPI, err := rc.schemeGet("reg")
	if err != nil {
		return err
	}
	rp, ok := schemeAPI.(repoPinger)
	if !ok {
		return types.ErrNotImplemented
	}
	return rp.Ping(ctx, hostname)
}
