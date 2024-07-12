package main

import (
	"strings"
	"testing"
)

func TestRef(t *testing.T) {
	tt := []struct {
		name        string
		cmd         []string
		expectOut   string
		outContains bool
	}{
		{
			name:      "default",
			cmd:       []string{"ref", "nginx"},
			expectOut: "docker.io/library/nginx:latest",
		},
		{
			name:      "get registry",
			cmd:       []string{"ref", "ghcr.io/regclient/regctl:v0.3", "--format", `{{.Registry}}`},
			expectOut: "ghcr.io",
		},
		{
			name:      "get repository",
			cmd:       []string{"ref", "ghcr.io/regclient/regctl:v0.3", "--format", `{{.Repository}}`},
			expectOut: "regclient/regctl",
		},
		{
			name:      "get tag",
			cmd:       []string{"ref", "ghcr.io/regclient/regctl:v0.3", "--format", `{{.Tag}}`},
			expectOut: "v0.3",
		},
		{
			name:      "get digest",
			cmd:       []string{"ref", "ghcr.io/regclient/regctl:v0.3", "--format", `{{.Digest}}`},
			expectOut: "",
		},
		{
			name:      "get ocidir path",
			cmd:       []string{"ref", "ocidir://regclient/regctl:v0.3", "--format", `{{.Path}}`},
			expectOut: "regclient/regctl",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.cmd...)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}
