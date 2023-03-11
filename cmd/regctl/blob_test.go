package main

import (
	"bytes"
	"testing"
)

// TODO: implement tests

func TestBlob(t *testing.T) {
	repo := "ocidir://../../testdata/testrepo"
	digBaseA, err := cobraTest(t, "manifest", "get", repo+":b1", "--platform", "linux/amd64", "--format", "{{(index .Layers 0).Digest}}")
	if err != nil {
		t.Errorf("failed getting layer digest: %v", err)
	}
	digBaseB, err := cobraTest(t, "manifest", "get", repo+":b3", "--platform", "linux/amd64", "--format", "{{(index .Layers 0).Digest}}")
	if err != nil {
		t.Errorf("failed getting layer digest: %v", err)
	}
	digConf1, err := cobraTest(t, "manifest", "get", repo+":b1", "--platform", "linux/amd64", "--format", "{{.Config.Digest}}")
	if err != nil {
		t.Errorf("failed getting layer digest: %v", err)
	}
	digConf3, err := cobraTest(t, "manifest", "get", repo+":b3", "--platform", "linux/amd64", "--format", "{{.Config.Digest}}")
	if err != nil {
		t.Errorf("failed getting layer digest: %v", err)
	}

	t.Run("Get", func(t *testing.T) {
		// run a get request
		out, err := cobraTest(t, "blob", "get", "--format", "{{printPretty .}}", repo, digBaseA)
		if err != nil {
			t.Errorf("failed to blob get: %v", err)
		}
		if out == "" {
			t.Errorf("no blob output received")
		}
		// run a head request
		out, err = cobraTest(t, "blob", "head", "--format", "{{printPretty .}}", repo, digBaseA)
		if err != nil {
			t.Errorf("failed to blob head: %v", err)
		}
		if out == "" {
			t.Errorf("no blob output received")
		}
		// get a file from the blob
		out, err = cobraTest(t, "blob", "get-file", repo, digBaseA, "base.txt")
		if err != nil {
			t.Errorf("failed to blob get-file: %v", err)
		}
		if out != "A" {
			t.Errorf("unexpected blob get-file output, expected A, received %s", out)
		}
	})

	t.Run("Put", func(t *testing.T) {
		dir := t.TempDir()
		bufStr := "hello world"
		buf := bytes.NewBufferString(bufStr)
		origIn := rootCmd.InOrStdin()
		rootCmd.SetIn(buf)
		defer rootCmd.SetIn(origIn)
		// put a blob
		dig, err := cobraTest(t, "blob", "put", "--format", "{{println .Digest}}", "ocidir://"+dir)
		if err != nil {
			t.Errorf("failed to blob copy: %v", err)
		}
		// get the blob from the tempdir
		out, err := cobraTest(t, "blob", "get", "--format", "{{printPretty .}}", "ocidir://"+dir, dig)
		if err != nil {
			t.Errorf("failed to blob get: %v", err)
		}
		if out != bufStr {
			t.Errorf("unexpected blob output, expected %s, received %s", bufStr, out)
		}
	})

	t.Run("Copy", func(t *testing.T) {
		dir := t.TempDir()
		// copy the blob to the tempdir
		_, err := cobraTest(t, "blob", "copy", repo, "ocidir://"+dir, digBaseA)
		if err != nil {
			t.Errorf("failed to blob copy: %v", err)
		}
		// get the blob from the tempdir
		out, err := cobraTest(t, "blob", "get", "--format", "{{printPretty .}}", "ocidir://"+dir, digBaseA)
		if err != nil {
			t.Errorf("failed to blob get: %v", err)
		}
		if out == "" {
			t.Errorf("no blob output received")
		}
	})

	t.Run("Diff", func(t *testing.T) {
		// diff the layers between two images
		out, err := cobraTest(t, "blob", "diff-layer", repo, digBaseA, repo, digBaseB)
		if err != nil {
			t.Errorf("failed to diff layers: %v", err)
		}
		if out == "" {
			t.Errorf("no output received from diff-layer")
		}
		// diff the config between two images
		out, err = cobraTest(t, "blob", "diff-config", repo, digConf1, repo, digConf3)
		if err != nil {
			t.Errorf("failed to diff config: %v", err)
		}
		if out == "" {
			t.Errorf("no output received from diff-config")
		}
	})

}
