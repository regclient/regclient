package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

func TestManifestHead(t *testing.T) {
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"manifest", "head"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"manifest", "head", "invalid*ref"},
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:      "Missing manifest",
			args:      []string{"manifest", "head", "ocidir://../../testdata/testrepo:missing"},
			expectErr: errs.ErrNotFound,
		},
		{
			name:        "Digest",
			args:        []string{"manifest", "head", "ocidir://../../testdata/testrepo:v1"},
			expectOut:   "sha256:",
			outContains: true,
		},
		{
			name:        "Platform amd64",
			args:        []string{"manifest", "head", "ocidir://../../testdata/testrepo:v1", "--platform", "linux/amd64"},
			expectOut:   "sha256:",
			outContains: true,
		},
		{
			name:      "Platform unknown",
			args:      []string{"manifest", "head", "ocidir://../../testdata/testrepo:v1", "--platform", "linux/unknown"},
			expectErr: errs.ErrNotFound,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.args...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("did not receive expected error: %v", tc.expectErr)
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, received %v, expected %v", err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("returned unexpected error: %v", err)
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}

func TestManifestRm(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boolT := true
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../testdata",
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
	rcOpts := []regclient.Opt{
		regclient.WithConfigHost(
			config.Host{
				Name: tsHost,
				TLS:  config.TLSDisabled,
			},
			config.Host{
				Name:     "invalid-tls." + tsHost,
				Hostname: tsHost,
				TLS:      config.TLSEnabled,
			},
		),
		regclient.WithRegOpts(reg.WithDelay(time.Millisecond*10, time.Millisecond*100), reg.WithRetryLimit(2)),
	}
	rb, err := ref.New("ocidir://../../testdata/testrepo:v3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rc := regclient.New(rcOpts...)
	mb, err := rc.ManifestHead(ctx, rb, regclient.WithManifestRequireDigest())
	if err != nil {
		t.Fatalf("failed to head ref %s: %v", rb.CommonName(), err)
	}
	dig := mb.GetDescriptor().Digest.String()
	missing := digest.Canonical.FromString("missing").String()

	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"manifest", "rm"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name: "Delete v3 by digest",
			args: []string{"manifest", "rm", tsHost + "/testrepo@" + dig},
		},
		{
			name:      "Delete v1 without deref",
			args:      []string{"manifest", "rm", tsHost + "/testrepo:v1"},
			expectErr: errs.ErrMissingDigest,
		},
		{
			name: "Delete v1 with deref",
			args: []string{"manifest", "rm", tsHost + "/testrepo:v1", "--force-tag-dereference"},
		},
		{
			name:      "Delete missing digest",
			args:      []string{"manifest", "rm", tsHost + "/testrepo@" + missing, "--force-tag-dereference"},
			expectErr: errs.ErrNotFound,
		},
		{
			name: "Delete missing digest with ignore missing",
			args: []string{"manifest", "rm", tsHost + "/testrepo@" + missing, "--ignore-missing"},
		},
		{
			name:      "Delete missing tag with deref",
			args:      []string{"manifest", "rm", tsHost + "/testrepo:missing", "--force-tag-dereference"},
			expectErr: errs.ErrNotFound,
		},
		{
			name: "Delete missing tag with deref and ignore missing",
			args: []string{"manifest", "rm", tsHost + "/testrepo:missing", "--force-tag-dereference", "--ignore-missing"},
		},
		{
			name:      "Delete tls error with ignore missing",
			args:      []string{"manifest", "rm", "invalid-tls." + tsHost + "/testrepo@" + missing, "--ignore-missing"},
			expectErr: http.ErrSchemeMismatch,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, &cobraTestOpts{rcOpts: rcOpts}, tc.args...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("did not receive expected error: %v", tc.expectErr)
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, received %v, expected %v", err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("returned unexpected error: %v", err)
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}
