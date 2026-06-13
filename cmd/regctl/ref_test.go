// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
