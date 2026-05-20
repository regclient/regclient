package regnet

import (
	"net/url"
	"testing"

	"github.com/regclient/regclient/types/errs"
)

func TestAllowRedirect(t *testing.T) {
	tt := []struct {
		name      string
		src, dest url.URL
		expect    error
	}{
		{
			name:   "http to https",
			src:    urlMustParse(t, "http://registry.example.org"),
			dest:   urlMustParse(t, "https://token.example.org"),
			expect: nil,
		},
		{
			name:   "https to http",
			src:    urlMustParse(t, "https://registry.example.org"),
			dest:   urlMustParse(t, "http://token.example.org"),
			expect: errs.ErrHTTPRedirectRefused,
		},
		{
			name:   "external to local",
			src:    urlMustParse(t, "http://10.0.0.1"),
			dest:   urlMustParse(t, "http://127.0.0.5"),
			expect: errs.ErrHTTPRedirectRefused,
		},
		{
			name:   "local to external",
			src:    urlMustParse(t, "http://127.0.0.5"),
			dest:   urlMustParse(t, "http://10.0.0.1"),
			expect: nil,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := AllowRedirect(tc.src, tc.dest)
			if (result == nil && tc.expect != nil) || (result != nil && tc.expect == nil) {
				t.Errorf("expected %v, received %v", tc.expect, result)
			}
		})
	}
}

func urlMustParse(t *testing.T, s string) url.URL {
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse url %s: %v", s, err)
	}
	return *u
}

func TestIsLocal(t *testing.T) {
	tt := []struct {
		host   string
		expect bool
	}{
		{
			host:   "127.0.0.2",
			expect: true,
		},
		{
			host:   "::1",
			expect: true,
		},
		{
			host:   "[::1]:8080",
			expect: true,
		},
		{
			host:   "localhost.",
			expect: true,
		},
		{
			host:   "0.0.0.0:8080",
			expect: true,
		},
		{
			host:   "10.0.0.1",
			expect: false,
		},
		{
			host:   "regclient.org",
			expect: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.host, func(t *testing.T) {
			result := IsLocal(tc.host)
			if result != tc.expect {
				t.Errorf("expected %t, received %t", tc.expect, result)
			}
		})
	}
}
