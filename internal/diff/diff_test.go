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

package diff

import "testing"

func TestDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		a, b, expect []string
		opts         []Opt
	}{
		{
			name: "empty",
		},
		{
			name: "deletes",
			a:    []string{"a", "b", "c"},
			expect: []string{
				"@@ -1,3 +1,0 @@",
				"- a",
				"- b",
				"- c",
			},
		},
		{
			name: "inserts",
			b:    []string{"a", "b", "c"},
			expect: []string{
				"@@ -1,0 +1,3 @@",
				"+ a",
				"+ b",
				"+ c",
			},
		},
		{
			name: "equal",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
		},
		{
			name: "myers",
			a:    []string{"a", "b", "c", "a", "b", "b", "a"},
			b:    []string{"c", "b", "a", "b", "a", "c"},
			expect: []string{
				"@@ -1,2 +1,0 @@",
				"- a",
				"- b",
				"@@ -4,0 +2,1 @@",
				"+ b",
				"@@ -6,1 +5,0 @@",
				"- b",
				"@@ -8,0 +6,1 @@",
				"+ c",
			},
		},
		{
			name: "replace",
			a:    []string{"a", "b", "c"},
			b:    []string{"d", "e", "f"},
			expect: []string{
				"@@ -1,3 +1,3 @@",
				"- a",
				"- b",
				"- c",
				"+ d",
				"+ e",
				"+ f",
			},
		},
		{
			name: "change one",
			a:    []string{"a", "b", "c", "d"},
			b:    []string{"a", "e", "f", "d"},
			expect: []string{
				"@@ -2,2 +2,2 @@",
				"- b",
				"- c",
				"+ e",
				"+ f",
			},
		},
		{
			name: "first",
			a:    []string{"a", "b", "c", "d", "e"},
			b:    []string{"f", "b", "c", "d", "e"},
			opts: []Opt{WithContext(1, 1)},
			expect: []string{
				"@@ -1,2 +1,2 @@",
				"- a",
				"+ f",
				"  b",
			},
		},
		{
			name: "last",
			a:    []string{"a", "b", "c", "d", "e"},
			b:    []string{"a", "b", "c", "d", "f"},
			opts: []Opt{WithContext(1, 1)},
			expect: []string{
				"@@ -4,2 +4,2 @@",
				"  d",
				"- e",
				"+ f",
			},
		},
		{
			name: "context one",
			a:    []string{"a", "b", "c", "d", "e"},
			b:    []string{"a", "b", "f", "d", "e"},
			opts: []Opt{WithContext(1, 1)},
			expect: []string{
				"@@ -2,3 +2,3 @@",
				"  b",
				"- c",
				"+ f",
				"  d",
			},
		},
		{
			name: "context three",
			a:    []string{"a", "b", "c", "d", "e"},
			b:    []string{"a", "b", "f", "d", "e"},
			opts: []Opt{WithContext(3, 3)},
			expect: []string{
				"@@ -1,5 +1,5 @@",
				"  a",
				"  b",
				"- c",
				"+ f",
				"  d",
				"  e",
			},
		},
		{
			name: "context full",
			a:    []string{"a", "b", "c", "d", "e"},
			b:    []string{"a", "b", "f", "d", "e"},
			opts: []Opt{WithFullContext()},
			expect: []string{
				"@@ -1,5 +1,5 @@",
				"  a",
				"  b",
				"- c",
				"+ f",
				"  d",
				"  e",
			},
		},
		{
			name: "context full multiple",
			a:    []string{"a", "b", "c", "d", "e"},
			b:    []string{"a", "f", "c", "g", "e"},
			opts: []Opt{WithFullContext()},
			expect: []string{
				"@@ -1,5 +1,5 @@",
				"  a",
				"- b",
				"+ f",
				"  c",
				"- d",
				"+ g",
				"  e",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Diff(tt.a, tt.b, tt.opts...)
			if !strSliceEq(tt.expect, result) {
				t.Errorf("mismatch, expected %v, received %v", tt.expect, result)
			}
		})
	}
}

func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
