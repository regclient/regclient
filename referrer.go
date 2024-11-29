package regclient

import (
	"context"
	"fmt"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/referrer"
	"github.com/regclient/regclient/types/warning"
)

// ReferrerList retrieves a list of referrers to a manifest.
// The descriptor list should contain manifests that each have a subject field matching the requested ref.
func (rc *RegClient) ReferrerList(ctx context.Context, r ref.Ref, opts ...scheme.ReferrerOpts) (referrer.ReferrerList, error) {
	if !r.IsSet() {
		return referrer.ReferrerList{}, fmt.Errorf("ref is not set: %s%.0w", r.CommonName(), errs.ErrInvalidReference)
	}
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}
	// resolve ref to a digest
	config := scheme.ReferrerConfig{}
	for _, opt := range opts {
		opt(&config)
	}
	if r.Digest == "" || config.Platform != "" {
		mo := []ManifestOpts{WithManifestRequireDigest()}
		if config.Platform != "" {
			p, err := platform.Parse(config.Platform)
			if err != nil {
				return referrer.ReferrerList{}, fmt.Errorf("failed to lookup referrer platform: %w", err)
			}
			mo = append(mo, WithManifestPlatform(p))
		}
		m, err := rc.ManifestHead(ctx, r, mo...)
		if err != nil {
			return referrer.ReferrerList{}, fmt.Errorf("failed to get digest for subject: %w", err)
		}
		r = r.SetDigest(m.GetDescriptor().Digest.String())
	}
	// update for a new referrer source repo if requested
	if !config.SrcRepo.IsZero() {
		r = config.SrcRepo.SetDigest(r.Digest)
	}
	schemeAPI, err := rc.schemeGet(r.Scheme)
	if err != nil {
		return referrer.ReferrerList{}, err
	}
	return schemeAPI.ReferrerList(ctx, r, opts...)
}
