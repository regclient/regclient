package ocidir

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/internal/reproducible"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/mediatype"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

// Verify OCIDir implements various interfaces.
var (
	_ scheme.API       = (*OCIDir)(nil)
	_ scheme.Closer    = (*OCIDir)(nil)
	_ scheme.GCLocker  = (*OCIDir)(nil)
	_ scheme.Throttler = (*OCIDir)(nil)
)

func TestIndex(t *testing.T) {
	// t.Parallel() // unable to use parallel when setting env
	t.Setenv(reproducible.EpocEnv, "0")
	expectCreated := "1970-01-01T00:00:00Z"
	// ctx := context.Background()
	tempDir := t.TempDir()
	o := New()
	dig1 := digest.FromString("test digest 1")
	dig2 := digest.FromString("test digest 2")
	dig3 := digest.FromString("test digest 3")
	r, err := ref.New("ocidir://" + tempDir + "/testrepo")
	if err != nil {
		t.Fatalf("failed to generate ref: %v", err)
	}
	rA := r.SetTag("tag-a")
	rB := r.SetTag("tag-b")
	rC := r.SetTag("tag-c")
	rDig1 := r.SetDigest(dig1.String())
	descDig1 := descriptor.Descriptor{
		MediaType: mediatype.Docker2Manifest,
		Size:      1234,
		Digest:    dig1,
	}
	descDig2TagA := descriptor.Descriptor{
		MediaType: mediatype.Docker2Manifest,
		Size:      1234,
		Digest:    dig2,
		Annotations: map[string]string{
			types.AnnotationRefName: "tag-a",
		},
	}
	descDig2TagB := descriptor.Descriptor{
		MediaType: mediatype.Docker2Manifest,
		Size:      1234,
		Digest:    dig2,
		Annotations: map[string]string{
			types.AnnotationRefName: "tag-b",
		},
	}
	descDig3TagFullC := descriptor.Descriptor{
		MediaType: mediatype.Docker2Manifest,
		Size:      1234,
		Digest:    dig3,
		Annotations: map[string]string{
			types.AnnotationRefName: rC.CommonName(),
		},
	}
	tests := []struct {
		name         string
		index        v1.Index
		getRef       ref.Ref
		expectGet    descriptor.Descriptor
		expectGetErr error
		setRef       ref.Ref
		setDesc      descriptor.Descriptor
		expectLen    int
	}{
		{
			name:         "empty",
			getRef:       rA,
			expectGetErr: errs.ErrNotFound,
		},
		{
			name: "no tag",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: mediatype.OCI1ManifestList,
				Manifests: []descriptor.Descriptor{
					descDig1,
				},
			},
			getRef:    rDig1,
			expectGet: descDig1,
			setRef:    rA,
			setDesc:   descDig2TagA,
			expectLen: 2,
		},
		{
			name: "tag a",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: mediatype.OCI1ManifestList,
				Manifests: []descriptor.Descriptor{
					descDig1,
					descDig2TagA,
				},
			},
			getRef:    rDig1,
			expectGet: descDig1,
			setRef:    rC,
			setDesc:   descDig1,
			expectLen: 3,
		},
		{
			name: "tag b",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: mediatype.OCI1ManifestList,
				Manifests: []descriptor.Descriptor{
					descDig1,
					descDig2TagB,
				},
			},
			getRef:    rB,
			expectGet: descDig2TagB,
			setRef:    rB,
			setDesc:   descDig1,
			expectLen: 2,
		},
		{
			name: "tag c",
			index: v1.Index{
				Versioned: v1.IndexSchemaVersion,
				MediaType: mediatype.OCI1ManifestList,
				Manifests: []descriptor.Descriptor{
					descDig2TagA,
					descDig3TagFullC,
				},
			},
			getRef:    rC,
			expectGet: descDig3TagFullC,
			setRef:    rA,
			setDesc:   descDig1,
			expectLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := o.writeIndex(r, tt.index, false)
			if err != nil {
				t.Fatalf("failed to write index: %v", err)
			}
			index, err := o.readIndex(r, false)
			if err != nil {
				t.Fatalf("failed to read index: %v", err)
			}
			if !tt.getRef.IsZero() {
				d, err := indexGet(index, tt.getRef)
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
			if !tt.setRef.IsZero() {
				err := indexSet(&index, tt.setRef, tt.setDesc)
				if err != nil {
					t.Errorf("indexSet failed: %v", err)
				}
				if !slices.ContainsFunc(index.Manifests, func(d descriptor.Descriptor) bool {
					return d.Digest == tt.setDesc.Digest && d.Annotations != nil && d.Annotations[types.AnnotationCreated] == expectCreated
				}) {
					b, _ := json.Marshal(index)
					t.Errorf("indexSet did not configure the timestamp for digest %s:\n%s", tt.setDesc.Digest, string(b))
				}
			}
			if len(index.Manifests) != tt.expectLen {
				t.Errorf("unexpected length, expected %d, found %d, index: %v", tt.expectLen, len(index.Manifests), index)
			}
		})
	}
}
