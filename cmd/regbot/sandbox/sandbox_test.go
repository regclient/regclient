package sandbox

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"

	"github.com/regclient/regclient"
	rcConfig "github.com/regclient/regclient/config"
)

func TestSandbox(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	boolT := true
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../../testdata",
		},
		API: oConfig.ConfigAPI{
			DeleteEnabled: &boolT,
		},
	})
	ts := httptest.NewServer(regHandler)
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	t.Cleanup(func() {
		ts.Close()
		_ = regHandler.Close()
	})
	rcHosts := []rcConfig.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      rcConfig.TLSDisabled,
		},
		{
			Name:     "registry.example.org",
			Hostname: tsHost,
			TLS:      rcConfig.TLSDisabled,
		},
	}
	// replace regclient with one configured for test hosts
	rc := regclient.New(
		regclient.WithConfigHost(rcHosts...),
	)
	s := New("test", WithContext(ctx), WithRegClient(rc))

	tt := []struct {
		name      string
		script    string
		expectErr error
	}{
		{
			name:   "Empty",
			script: "",
		},
		{
			name: "List tags",
			script: `
				tags = tag.ls("registry.example.org/testrepo")
				for k, t in pairs(tags) do
					if t == "v2" then
						return
					end
				end
				error("v2 tag was seen in the listing")
			`,
		},
		{
			name: "Find duplicate digest to mirror tag",
			script: `
				target = manifest.descriptor("registry.example.org/testrepo:mirror").Digest
				tags = tag.ls("registry.example.org/testrepo")
				for k, t in pairs(tags) do
					if t ~= "mirror" and target == manifest.descriptor("registry.example.org/testrepo:" .. t).Digest then
						log("found matching tag to mirror: " .. t)
						return
					end
				end
				error("did not find matching tag to mirror tag")
			`,
		},
		{
			name: "Manifest descriptor",
			script: `
				m = manifest.getList("registry.example.org/testrepo:v1")
				if m:descriptor().MediaType ~= "application/vnd.oci.image.index.v1+json" then
					error("v1 media type is " .. m:descriptor().MediaType)
				end
				if not string.match(m:descriptor().Digest, "^sha256:") then
					error("v1 digest is " .. m:descriptor().Digest)
				end
				-- get the descriptor directly
				desc = manifest.descriptor("registry.example.org/testrepo:v2")
				if desc.MediaType ~= "application/vnd.oci.image.index.v1+json" then
					error("v2 media type is " .. desc.MediaType)
				end
			`,
		},
		{
			name: "Get config",
			script: `
				m = manifest.get("registry.example.org/testrepo:v1", "linux/amd64")
				ic = image.config(m)
				if ic.Config.Labels["version"] ~= "1" then
					error("version label missing/invalid: " .. ic.Config.Labels["version"])
				end`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := s.RunScript(tc.script)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("process did not fail")
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error on process: %v, expected %v", err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error on process: %v", err)
			}
		})
	}
}
