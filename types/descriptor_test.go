package types

import (
	"bytes"
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types/platform"
)

func TestDescriptorData(t *testing.T) {
	tests := []struct {
		name     string
		d        Descriptor
		wantData []byte
		wantErr  error
	}{
		{
			name: "No Data",
			d: Descriptor{
				MediaType: MediaTypeDocker2LayerGzip,
				Size:      941,
				Digest:    digest.Digest("sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14"),
			},
			wantErr: ErrParsingFailed,
		},
		{
			name: "Bad Data",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      1234,
				Digest:    digest.Digest("sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14"),
				Data:      []byte("Invalid data string"),
			},
			wantErr: ErrParsingFailed,
		},
		{
			name: "Bad Digest",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      10,
				Digest:    digest.Digest("sha256:e4a380728755139f156563e8b795581d5915dcc947fe937c524c6d52fd604b99"),
				Data:      []byte("R29vZCBkYXRhCg=="),
			},
			wantErr: ErrParsingFailed,
		},
		{
			name: "Bad Size",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      1000,
				Digest:    digest.Digest("sha256:e4a380728755139f156563e8b795581d5915dcc947fe937c524c6d52fd604b88"),
				Data:      []byte("R29vZCBkYXRhCg=="),
			},
			wantErr: ErrParsingFailed,
		},
		{
			name: "Good data",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      10,
				Digest:    digest.Digest("sha256:e4a380728755139f156563e8b795581d5915dcc947fe937c524c6d52fd604b88"),
				Data:      []byte("R29vZCBkYXRhCg=="),
			},
			wantData: []byte("Good data\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := tt.d.GetData()
			if tt.wantErr != nil {
				if err == nil || (!errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error()) {
					t.Errorf("expected error %v, received %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("received error %v", err)
				return
			}
			if !bytes.Equal(out, tt.wantData) {
				t.Errorf("data mismatch, expected %s, received %s", string(tt.wantData), string(out))
			}
		})
	}
}

func TestDescriptorEq(t *testing.T) {
	digA := digest.FromString("test A")
	digB := digest.FromString("test B")
	tests := []struct {
		name        string
		d1, d2      Descriptor
		expectEqual bool
		expectSame  bool
	}{
		{
			name:        "empty",
			expectEqual: true,
			expectSame:  true,
		},
		{
			name: "empty d1",
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  false,
		},
		{
			name: "empty d2",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  false,
		},
		{
			name: "same simple manifest",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: true,
			expectSame:  true,
		},
		{
			name: "different media type",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2ManifestList,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  false,
		},
		{
			name: "different size",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      4321,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  false,
		},
		{
			name: "different digest",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digB,
			},
			expectEqual: false,
			expectSame:  false,
		},
		{
			name: "annotation eq",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Annotations: map[string]string{
					"key a": "value a",
					"key b": "value b",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Annotations: map[string]string{
					"key b": "value b",
					"key a": "value a",
				},
			},
			expectEqual: true,
			expectSame:  true,
		},
		{
			name: "annotation diff",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Annotations: map[string]string{
					"key a": "value a",
					"key b": "value b",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Annotations: map[string]string{
					"key a": "value c",
					"key d": "value b",
				},
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "annotation missing",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Annotations: map[string]string{
					"key a": "value a",
					"key b": "value b",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "urls eq",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				URLs: []string{
					"url a",
					"url b",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				URLs: []string{
					"url a",
					"url b",
				},
			},
			expectEqual: true,
			expectSame:  true,
		},
		{
			name: "urls diff",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				URLs: []string{
					"url a",
					"url b",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				URLs: []string{
					"url c",
					"url d",
				},
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "urls missing",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				URLs: []string{
					"url a",
					"url b",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "platform eq",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			expectEqual: true,
			expectSame:  true,
		},
		{
			name: "platform diff",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "arm64",
				},
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "platform missing",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "artifactType eq",
			d1: Descriptor{
				MediaType:    MediaTypeDocker2Manifest,
				Size:         1234,
				Digest:       digA,
				ArtifactType: "application/vnd.example.test",
			},
			d2: Descriptor{
				MediaType:    MediaTypeDocker2Manifest,
				Size:         1234,
				Digest:       digA,
				ArtifactType: "application/vnd.example.test",
			},
			expectEqual: true,
			expectSame:  true,
		},
		{
			name: "artifactType diff",
			d1: Descriptor{
				MediaType:    MediaTypeDocker2Manifest,
				Size:         1234,
				Digest:       digA,
				ArtifactType: "application/vnd.example.test",
			},
			d2: Descriptor{
				MediaType:    MediaTypeDocker2Manifest,
				Size:         1234,
				Digest:       digA,
				ArtifactType: "application/vnd.example.test2",
			},
			expectEqual: false,
			expectSame:  true,
		},
		{
			name: "artifactType missing",
			d1: Descriptor{
				MediaType:    MediaTypeDocker2Manifest,
				Size:         1234,
				Digest:       digA,
				ArtifactType: "application/vnd.example.test",
			},
			d2: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
			expectSame:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.d1.Equal(tt.d2) != tt.expectEqual {
				t.Errorf("equal is not %v", tt.expectEqual)
			}
			if tt.d1.Same(tt.d2) != tt.expectSame {
				t.Errorf("same is not %v", tt.expectSame)
			}
		})
	}
}
