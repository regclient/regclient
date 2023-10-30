package reg

import (
	"context"
	"fmt"
	"github.com/regclient/regclient/internal/reghttp"
)

func (reg *Reg) Ping(ctx context.Context, hostname string) error {
	req := &reghttp.Req{
		Host:      hostname,
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
		return fmt.Errorf("failed to ping registry %s: %w", hostname, err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return fmt.Errorf("failed to ping registry %s: %w", hostname, reghttp.HTTPError(resp.HTTPResponse().StatusCode))
	}

	return nil
}
