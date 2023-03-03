package main

import (
	"fmt"
	"testing"
)

func TestIndexCreate(t *testing.T) {
	tmpDir := t.TempDir()
	tmpRef := fmt.Sprintf("ocidir://%s/repo:latest", tmpDir)
	srcRef := "ocidir://../../testdata/testrepo:v3"

	// create index with 2 platforms from test repo
	out, err := cobraTest(t, "index", "create", "--ref", srcRef, "--platform", "linux/amd64", "--platform", "linux/arm/v7", tmpRef)
	if err != nil {
		t.Errorf("failed to run index create: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}
	// verify content
	_, err = cobraTest(t, "manifest", "get", "--platform", "linux/amd64", tmpRef)
	if err != nil {
		t.Errorf("failed to get linux/amd64 entry: %v", err)
	}
	_, err = cobraTest(t, "manifest", "get", "--platform", "linux/arm/v7", tmpRef)
	if err != nil {
		t.Errorf("failed to get linux/arm/v7 entry: %v", err)
	}
	_, err = cobraTest(t, "manifest", "get", "--platform", "linux/arm64", tmpRef)
	if err == nil {
		t.Errorf("found linux/arm64 entry: %v", err)
	}
}
