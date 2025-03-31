package main

import (
	"errors"
	"fmt"
	"io/fs"
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
)

func TestTagList(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"tag", "ls"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"tag", "ls", "invalid*ref"},
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:      "Missing repo",
			args:      []string{"tag", "ls", "ocidir://../../testdata/test-missing"},
			expectErr: fs.ErrNotExist,
		},
		{
			name:        "List tags",
			args:        []string{"tag", "ls", "ocidir://../../testdata/testrepo"},
			expectOut:   "v1\nv2\nv3",
			outContains: true,
		},
		{
			name:        "List tags filtered",
			args:        []string{"tag", "ls", "--include", "sha256.*", "--exclude", ".*\\.meta", "ocidir://../../testdata/testrepo"},
			expectOut:   "sha256-",
			outContains: true,
		},
		{
			name:        "List tags limited",
			args:        []string{"tag", "ls", "--include", "v.*", "--limit", "5", "ocidir://../../testdata/testrepo"},
			expectOut:   "v1\nv2\nv3",
			outContains: true,
		},
		{
			name:        "List tags formatted",
			args:        []string{"tag", "ls", "--format", "raw", "ocidir://../../testdata/testrepo"},
			expectOut:   "application/vnd.oci.image.index.v1+json",
			outContains: true,
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

func TestTagRm(t *testing.T) {
	t.Parallel()
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
	dig := digest.Canonical.FromString("test digest").String()

	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"tag", "rm"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name:      "Delete digest",
			args:      []string{"tag", "rm", tsHost + "/testrepo@" + dig},
			expectErr: errs.ErrMissingTag,
		},
		{
			name: "Delete v1",
			args: []string{"tag", "rm", tsHost + "/testrepo:v1"},
		},
		{
			name:      "Delete missing",
			args:      []string{"tag", "rm", tsHost + "/testrepo:missing"},
			expectErr: errs.ErrNotFound,
		},
		{
			name: "Delete missing with ignore missing",
			args: []string{"tag", "rm", tsHost + "/testrepo:missing", "--ignore-missing"},
		},
		{
			name:      "Delete tls error with ignore missing",
			args:      []string{"tag", "rm", "invalid-tls." + tsHost + "/testrepo:missing", "--ignore-missing"},
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
