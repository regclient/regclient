package ocidir

import (
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

func cmpSliceString(a, b []string) bool {
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

func TestIndex(t *testing.T) {
	// ctx := context.Background()
	fsMem := rwfs.MemNew()
	o := New(WithFS(fsMem))
	dig1 := digest.FromString("test digest 1")
	dig2 := digest.FromString("test digest 2")
	dig3 := digest.FromString("test digest 3")
	r, err := ref.New("ocidir://testrepo")
	if err != nil {
		t.Errorf("failed to generate ref: %v", err)
	}
	rA, err := ref.New("ocidir://testrepo:tag-a")
	if err != nil {
		t.Errorf("failed to generate ref: %v", err)
	}
	rB, err := ref.New("ocidir://testrepo:tag-b")
	if err != nil {
		t.Errorf("failed to generate ref: %v", err)
	}
	rC, err := ref.New("ocidir://testrepo:tag-c")
	if err != nil {
		t.Errorf("failed to generate ref: %v", err)
	}
	rDig, err := ref.New("ocidir://testrepo@" + dig1.String())
	if err != nil {
		t.Errorf("failed to generate ref: %v", err)
	}
	descNoTag := types.Descriptor{
		MediaType: types.MediaTypeDocker2Manifest,
		Size:      1234,
		Digest:    dig1,
	}
	descA := types.Descriptor{
		MediaType: types.MediaTypeDocker2Manifest,
		Size:      1234,
		Digest:    dig2,
		Annotations: map[string]string{
			aOCIRefName: "tag-a",
		},
	}
	descB := types.Descriptor{
		MediaType: types.MediaTypeDocker2Manifest,
		Size:      1234,
		Digest:    dig2,
		Annotations: map[string]string{
			aOCIRefName: "tag-b",
		},
	}
	descC := types.Descriptor{
		MediaType: types.MediaTypeDocker2Manifest,
		Size:      1234,
		Digest:    dig3,
		Annotations: map[string]string{
			aOCIRefName: rC.CommonName(),
		},
	}
	tests := []struct {
		name         string
		index        v1.Index
		get          ref.Ref
		expectGet    types.Descriptor
		expectGetErr error
		set          ref.Ref
		setDesc      types.Descriptor
		expectLen    int
	}{
		{
			name:         "empty",
			get:          rA,
			expectGetErr: types.ErrNotFound,
		},
		{
			name: "no tag",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: types.MediaTypeOCI1ManifestList,
				Manifests: []types.Descriptor{
					descNoTag,
				},
			},
			get:       rDig,
			expectGet: descNoTag,
			set:       rA,
			setDesc:   descA,
			expectLen: 2,
		},
		{
			name: "tag a",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: types.MediaTypeOCI1ManifestList,
				Manifests: []types.Descriptor{
					descNoTag,
					descA,
				},
			},
			get:       rDig,
			expectGet: descNoTag,
			set:       rC,
			setDesc:   descNoTag,
			expectLen: 2,
		},
		{
			name: "tag b",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: types.MediaTypeOCI1ManifestList,
				Manifests: []types.Descriptor{
					descNoTag,
					descB,
				},
			},
			get:       rB,
			expectGet: descB,
			set:       rB,
			setDesc:   descNoTag,
			expectLen: 1,
		},
		{
			name: "tag c",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: types.MediaTypeOCI1ManifestList,
				Manifests: []types.Descriptor{
					descA,
					descC,
				},
			},
			get:       rC,
			expectGet: descC,
			set:       rA,
			setDesc:   descNoTag,
			expectLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := o.writeIndex(r, tt.index)
			if err != nil {
				t.Errorf("failed to write index: %v", err)
			}
			index, err := o.readIndex(r)
			if err != nil {
				t.Errorf("failed to read index: %v", err)
			}
			if !tt.get.IsZero() {
				d, err := indexGet(index, tt.get)
				if tt.expectGetErr != nil {
					if err == nil {
						t.Errorf("indexGet did not fail")
					} else if !errors.Is(err, tt.expectGetErr) && err.Error() != tt.expectGetErr.Error() {
						t.Errorf("unexpected error from indexGet, expected %v, received %v", tt.expectGetErr, err)
					}
				} else {
					if err != nil {
						t.Errorf("indexGet failed: %v", err)
					} else if !d.Equal(tt.expectGet) {
						t.Errorf("indexGet descriptor, expected %v, received %v", tt.expectGet, d)
					}
				}
			}
			if !tt.set.IsZero() {
				err := indexSet(&index, tt.set, tt.setDesc)
				if err != nil {
					t.Errorf("indexSet failed: %v", err)
				}
			}
			if len(index.Manifests) != tt.expectLen {
				t.Errorf("unexpected length, expected %d, found %d, index: %v", tt.expectLen, len(index.Manifests), index)
			}
		})
	}
}
