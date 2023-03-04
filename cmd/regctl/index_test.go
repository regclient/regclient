package main

import (
	"fmt"
	"testing"
)

func TestIndex(t *testing.T) {
	tmpDir := t.TempDir()
	tmpRef := fmt.Sprintf("ocidir://%s/repo:latest", tmpDir)
	srcRef := "ocidir://../../testdata/testrepo:v2"

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
		t.Errorf("found linux/arm64 entry")
	}
	_, err = cobraTest(t, "artifact", "get", "--subject", tmpRef, "--platform", "linux/amd/v7", "--filter-artifact-type", "application/example.arms")
	if err == nil {
		t.Errorf("found referrers that were not copied")
	}

	// add artifact with referrers
	out, err = cobraTest(t, "index", "add", "--ref", srcRef, "--platform", "linux/arm64", "--referrers", "--digest-tags", tmpRef)
	if err != nil {
		t.Errorf("failed to run index add: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}
	_, err = cobraTest(t, "manifest", "get", "--platform", "linux/arm64", tmpRef)
	if err != nil {
		t.Errorf("failed to get linux/arm64 entry: %v", err)
	}
	out, err = cobraTest(t, "artifact", "get", "--subject", tmpRef, "--platform", "linux/arm64", "--filter-artifact-type", "application/example.arms")
	if err != nil {
		t.Errorf("artifact not found: %v", err)
	}
	artifact64Out := "64 arms"
	if out != artifact64Out {
		t.Errorf("unexpected artifact content, expected: %s, received: %s", artifact64Out, out)
	}
}
