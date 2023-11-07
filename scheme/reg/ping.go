package reg

import (
	"context"
	"fmt"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/types/ref"
)

func (reg *Reg) Ping(ctx context.Context, r ref.Ref) error {
	req := &reghttp.Req{
		Host:      r.Registry,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method: "GET",
				Path:   "",
			},
		},
	}

	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to ping registry %s: %w", r.Registry, err)
	}
	defer resp.Close()

	if resp.HTTPResponse().StatusCode != 200 {
		return fmt.Errorf("failed to ping registry %s: %w",
			r.Registry, reghttp.HTTPError(resp.HTTPResponse().StatusCode))
	}

	return nil
}
