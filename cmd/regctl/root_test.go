package main

import (
	"testing"
)

func TestRootConfigDir(t *testing.T) {
	tmpDir := t.TempDir()

	// This is invalid but should gracefully degrade and not crash
	t.Setenv("REGCTL_CONFIG", tmpDir)

	out, err := cobraTest(t, nil, "tag", "ls", "ocidir://../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to list tags: %v", err)
	}
	if out == "" {
		t.Errorf("missing output")
	}
}
