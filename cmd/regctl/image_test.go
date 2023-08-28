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

func TestImageMod(t *testing.T) {
	tmpDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v3"
	baseRef := "ocidir://../../testdata/testrepo:b1"
	modRef := fmt.Sprintf("ocidir://%s/repo:mod", tmpDir)
	saveOpts := imageOpts

	out, err := cobraTest(t, "image", "mod", srcRef, "--create", modRef, "--time", "set=2000-01-01T00:00:00Z,base-ref="+baseRef)
	imageOpts = saveOpts
	if err != nil {
		t.Errorf("failed to run image import: %v", err)
		return
	}
	if out == "" {
		t.Errorf("missing output")
	}

}
