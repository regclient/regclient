package diff

import "testing"

func TestDiff(t *testing.T) {
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
