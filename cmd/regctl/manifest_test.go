package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/regclient/regclient/types"
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
			expectErr: fmt.Errorf("invalid reference \"%s\"", "invalid*ref"),
		},
		{
			name:      "Missing manifest",
			args:      []string{"manifest", "head", "ocidir://../../testdata/testrepo:missing"},
			expectErr: types.ErrNotFound,
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
			expectErr: ErrNotFound,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, tc.args...)
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
