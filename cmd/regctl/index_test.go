package main

import (
	"fmt"
	"testing"
)

func TestIndex(t *testing.T) {
	tmpDir := t.TempDir()
	latestRef := fmt.Sprintf("ocidir://%s/repo:latest", tmpDir)
	artifactRef := fmt.Sprintf("ocidir://%s/repo:latest", tmpDir)
	srcRef := "ocidir://../../testdata/testrepo:v2"

	// create index with 2 platforms from test repo
	out, err := cobraTest(t, nil, "index", "create", "--ref", srcRef, "--platform", "linux/amd64", "--platform", "linux/arm/v7", latestRef)
	if err != nil {
		t.Errorf("failed to run index create: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %s", out)
	}
	// verify content
	_, err = cobraTest(t, nil, "manifest", "get", "--platform", "linux/amd64", latestRef)
	if err != nil {
		t.Errorf("failed to get linux/amd64 entry: %v", err)
	}
	_, err = cobraTest(t, nil, "manifest", "get", "--platform", "linux/arm/v7", latestRef)
	if err != nil {
		t.Errorf("failed to get linux/arm/v7 entry: %v", err)
	}
	_, err = cobraTest(t, nil, "manifest", "get", "--platform", "linux/arm64", latestRef)
	if err == nil {
		t.Errorf("found linux/arm64 entry")
	}
	_, err = cobraTest(t, nil, "artifact", "get", "--subject", latestRef, "--platform", "linux/amd/v7", "--filter-artifact-type", "application/example.arms")
	if err == nil {
		t.Errorf("found referrers that were not copied")
	}

	// add artifact with referrers
	out, err = cobraTest(t, nil, "index", "add", "--ref", srcRef, "--platform", "linux/arm64", "--referrers", "--digest-tags", latestRef)
	if err != nil {
		t.Errorf("failed to run index add: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %s", out)
	}
	_, err = cobraTest(t, nil, "manifest", "get", "--platform", "linux/arm64", latestRef)
	if err != nil {
		t.Errorf("failed to get linux/arm64 entry: %v", err)
	}
	out, err = cobraTest(t, nil, "artifact", "get", "--subject", latestRef, "--platform", "linux/arm64", "--filter-artifact-type", "application/example.arms")
	if err != nil {
		t.Errorf("artifact not found: %v", err)
	}
	artifact64Out := "64 arms"
	if out != artifact64Out {
		t.Errorf("unexpected artifact content, expected: %s, received: %s", artifact64Out, out)
	}

	// create an index that itself is an artifact
	testArtifactType := "application/example.test"
	out, err = cobraTest(t, nil, "index", "create", artifactRef, "--subject", "latest", "--artifact-type", testArtifactType, "--ref", srcRef)
	if err != nil {
		t.Errorf("failed to run index create for artifact: %v", err)
		return
	}
	if out != "" {
		t.Errorf("unexpected output: %s", out)
	}
	out, err = cobraTest(t, nil, "manifest", "get", artifactRef, "--format", "{{.ArtifactType}}")
	if err != nil {
		t.Errorf("failed to get artifact type from manifest: %v", err)
	}
	if out != testArtifactType {
		t.Errorf("manifest artifact type, expected %s, received %s", testArtifactType, out)
	}
}
