package regclient

import (
	"context"
	"errors"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

func TestImageCheckBase(t *testing.T) {
	ctx := context.Background()
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	rc := New(WithFS(fsMem))
	rb1, err := ref.New("ocidir://testrepo:b1")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}
	rb2, err := ref.New("ocidir://testrepo:b2")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}
	rb3, err := ref.New("ocidir://testrepo:b3")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}
	m3, err := rc.ManifestHead(ctx, rb3)
	if err != nil {
		t.Errorf("failed to get digest for base3: %v", err)
		return
	}
	dig3 := m3.GetDescriptor().Digest
	r1, err := ref.New("ocidir://testrepo:v1")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}
	r2, err := ref.New("ocidir://testrepo:v2")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}
	r3, err := ref.New("ocidir://testrepo:v3")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}

	tests := []struct {
		name      string
		opts      []ImageOpts
		r         ref.Ref
		expectErr error
	}{
		{
			name:      "missing annotation",
			r:         r1,
			expectErr: types.ErrMissingAnnotation,
		},
		{
			name:      "annotation v2",
			r:         r2,
			expectErr: types.ErrMismatch,
		},
		{
			name:      "annotation v3",
			r:         r3,
			expectErr: types.ErrMismatch,
		},
		{
			name: "manual v1, b1",
			r:    r1,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb1.CommonName())},
		},
		{
			name:      "manual v1, b2",
			r:         r1,
			opts:      []ImageOpts{ImageWithCheckBaseRef(rb2.CommonName())},
			expectErr: types.ErrMismatch,
		},
		{
			name:      "manual v1, b3",
			r:         r1,
			opts:      []ImageOpts{ImageWithCheckBaseRef(rb3.CommonName())},
			expectErr: types.ErrMismatch,
		},
		{
			name: "manual v2, b1",
			r:    r2,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb1.CommonName())},
		},
		{
			name: "manual v3, b1",
			r:    r3,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb1.CommonName())},
		},
		{
			name: "manual v3, b3 with digest",
			r:    r3,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb3.CommonName()), ImageWithCheckBaseDigest(dig3.String())},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rc.ImageCheckBase(ctx, tt.r, tt.opts...)
			if tt.expectErr != nil {
				if err == nil {
					t.Errorf("check base did not fail")
				} else if err.Error() != tt.expectErr.Error() && !errors.Is(err, tt.expectErr) {
					t.Errorf("error mismatch, expected %v, received %v", tt.expectErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("check base failed")
				}
			}
		})
	}
}
