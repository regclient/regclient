package platform

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		name               string
		host, target, prev Platform
		expectMatch        bool
		expectCompat       bool
		expectBetter       bool
	}{
		{
			name:         "linux match",
			host:         Platform{OS: "linux", Architecture: "amd64"},
			target:       Platform{OS: "linux", Architecture: "amd64"},
			expectMatch:  true,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "linux arch",
			host:         Platform{OS: "linux", Architecture: "amd64"},
			target:       Platform{OS: "linux", Architecture: "arm64"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "linux normalized",
			host:         Platform{OS: "linux", Architecture: "arm64"},
			target:       Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			expectMatch:  true,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "linux variant higher",
			host:         Platform{OS: "linux", Architecture: "arm", Variant: "v6"},
			target:       Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			prev:         Platform{OS: "linux", Architecture: "arm", Variant: "v5"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "linux variant lower",
			host:         Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			target:       Platform{OS: "linux", Architecture: "arm", Variant: "v5"},
			prev:         Platform{OS: "linux", Architecture: "arm", Variant: "v6"},
			expectMatch:  false,
			expectCompat: true,
			expectBetter: false,
		},
		{
			name:         "linux variant prev undef",
			host:         Platform{OS: "linux", Architecture: "amd64", Variant: "v3"},
			target:       Platform{OS: "linux", Architecture: "amd64", Variant: "v2"},
			prev:         Platform{OS: "linux", Architecture: "amd64", Variant: ""},
			expectMatch:  false,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "linux variant host undef",
			host:         Platform{OS: "linux", Architecture: "amd64", Variant: ""},
			target:       Platform{OS: "linux", Architecture: "amd64", Variant: "v2"},
			prev:         Platform{OS: "linux", Architecture: "amd64", Variant: "v1"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "linux variant target undef",
			host:         Platform{OS: "linux", Architecture: "amd64", Variant: "v3"},
			target:       Platform{OS: "linux", Architecture: "amd64", Variant: ""},
			prev:         Platform{OS: "linux", Architecture: "amd64", Variant: "v2"},
			expectMatch:  false,
			expectCompat: true,
			expectBetter: false,
		},
		{
			name:         "windows match",
			host:         Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			target:       Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			prev:         Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.1224"},
			expectMatch:  true,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "windows patch",
			host:         Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2014"},
			target:       Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			prev:         Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2004"},
			expectMatch:  true,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "windows minor",
			host:         Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.14393.4583"},
			target:       Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "darwin compatible",
			host:         Platform{OS: "darwin", Architecture: "amd64", Variant: "v2"},
			target:       Platform{OS: "linux", Architecture: "amd64"},
			prev:         Platform{OS: "darwin", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: true,
			expectBetter: false,
		},
		{
			name:         "darwin target",
			host:         Platform{OS: "linux", Architecture: "amd64"},
			target:       Platform{OS: "darwin", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "freebsd host",
			host:         Platform{OS: "freebsd", Architecture: "amd64"},
			target:       Platform{OS: "linux", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "freebsd match",
			host:         Platform{OS: "freebsd", Architecture: "amd64", Variant: "v2"},
			target:       Platform{OS: "freebsd", Architecture: "amd64", Variant: "v2"},
			prev:         Platform{OS: "freebsd", Architecture: "amd64"},
			expectMatch:  true,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "freebsd target",
			host:         Platform{OS: "linux", Architecture: "amd64"},
			target:       Platform{OS: "freebsd", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
		{
			name:         "windows compatible",
			host:         Platform{OS: "windows", Architecture: "amd64"},
			target:       Platform{OS: "linux", Architecture: "amd64"},
			prev:         Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2004"},
			expectMatch:  false,
			expectCompat: true,
			expectBetter: false,
		},
		{
			name:         "other",
			host:         Platform{OS: "other", Architecture: "amd64", Variant: "42"},
			target:       Platform{OS: "other", Architecture: "amd64", Variant: "42"},
			expectMatch:  true,
			expectCompat: true,
			expectBetter: true,
		},
		{
			name:         "other variant",
			host:         Platform{OS: "other", Architecture: "amd64", Variant: "42"},
			target:       Platform{OS: "other", Architecture: "amd64", Variant: "45"},
			expectMatch:  false,
			expectCompat: false,
			expectBetter: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Match(tt.host, tt.target)
			if result != tt.expectMatch {
				t.Errorf("unexpected match, result: %v, host: %v, target: %v", result, tt.host, tt.target)
			}
			result = Compatible(tt.host, tt.target)
			if result != tt.expectCompat {
				t.Errorf("unexpected compatible, result: %v, host: %v, target: %v", result, tt.host, tt.target)
			}
			comp := NewCompare(tt.host)
			result = comp.Better(tt.target, tt.prev)
			if result != tt.expectBetter {
				t.Errorf("unexpected better, result: %v, host: %v, target: %v, prev: %v", result, tt.host, tt.target, tt.prev)
			}
		})
	}
}
