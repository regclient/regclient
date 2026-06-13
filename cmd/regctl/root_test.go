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
