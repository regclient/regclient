package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/regclient/regclient/types"
)

func TestArtifactGet(t *testing.T) {
	saveArtifactOpts := artifactOpts
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"artifact", "get"},
			expectErr: fmt.Errorf("either a reference or subject must be provided"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"artifact", "get", "invalid*ref"},
			expectErr: types.ErrInvalidReference,
		},
		{
			name:      "Missing manifest",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:missing"},
			expectErr: types.ErrNotFound,
		},
		{
			name:      "By Manifest",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:a1"},
			expectOut: "eggs",
		},
		{
			name:      "By Subject",
			args:      []string{"artifact", "get", "--subject", "ocidir://../../testdata/testrepo:v2", "--filter-artifact-type", "application/example.sbom"},
			expectOut: "eggs",
		},
		{
			name:      "By Index",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:ai", "--filter-annotation", "type=sbom"},
			expectOut: "eggs",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, tc.args...)
			artifactOpts = saveArtifactOpts
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

func TestArtifactList(t *testing.T) {
	saveArtifactOpts := artifactOpts
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"artifact", "list"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"artifact", "list", "invalid*ref"},
			expectErr: types.ErrInvalidReference,
		},
		{
			name:      "Missing manifest",
			args:      []string{"artifact", "list", "ocidir://../../testdata/testrepo:missing"},
			expectErr: types.ErrNotFound,
		},
		{
			name:        "No referrers",
			args:        []string{"artifact", "list", "ocidir://../../testdata/testrepo:v1"},
			expectOut:   "Referrers:",
			outContains: true,
		},
		{
			name:        "Referrers",
			args:        []string{"artifact", "list", "ocidir://../../testdata/testrepo:v2"},
			expectOut:   "Referrers:",
			outContains: true,
		},
		{
			name:        "With Digest Tags",
			args:        []string{"artifact", "list", "ocidir://../../testdata/testrepo:v2", "--digest-tags"},
			expectOut:   "Referrers:",
			outContains: true,
		},
		{
			name:        "Filter",
			args:        []string{"artifact", "list", "ocidir://../../testdata/testrepo:v2", "--filter-artifact-type", "application/example.sbom"},
			expectOut:   "application/example.sbom",
			outContains: true,
		},
		{
			name:      "Filter and Format",
			args:      []string{"artifact", "list", "ocidir://../../testdata/testrepo:v2", "--filter-artifact-type", "application/example.sbom", "--format", "{{ ( index .Descriptors 0 ).ArtifactType }}"},
			expectOut: "application/example.sbom",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, tc.args...)
			artifactOpts = saveArtifactOpts
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

func TestArtifactPut(t *testing.T) {
	testDir := t.TempDir()
	testData := []byte("hello world")
	testConfName := filepath.Join(testDir, "exConf")
	err := os.WriteFile(testConfName, []byte(`{"hello": "world"}`), 0600)
	if err != nil {
		t.Errorf("failed creating test conf: %v", err)
		return
	}
	saveArtifactOpts := artifactOpts

	tt := []struct {
		name        string
		args        []string
		in          []byte
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"artifact", "put"},
			expectErr: fmt.Errorf("either a reference or subject must be provided"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"artifact", "put", "invalid*ref"},
			expectErr: types.ErrInvalidReference,
		},
		{
			name: "Put artifact",
			args: []string{"artifact", "put", "ocidir://" + testDir + ":put"},
			in:   testData,
		},
		{
			name: "Put artifact example AT",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "ocidir://" + testDir + ":put-example-at"},
			in:   testData,
		},
		{
			name: "Put artifact example conf",
			args: []string{"artifact", "put", "--config-type", "application/vnd.example", "ocidir://" + testDir + ":put-example-conf"},
			in:   testData,
		},
		{
			name: "Put artifact example conf data",
			args: []string{"artifact", "put", "--artifact-type", "", "--config-type", "application/vnd.example", "--config-file", testConfName, "ocidir://" + testDir + ":put-example-conf-data"},
			in:   testData,
		},
		{
			name: "Put subject",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--config-file", "", "--subject", "ocidir://" + testDir + ":put-example-at"},
			in:   testData,
		},
		{
			name: "Put create index",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--config-file", "", "--annotation", "test=a", "--platform", "linux/amd64", "--index", "ocidir://" + testDir + ":index"},
			in:   testData,
		},
		{
			name: "Put append index",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--config-file", "", "--annotation", "test=b", "--platform", "linux/arm64", "--index", "ocidir://" + testDir + ":index"},
			in:   testData,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.in != nil {
				origIn := rootCmd.InOrStdin()
				defer rootCmd.SetIn(origIn)
				rootCmd.SetIn(bytes.NewBuffer(tc.in))
			}
			out, err := cobraTest(t, tc.args...)
			artifactOpts = saveArtifactOpts
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
	// reset flags
	artifactOpts.artifactConfig = ""
	artifactOpts.subject = ""
}

func TestArtifactTree(t *testing.T) {
	saveArtifactOpts := artifactOpts
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
			expectErr: types.ErrInvalidReference,
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
			name:      "Loop",
			args:      []string{"artifact", "tree", "ocidir://../../testdata/testrepo:loop"},
			expectErr: ErrLoopEncountered,
		},
		{
			name:        "Referrers",
			args:        []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v2"},
			expectOut:   "Referrers",
			outContains: true,
		},
		{
			name:        "With Digest Tags",
			args:        []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v2", "--digest-tags"},
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
			artifactOpts = saveArtifactOpts
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
