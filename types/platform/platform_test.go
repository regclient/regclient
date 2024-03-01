package platform

import (
	"errors"
	"testing"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		name         string
		a, b         Platform
		expectMatch  bool
		expectCompat bool
	}{
		{
			name:         "linux match",
			a:            Platform{OS: "linux", Architecture: "amd64"},
			b:            Platform{OS: "linux", Architecture: "amd64"},
			expectMatch:  true,
			expectCompat: true,
		},
		{
			name:         "linux arch",
			a:            Platform{OS: "linux", Architecture: "amd64"},
			b:            Platform{OS: "linux", Architecture: "arm64"},
			expectMatch:  false,
			expectCompat: false,
		},
		{
			name:         "linux normalized",
			a:            Platform{OS: "linux", Architecture: "arm64"},
			b:            Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			expectMatch:  true,
			expectCompat: true,
		},
		{
			name:         "linux variant",
			a:            Platform{OS: "linux", Architecture: "arm", Variant: "v6"},
			b:            Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			expectMatch:  false,
			expectCompat: false,
		},
		{
			name:         "windows match",
			a:            Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			b:            Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			expectMatch:  true,
			expectCompat: true,
		},
		{
			name:         "windows patch",
			a:            Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2014"},
			b:            Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			expectMatch:  true,
			expectCompat: true,
		},
		{
			name:         "windows minor",
			a:            Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.14393.4583"},
			b:            Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			expectMatch:  false,
			expectCompat: false,
		},
		{
			name:         "darwin compatible",
			a:            Platform{OS: "darwin", Architecture: "amd64"},
			b:            Platform{OS: "linux", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: true,
		},
		{
			name:         "darwin target",
			a:            Platform{OS: "linux", Architecture: "amd64"},
			b:            Platform{OS: "darwin", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: false,
		},
		{
			name:         "windows compatible",
			a:            Platform{OS: "windows", Architecture: "amd64"},
			b:            Platform{OS: "linux", Architecture: "amd64"},
			expectMatch:  false,
			expectCompat: true,
		},
		{
			name:         "other",
			a:            Platform{OS: "other", Architecture: "amd64", Variant: "42"},
			b:            Platform{OS: "other", Architecture: "amd64", Variant: "42"},
			expectMatch:  true,
			expectCompat: true,
		},
		{
			name:         "other variant",
			a:            Platform{OS: "other", Architecture: "amd64", Variant: "42"},
			b:            Platform{OS: "other", Architecture: "amd64", Variant: "45"},
			expectMatch:  false,
			expectCompat: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Match(tt.a, tt.b)
			if result != tt.expectMatch {
				t.Errorf("unexpected match, result: %v, a: %v, b: %v", result, tt.a, tt.b)
			}
			result = Compatible(tt.a, tt.b)
			if result != tt.expectCompat {
				t.Errorf("unexpected compatible, result: %v, a: %v, b: %v", result, tt.a, tt.b)
			}
		})
	}
}

func TestPlatformParse(t *testing.T) {
	platLocal := Local()
	linuxGoal := Platform{OS: "linux"}
	if Compatible(Platform{OS: platLocal.OS}, Platform{OS: "linux"}) {
		linuxGoal.Architecture = platLocal.Architecture
		linuxGoal.Variant = platLocal.Variant
	}
	winGoal := Platform{OS: "windows"}
	if Compatible(Platform{OS: platLocal.OS}, Platform{OS: "windows"}) {
		winGoal.Architecture = platLocal.Architecture
		winGoal.Variant = platLocal.Variant
		winGoal.OSVersion = platLocal.OSVersion
	}
	tests := []struct {
		name    string
		parse   string
		goal    Platform
		wantErr error
	}{
		{
			name:  "linux amd64",
			parse: "linux/amd64",
			goal:  Platform{OS: "linux", Architecture: "amd64"},
		},
		{
			name:  "linux arm/v5",
			parse: "linux/arm/v5",
			goal:  Platform{OS: "linux", Architecture: "arm", Variant: "v5"},
		},
		{
			name:  "linux arm/v6",
			parse: "linux/arm/v6",
			goal:  Platform{OS: "linux", Architecture: "arm", Variant: "v6"},
		},
		{
			name:  "linux arm/v7",
			parse: "linux/arm/v7",
			goal:  Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
		},
		{
			name:  "linux arm64/v8",
			parse: "linux/arm64",
			goal:  Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
		},
		{
			name:  "linux",
			parse: "linux",
			goal:  linuxGoal,
		},
		{
			name:  "windows amd64",
			parse: "windows/amd64/v2",
			goal:  Platform{OS: "windows", Architecture: "amd64", Variant: "v2"},
		},
		{
			name:  "windows",
			parse: "windows",
			goal:  winGoal,
		},
		{
			name:  "local",
			parse: "local",
			goal:  platLocal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.parse)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Errorf("unexpected error, want %v, received %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Match(p, tt.goal) {
				t.Errorf("platform did not match, want %v, received %v", tt.goal, p)
			}
		})
	}
}

func TestPlatformString(t *testing.T) {
	tests := []struct {
		name string
		goal string
		p    Platform
	}{
		{
			name: "empty",
			p:    Platform{},
			goal: "unknown",
		},
		{
			name: "linux/amd64",
			p:    Platform{OS: "linux", Architecture: "amd64"},
			goal: "linux/amd64",
		},
		{
			name: "linux/arm64",
			p:    Platform{OS: "linux", Architecture: "arm64"},
			goal: "linux/arm64",
		},
		{
			name: "linux/arm64/v8",
			p:    Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			goal: "linux/arm64",
		},
		{
			name: "linux/arm/v7",
			p:    Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			goal: "linux/arm/v7",
		},
		{
			name: "windows/amd64",
			p:    Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.2114"},
			goal: "windows/amd64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.p.String()
			if result != tt.goal {
				t.Errorf("string did not match, expected %s, received %s", tt.goal, result)
			}
		})
	}
}
