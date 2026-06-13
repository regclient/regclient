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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredHelper(t *testing.T) {
	// cannot run cred helper in parallel because of OS working directory race conditions
	tests := []struct {
		name        string
		host        string
		credHelper  string
		credHost    string
		expectUser  string
		expectPass  string
		expectToken string
		expectErr   bool
	}{
		{
			name:       "user/pass",
			host:       "testhost.example.com",
			credHelper: "docker-credential-test",
			expectUser: "hello",
			expectPass: "world",
		},
		{
			name:        "token",
			host:        "testtoken.example.com",
			credHelper:  "docker-credential-test",
			expectToken: "deadbeefcafe",
		},
		{
			name:       DockerRegistry,
			host:       DockerRegistryDNS,
			credHost:   DockerRegistryAuth,
			credHelper: "docker-credential-test",
			expectUser: "hubuser",
			expectPass: "password123",
		},
		{
			name:       "http.example.com",
			host:       "http.example.com",
			credHost:   "http://http.example.com/",
			credHelper: "docker-credential-test",
			expectUser: "hello",
			expectPass: "universe",
		},
		{
			name:       "missing helper",
			host:       "missing.example.org",
			credHelper: "./testdata/docker-credential-missing",
			expectErr:  true,
		},
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed checking current directory: %v", err)
	}
	curPath := os.Getenv("PATH")
	t.Setenv("PATH", filepath.Join(cwd, "testdata")+string(os.PathListSeparator)+curPath)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := HostNewName(tt.host)
			h.CredHelper = tt.credHelper
			h.CredHost = tt.credHost
			ch := newCredHelper(tt.credHelper, map[string]string{})
			err := ch.get(h)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error not encountered")
				}
				return
			}
			if err != nil {
				t.Fatalf("error running get: %v", err)
			}
			if tt.expectUser != h.User {
				t.Errorf("user mismatch: expected %s, received %s", tt.expectUser, h.User)
			}
			if tt.expectPass != h.Pass {
				t.Errorf("password mismatch: expected %s, received %s", tt.expectPass, h.Pass)
			}
			if tt.expectToken != h.Token {
				t.Errorf("token mismatch: expected %s, received %s", tt.expectToken, h.Token)
			}
		})
	}
}
