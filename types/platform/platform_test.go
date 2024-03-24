package platform

import (
	"errors"
	"testing"

	"github.com/regclient/regclient/types/errs"
)

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
			name:    "wildcard",
			parse:   "linux/*",
			wantErr: errs.ErrParsingFailed,
		},
		{
			name:    "unsupported arg",
			parse:   "linux,amd64",
			wantErr: errs.ErrParsingFailed,
		},
		{
			name:  "linux amd64",
			parse: "linux/amd64",
			goal:  Platform{OS: "linux", Architecture: "amd64"},
		},
		{
			name:  "linux amd64 v1",
			parse: "linux/amd64/v1",
			goal:  Platform{OS: "linux", Architecture: "amd64"},
		},
		{
			name:  "linux amd64 v3",
			parse: "linux/amd64/v3",
			goal:  Platform{OS: "linux", Architecture: "amd64", Variant: "v3"},
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
			goal:  Platform{OS: "linux", Architecture: "arm64"},
		},
		{
			name:  "linux armel",
			parse: "linux/armel",
			goal:  Platform{OS: "linux", Architecture: "arm", Variant: "v6"},
		},
		{
			name:  "linux armhf",
			parse: "linux/armhf",
			goal:  Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
		},
		{
			name:  "linux aarch64",
			parse: "linux/aarch64",
			goal:  Platform{OS: "linux", Architecture: "arm64"},
		},
		{
			name:  "linux 386",
			parse: "linux/386",
			goal:  Platform{OS: "linux", Architecture: "386"},
		},
		{
			name:  "linux i386",
			parse: "linux/i386",
			goal:  Platform{OS: "linux", Architecture: "386"},
		},
		{
			name:  "linux",
			parse: "linux",
			goal:  linuxGoal,
		},
		{
			name:  "macos amd64",
			parse: "macos/amd64",
			goal:  Platform{OS: "darwin", Architecture: "amd64"},
		},
		{
			name:  "darwin arm64",
			parse: "darwin/arm64",
			goal:  Platform{OS: "darwin", Architecture: "arm64"},
		},
		{
			name:  "windows amd64 with version",
			parse: "windows/amd64,osver=10.0.17763.4974",
			goal:  Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.17763.4974"},
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
			if p.OS != tt.goal.OS || p.Architecture != tt.goal.Architecture || p.Variant != tt.goal.Variant || p.OSVersion != tt.goal.OSVersion {
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
