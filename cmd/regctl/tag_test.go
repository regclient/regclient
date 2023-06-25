package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/regclient/regclient/types"
)

func TestTagList(t *testing.T) {
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "Missing arg",
			args:      []string{"tag", "ls"},
			expectErr: fmt.Errorf("accepts 1 arg(s), received 0"),
		},
		{
			name:      "Invalid ref",
			args:      []string{"tag", "ls", "invalid*ref"},
			expectErr: types.ErrInvalidReference,
		},
		{
			name:      "Missing repo",
			args:      []string{"tag", "ls", "ocidir://../../testdata/test-missing"},
			expectErr: fs.ErrNotExist,
		},
		{
			name:        "List tags",
			args:        []string{"tag", "ls", "ocidir://../../testdata/testrepo"},
			expectOut:   "v1\nv2\nv3",
			outContains: true,
		},
		{
			name:        "List tags filtered",
			args:        []string{"tag", "ls", "--include", "sha256.*", "--exclude", ".*\\.meta", "ocidir://../../testdata/testrepo"},
			expectOut:   "sha256-",
			outContains: true,
		},
		{
			name:        "List tags limited",
			args:        []string{"tag", "ls", "--include", "v.*", "--limit", "5", "ocidir://../../testdata/testrepo"},
			expectOut:   "v1\nv2\nv3",
			outContains: true,
		},
		{
			name:        "List tags formatted",
			args:        []string{"tag", "ls", "--format", "raw", "ocidir://../../testdata/testrepo"},
			expectOut:   "application/vnd.oci.image.index.v1+json",
			outContains: true,
		},
	}
	optInit := tagOpts
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tagOpts = optInit
			out, err := cobraTest(t, tc.args...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("did not receive expected error: %v", tc.expectErr)
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, received %v, expected %v", err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Errorf("returned unexpected error: %v", err)
				return
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}

}
