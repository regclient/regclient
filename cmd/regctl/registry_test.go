package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
)

func TestRegistry(t *testing.T) {
	// t.Parallel() // this is not parallel due to environment variable settings
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
	})
	tsGood := httptest.NewServer(regHandler)
	tsGoodURL, _ := url.Parse(tsGood.URL)
	tsGoodHost := tsGoodURL.Host
	tsBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request was made to the bad host")
		w.WriteHeader(http.StatusForbidden)
	}))
	tsBadURL, _ := url.Parse(tsBad.URL)
	tsBadHost := tsBadURL.Host
	t.Cleanup(func() {
		tsGood.Close()
		tsBad.Close()
		_ = regHandler.Close()
	})
	tempDir := t.TempDir()
	t.Setenv(ConfigEnv, filepath.Join(tempDir, "config.json"))
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		// set a good host
		{
			name:        "set no TLS",
			args:        []string{"registry", "set", tsGoodHost, "--tls", "disabled"},
			expectOut:   "",
			outContains: false,
		},
		// set a bad host but skip the check
		{
			name:        "set without check",
			args:        []string{"registry", "set", tsBadHost, "--tls", "disabled", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		// query the good host
		{
			name:        "query good host",
			args:        []string{"registry", "config", tsGoodHost},
			expectOut:   `"tls": "disabled",`,
			outContains: true,
		},
		// query the bad host
		{
			name:        "query bad host",
			args:        []string{"registry", "config", tsBadHost},
			expectOut:   `"tls": "disabled",`,
			outContains: true,
		},
		// login to good and bad hosts
		{
			name:        "login good host",
			args:        []string{"registry", "login", tsGoodHost, "-u", "testgooduser", "-p", "testpass"},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "login bad host",
			args:        []string{"registry", "login", tsBadHost, "-u", "testbaduser", "-p", "testpass", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		// query good and bad to ensure user is set
		{
			name:      "query good host",
			args:      []string{"registry", "config", tsGoodHost, "--format", "{{.User}}"},
			expectOut: `testgooduser`,
		},
		{
			name:      "query bad host",
			args:      []string{"registry", "config", tsBadHost, "--format", "{{.User}}"},
			expectOut: `testbaduser`,
		},
		// logout
		{
			name:        "logout good host",
			args:        []string{"registry", "logout", tsGoodHost},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "logout bad host",
			args:        []string{"registry", "logout", tsBadHost},
			expectOut:   "",
			outContains: false,
		},
		// verify logout
		{
			name:      "check logout on good host",
			args:      []string{"registry", "config", tsGoodHost, "--format", "{{.User}}"},
			expectOut: ``,
		},
		{
			name:      "check logout on bad host",
			args:      []string{"registry", "config", tsBadHost, "--format", "{{.User}}"},
			expectOut: ``,
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
				t.Errorf("returned unexpected error: %v", err)
				return
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}
