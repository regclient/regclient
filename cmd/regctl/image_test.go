package main

import (
	"fmt"
	"testing"
)

func TestImageExportImport(t *testing.T) {
	tmpDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v2"
	exportFile := tmpDir + "/export.tar"
	exportName := "registry.example.com/repo:v2"
	importRefA := fmt.Sprintf("ocidir://%s/repo:v2", tmpDir)
	saveOpts := imageOpts

	out, err := cobraTest(t, "image", "export", "--name", exportName, srcRef, exportFile)
	imageOpts = saveOpts
	if err != nil {
		t.Errorf("failed to run image export: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}

	out, err = cobraTest(t, "image", "import", importRefA, exportFile)
	imageOpts = saveOpts
	if err != nil {
		t.Errorf("failed to run image import: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}

	out, err = cobraTest(t, "image", "export", "--name", exportName, "--platform", "linux/amd64", srcRef, exportFile)
	imageOpts = saveOpts
	if err != nil {
		t.Errorf("failed to run image export: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}
}
