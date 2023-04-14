package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig(t *testing.T) {
	// set a temp dir for storing configs
	tempDir := t.TempDir()
	origEnv, set := os.LookupEnv(ConfigEnv)
	if set {
		defer os.Setenv(ConfigEnv, origEnv)
	}
	os.Setenv(ConfigEnv, filepath.Join(tempDir, "config.json"))

	// test empty config
	out, err := cobraTest(t, "config", "get", "--format", "{{ json . }}")
	if err != nil {
		t.Errorf("failed to run config get: %v", err)
	}
	if out != `{"hosts":{}}` {
		t.Errorf("unexpected output from empty config, expected: %s, received: %s", `{"hosts":{}}`, out)
	}

	// set options
	testLimit := "420000000"
	out, err = cobraTest(t, "config", "set", "--blob-limit", testLimit)
	if err != nil {
		t.Errorf("failed to set blob-limit: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}
	out, err = cobraTest(t, "config", "set", "--docker-cert=false")
	if err != nil {
		t.Errorf("failed to set docker-cert: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}
	out, err = cobraTest(t, "config", "set", "--docker-cred=false")
	if err != nil {
		t.Errorf("failed to set docker-cred: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}

	// get changes
	out, err = cobraTest(t, "config", "get", "--format", "{{ .BlobLimit }}")
	if err != nil {
		t.Errorf("failed to run config get on blob-limit: %v", err)
	}
	if out != testLimit {
		t.Errorf("unexpected output for blob-limit, expected: %s, received: %s", testLimit, out)
	}
	out, err = cobraTest(t, "config", "get", "--format", "{{ .IncDockerCert }}")
	if err != nil {
		t.Errorf("failed to run config get on docker-cert: %v", err)
	}
	if out != "false" {
		t.Errorf("unexpected output for docker-cert, expected: false, received: %s", out)
	}
	out, err = cobraTest(t, "config", "get", "--format", "{{ .IncDockerCred }}")
	if err != nil {
		t.Errorf("failed to run config get on docker-cred: %v", err)
	}
	if out != "false" {
		t.Errorf("unexpected output for docker-cred, expected: false, received: %s", out)
	}

	// reset back to zero values
	out, err = cobraTest(t, "config", "set", "--blob-limit", "0")
	if err != nil {
		t.Errorf("failed to set blob-limit: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}
	out, err = cobraTest(t, "config", "set", "--docker-cert")
	if err != nil {
		t.Errorf("failed to set docker-cert: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}
	out, err = cobraTest(t, "config", "set", "--docker-cred")
	if err != nil {
		t.Errorf("failed to set docker-cred: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}

	// verify config is empty
	out, err = cobraTest(t, "config", "get", "--format", "{{ json . }}")
	if err != nil {
		t.Errorf("failed to run config get: %v", err)
	}
	if out != `{"hosts":{}}` {
		t.Errorf("unexpected output from empty config, expected: %s, received: %s", `{"hosts":{}}`, out)
	}

}
