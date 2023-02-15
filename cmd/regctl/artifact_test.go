package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/regclient/regclient/types"
)

func TestArtifactTree(t *testing.T) {
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"artifact", "tree"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"artifact", "tree", "invalid*ref"},
			expectErr: fmt.Errorf("invalid reference \"%s\"", "invalid*ref"),
		},
		{
			name:      "Missing manifest",
			args:      []string{"artifact", "tree", "ocidir://../../testdata/testrepo:missing"},
			expectErr: types.ErrNotFound,
		},
		{
			name:        "No referrers",
			args:        []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v1"},
			expectOut:   "Children",
			outContains: true,
		},
		{
			name:        "Referrers",
			args:        []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v2"},
			expectOut:   "Referrers",
			outContains: true,
		},
		{
			name:        "Filter",
			args:        []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v2", "--filter-artifact-type", "application/example.sbom"},
			expectOut:   "application/example.sbom",
			outContains: true,
		},
		{
			name:      "Filter and Format",
			args:      []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v2", "--filter-artifact-type", "application/example.sbom", "--format", "{{ ( index .Referrer 0 ).ArtifactType }}"},
			expectOut: "application/example.sbom",
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
