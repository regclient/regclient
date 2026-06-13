// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"path/filepath"
	"testing"
)

func TestConfig(t *testing.T) {
	// set a temp dir for storing configs
	tempDir := t.TempDir()
	t.Setenv(ConfigEnv, filepath.Join(tempDir, "config.json"))

	// test empty config
	out, err := cobraTest(t, nil, "config", "get", "--format", "{{ json . }}")
	if err != nil {
		t.Errorf("failed to run config get: %v", err)
	}
	if out != `{}` {
		t.Errorf("unexpected output from empty config, expected: %s, received: %s", `{}`, out)
	}

	// set options
	testLimit := "420000000"
	out, err = cobraTest(t, nil, "config", "set", "--blob-limit", testLimit, "--docker-cert=false", "--docker-cred=false")
	if err != nil {
		t.Errorf("failed to set config: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}

	// get changes
	out, err = cobraTest(t, nil, "config", "get", "--format", "{{ .BlobLimit }}")
	if err != nil {
		t.Errorf("failed to run config get on blob-limit: %v", err)
	}
	if out != testLimit {
		t.Errorf("unexpected output for blob-limit, expected: %s, received: %s", testLimit, out)
	}
	out, err = cobraTest(t, nil, "config", "get", "--format", "{{ .IncDockerCert }}")
	if err != nil {
		t.Errorf("failed to run config get on docker-cert: %v", err)
	}
	if out != "false" {
		t.Errorf("unexpected output for docker-cert, expected: false, received: %s", out)
	}
	out, err = cobraTest(t, nil, "config", "get", "--format", "{{ .IncDockerCred }}")
	if err != nil {
		t.Errorf("failed to run config get on docker-cred: %v", err)
	}
	if out != "false" {
		t.Errorf("unexpected output for docker-cred, expected: false, received: %s", out)
	}

	// set a default credential helper
	out, err = cobraTest(t, nil, "config", "set", "--default-cred-helper", "test-helper")
	if err != nil {
		t.Errorf("failed to set credential helper: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}

	out, err = cobraTest(t, nil, "config", "get", "--format", "{{ .HostDefault.CredHelper }}")
	if err != nil {
		t.Errorf("failed to run config get on default cred helper: %v", err)
	}
	if out != "test-helper" {
		t.Errorf("unexpected output for default cred helper, expected: test-helper, received: %s", out)
	}

	// reset back to zero values
	out, err = cobraTest(t, nil, "config", "set", "--blob-limit", "0", "--docker-cert", "--docker-cred", "--default-cred-helper", "")
	if err != nil {
		t.Errorf("failed to set default values: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output from set: %s", out)
	}

	// verify config is empty
	out, err = cobraTest(t, nil, "config", "get", "--format", "{{ json . }}")
	if err != nil {
		t.Errorf("failed to run config get: %v", err)
	}
	if out != `{}` {
		t.Errorf("unexpected output from empty config, expected: %s, received: %s", `{}`, out)
	}
}
