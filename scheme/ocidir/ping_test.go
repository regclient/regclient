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

package ocidir

import (
	"context"
	"testing"

	"github.com/regclient/regclient/types/ref"
)

func TestPing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	o := New()
	rOkay, err := ref.NewHost("ocidir://../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to create ref: %v", err)
	}
	result, err := o.Ping(ctx, rOkay)
	if err != nil {
		t.Fatalf("failed to ping: %v", err)
	}
	if result.Header != nil {
		t.Errorf("header is not nil")
	}
	if result.Stat == nil {
		t.Errorf("stat is nil")
	} else {
		if !result.Stat.IsDir() {
			t.Errorf("stat is not a directory")
		}
	}

	rMissing, err := ref.NewHost("ocidir://../../testdata/missing")
	if err != nil {
		t.Fatalf("failed to create ref: %v", err)
	}
	result, err = o.Ping(ctx, rMissing)
	if err == nil {
		t.Errorf("ping to missing directory succeeded")
	}
	if result.Header != nil {
		t.Errorf("header is not nil")
	}
	if result.Stat != nil {
		t.Errorf("stat on missing is not nil")
	}

	rFile, err := ref.NewHost("ocidir://../../testdata/testrepo/index.json")
	if err != nil {
		t.Fatalf("failed to create ref: %v", err)
	}
	result, err = o.Ping(ctx, rFile)
	if err == nil {
		t.Fatalf("ping to a file did not fail")
	}
	if result.Header != nil {
		t.Errorf("header is not nil")
	}
	if result.Stat == nil {
		t.Errorf("stat to a file is nil")
	} else {
		if result.Stat.IsDir() {
			t.Errorf("stat of a file is a directory")
		}
	}
}
