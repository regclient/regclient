package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/regclient/regclient/types/errs"
)

func TestArtifactGet(t *testing.T) {
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
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:      "Missing manifest",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:missing"},
			expectErr: errs.ErrNotFound,
		},
		{
			name:      "By Manifest",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:a1"},
			expectOut: "eggs",
		},
		{
			name:      "By Manifest filter layer media type",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:a3", "--file-media-type", "application/example.layer.2"},
			expectOut: "2",
		},
		{
			name:      "By Manifest filter layer filename",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:a3", "--file", "layer3.txt"},
			expectOut: "3",
		},
		{
			name:      "By Manifest filter layer media type missing",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:a3", "--file-media-type", "application/example.missing"},
			expectErr: errs.ErrNotFound,
		},
		{
			name:      "By Manifest filter layer filename missing",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:a3", "--file", "missing.txt"},
			expectErr: errs.ErrNotFound,
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
		{
			name:      "Get Config",
			args:      []string{"artifact", "get", "ocidir://../../testdata/testrepo:ai", "--filter-annotation", "type=sbom", "--config"},
			expectOut: "{}",
		},
		{
			name:      "External",
			args:      []string{"artifact", "get", "--subject", "ocidir://../../testdata/testrepo:v2", "--filter-artifact-type", "application/example.sbom", "--sort-annotation", "preference", "--external", "ocidir://../../testdata/external"},
			expectOut: "bacon",
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

func TestArtifactList(t *testing.T) {
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
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:      "Missing manifest",
			args:      []string{"artifact", "list", "ocidir://../../testdata/testrepo:missing"},
			expectErr: errs.ErrNotFound,
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
		{
			name:        "External referrers",
			args:        []string{"artifact", "list", "ocidir://../../testdata/testrepo:v2", "--external", "ocidir://../../testdata/external"},
			expectOut:   "Referrers:",
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

func TestArtifactPut(t *testing.T) {
	testDir := t.TempDir()
	testData := []byte("hello world")
	testConfName := filepath.Join(testDir, "exConf")
	err := os.WriteFile(testConfName, []byte(`{"hello": "world"}`), 0600)
	if err != nil {
		t.Fatalf("failed creating test conf: %v", err)
	}
	testFileName := filepath.Join(testDir, "exFile")
	err = os.WriteFile(testFileName, []byte(`example test file`), 0600)
	if err != nil {
		t.Fatalf("failed creating test conf: %v", err)
	}

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
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:        "Put artifact",
			args:        []string{"artifact", "put", "ocidir://" + testDir + ":put"},
			in:          testData,
			expectOut:   "using default value for artifact-type is not recommended",
			outContains: true,
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
			args: []string{"artifact", "put", "--config-type", "application/vnd.example", "--config-file", testConfName, "ocidir://" + testDir + ":put-example-conf-data"},
			in:   testData,
		},
		{
			name: "Put artifact example conf data and file",
			args: []string{"artifact", "put", "--config-type", "application/vnd.example", "--config-file", testConfName, "--file", testFileName, "ocidir://" + testDir + ":put-example-file-data"},
			in:   testData,
		},
		{
			name: "Put artifact example conf data and file stripped",
			args: []string{"artifact", "put", "--config-type", "application/vnd.example", "--config-file", testConfName, "--file", testFileName, "--file-title", "--strip-dirs", "ocidir://" + testDir + ":put-example-file-data"},
			in:   testData,
		},
		{
			name: "Put subject",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--subject", "ocidir://" + testDir + ":put-example-at"},
			in:   testData,
		},
		{
			name: "Put subject to external repo",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--subject", "ocidir://" + testDir + ":put-example-at", "--external", "ocidir://" + testDir + "/external"},
			in:   testData,
		},
		{
			name: "Put subject to external name",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--subject", "ocidir://" + testDir + ":put-example-at", "ocidir://" + testDir + "/external:external-subj"},
			in:   testData,
		},
		{
			name: "Put subject to external repo and name",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--subject", "ocidir://" + testDir + ":put-example-at", "--external", "ocidir://" + testDir + "/external", "ocidir://" + testDir + "/external:external-subj"},
			in:   testData,
		},
		{
			name:      "Put external name without subject",
			args:      []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--external", "ocidir://" + testDir + "/external", "ocidir://" + testDir + "/external:external-subj"},
			in:        testData,
			expectErr: errs.ErrUnsupported,
		},
		{
			name:      "Put subject to external repo and different name",
			args:      []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--subject", "ocidir://" + testDir + ":put-example-at", "--external", "ocidir://" + testDir + "/external", "ocidir://" + testDir + "/copy:copy-subj"},
			in:        testData,
			expectErr: errs.ErrUnsupported,
		},
		{
			name: "Put create index",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--annotation", "test=a", "--platform", "linux/amd64", "--index", "ocidir://" + testDir + ":index"},
			in:   testData,
		},
		{
			name: "Put append index",
			args: []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--annotation", "test=b", "--platform", "linux/arm64", "--index", "ocidir://" + testDir + ":index"},
			in:   testData,
		},
		{
			name:      "Invalid-artifact-media-type",
			args:      []string{"artifact", "put", "--artifact-type", "application/vnd.example;version=1.0", "ocidir://" + testDir + ":err"},
			in:        testData,
			expectErr: errs.ErrUnsupportedMediaType,
		},
		{
			name:      "Invalid-config-media-type",
			args:      []string{"artifact", "put", "--config-type", "application/vnd.example;version=1.0", "--config-file", testConfName, "ocidir://" + testDir + ":err"},
			in:        testData,
			expectErr: errs.ErrUnsupportedMediaType,
		},
		{
			name:      "Invalid-file-media-type",
			args:      []string{"artifact", "put", "--artifact-type", "application/vnd.example", "--file", testFileName, "--file-media-type", "application/vnd.example;version=1.0", "ocidir://" + testDir + ":err"},
			in:        testData,
			expectErr: errs.ErrUnsupportedMediaType,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cobraOpts := cobraTestOpts{}
			if tc.in != nil {
				cobraOpts.stdin = bytes.NewBuffer(tc.in)
			}
			out, err := cobraTest(t, &cobraOpts, tc.args...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("did not fail with error: %v", tc.expectErr)
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
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:      "Missing manifest",
			args:      []string{"artifact", "tree", "ocidir://../../testdata/testrepo:missing"},
			expectErr: errs.ErrNotFound,
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
		{
			name:        "External referrers",
			args:        []string{"artifact", "tree", "ocidir://../../testdata/testrepo:v2", "--external", "ocidir://../../testdata/external"},
			expectOut:   "Referrers",
			outContains: true,
		}}
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
