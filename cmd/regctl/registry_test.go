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

	"github.com/regclient/regclient/types/errs"
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
	tsUnauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	tsUnauthURL, _ := url.Parse(tsUnauth.URL)
	tsUnauthHost := tsUnauthURL.Host
	t.Cleanup(func() {
		tsGood.Close()
		tsBad.Close()
		tsUnauth.Close()
		_ = regHandler.Close()
	})
	tsExampleHost := "registry.example.org"
	tempDir := t.TempDir()
	t.Setenv(ConfigEnv, filepath.Join(tempDir, "config.json"))
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "whoami to an unknown server",
			args:      []string{"registry", "whoami", tsGoodHost},
			expectErr: errs.ErrNoLogin,
		},
		// disable tls
		{
			name:        "set no TLS",
			args:        []string{"registry", "set", tsGoodHost, "--tls", "disabled"},
			expectOut:   "",
			outContains: false,
		},
		{
			name:      "set no TLS on unauth host",
			args:      []string{"registry", "set", tsUnauthHost, "--tls", "disabled"},
			expectErr: errs.ErrHTTPUnauthorized,
		},
		{
			name:        "set without check",
			args:        []string{"registry", "set", tsBadHost, "--tls", "disabled", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		// set and unset config on example
		{
			name:        "set example",
			args:        []string{"registry", "set", tsExampleHost, "--cred-helper", "docker-credential-example", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "set example",
			args:        []string{"registry", "set", tsExampleHost, "--cred-helper", "", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		// query the config change
		{
			name:        "query good host",
			args:        []string{"registry", "config", tsGoodHost},
			expectOut:   `"tls": "disabled",`,
			outContains: true,
		},
		{
			name:      "whoami to an known server without logging in",
			args:      []string{"registry", "whoami", tsGoodHost},
			expectErr: errs.ErrNoLogin,
		},
		{
			name:        "query unauth host",
			args:        []string{"registry", "config", tsUnauthHost},
			expectOut:   `"tls": "disabled",`,
			outContains: true,
		},
		{
			name:        "query bad host",
			args:        []string{"registry", "config", tsBadHost},
			expectOut:   `"tls": "disabled",`,
			outContains: true,
		},
		{
			name:        "query example",
			args:        []string{"registry", "config", tsExampleHost},
			expectOut:   "No configuration found for registry",
			outContains: true,
		},
		// login
		{
			name:        "login good host",
			args:        []string{"registry", "login", tsGoodHost, "-u", "testgooduser", "-p", "testpass"},
			expectOut:   "",
			outContains: false,
		},
		{
			name:      "login unauth host",
			args:      []string{"registry", "login", tsUnauthHost, "-u", "testunauthuser", "-p", "testpass"},
			expectErr: errs.ErrHTTPUnauthorized,
		},
		{
			name:        "login bad host",
			args:        []string{"registry", "login", tsBadHost, "-u", "testbaduser", "-p", "testpass", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "login example",
			args:        []string{"registry", "login", tsExampleHost, "-u", "testexample", "-p", "testpass", "--skip-check"},
			expectOut:   "",
			outContains: false,
		},
		// query for user
		{
			name:      "query good host",
			args:      []string{"registry", "config", tsGoodHost, "--format", "{{.User}}"},
			expectOut: `testgooduser`,
		},
		{
			name:      "whoami to an known server",
			args:      []string{"registry", "whoami", tsGoodHost},
			expectOut: "testgooduser",
		},
		{
			name:      "query unauth host",
			args:      []string{"registry", "config", tsUnauthHost, "--format", "{{.User}}"},
			expectOut: `testunauthuser`,
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
			name:        "logout unauth host",
			args:        []string{"registry", "logout", tsUnauthHost},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "logout bad host",
			args:        []string{"registry", "logout", tsBadHost},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "logout example",
			args:        []string{"registry", "logout", tsExampleHost},
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
			name:      "whoami to an known server after logout",
			args:      []string{"registry", "whoami", tsGoodHost},
			expectErr: errs.ErrNoLogin,
		},
		{
			name:      "check logout on unauth host",
			args:      []string{"registry", "config", tsUnauthHost, "--format", "{{.User}}"},
			expectOut: ``,
		},
		{
			name:      "check logout on bad host",
			args:      []string{"registry", "config", tsBadHost, "--format", "{{.User}}"},
			expectOut: ``,
		},
		{
			name:        "check logout on example",
			args:        []string{"registry", "config", tsExampleHost},
			expectOut:   "No configuration found for registry",
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
