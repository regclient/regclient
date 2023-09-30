package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestImageExportImport(t *testing.T) {
	tmpDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v2"
	exportFile := tmpDir + "/export.tar"
	exportName := "registry.example.com/repo:v2"
	importRefA := fmt.Sprintf("ocidir://%s/repo:v2", tmpDir)

	out, err := cobraTest(t, nil, "image", "export", "--name", exportName, srcRef, exportFile)
	if err != nil {
		t.Errorf("failed to run image export: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}

	out, err = cobraTest(t, nil, "image", "import", importRefA, exportFile)
	if err != nil {
		t.Errorf("failed to run image import: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}

	out, err = cobraTest(t, nil, "image", "export", "--name", exportName, "--platform", "linux/amd64", srcRef, exportFile)
	if err != nil {
		t.Errorf("failed to run image export: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}
}

func TestImageInspect(t *testing.T) {
	srcRef := "ocidir://../../testdata/testrepo:v3"
	tt := []struct {
		name        string
		cmd         []string
		expectOut   string
		outContains bool
	}{
		{
			name:        "default",
			cmd:         []string{"image", "inspect", srcRef},
			expectOut:   "created",
			outContains: true,
		},
		{
			name:        "format body",
			cmd:         []string{"image", "inspect", srcRef, "--format", `body`},
			expectOut:   "created",
			outContains: true,
		},
		{
			name:        "format config",
			cmd:         []string{"image", "inspect", srcRef, "--format", `{{ index .Config.Labels "version" }}`},
			expectOut:   "3",
			outContains: false,
		},
		{
			name:        "format getconfig",
			cmd:         []string{"image", "inspect", srcRef, "--format", `{{ .GetConfig.OS}}`},
			expectOut:   "linux",
			outContains: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.cmd...)
			if err != nil {
				t.Errorf("error: %v", err)
				return
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}

func TestImageMod(t *testing.T) {
	tmpDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v3"
	baseRef := "ocidir://../../testdata/testrepo:b1"
	modRef := fmt.Sprintf("ocidir://%s/repo:mod", tmpDir)

	out, err := cobraTest(t, nil, "image", "mod", srcRef, "--create", modRef, "--time", "set=2000-01-01T00:00:00Z,base-ref="+baseRef)
	if err != nil {
		t.Errorf("failed to run image mod: %v", err)
		return
	}
	if out == "" {
		t.Errorf("missing output")
	}
}
