package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/types/platform"
)

func TestDescriptorData(t *testing.T) {
	t.Parallel()
	tt := []struct {
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
			name: "Bad Digest",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      10,
				Digest:    digest.Digest("sha256:e4a380728755139f156563e8b795581d5915dcc947fe937c524c6d52fd604b99"),
				Data:      []byte("example data"),
			},
			wantErr: ErrParsingFailed,
		},
		{
			name: "Bad Size",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      1000,
				Digest:    digest.Digest("sha256:44752f37272e944fd2c913a35342eaccdd1aaf189bae50676b301ab213fc5061"),
				Data:      []byte("example data"),
			},
			wantErr: ErrParsingFailed,
		},
		{
			name: "Good data",
			d: Descriptor{
				MediaType: MediaTypeOCI1LayerGzip,
				Size:      12,
				Digest:    digest.Digest("sha256:44752f37272e944fd2c913a35342eaccdd1aaf189bae50676b301ab213fc5061"),
				Data:      []byte("example data"),
			},
			wantData: []byte("example data"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.d.GetData()
			if tc.wantErr != nil {
				if err == nil || (!errors.Is(err, tc.wantErr) && err.Error() != tc.wantErr.Error()) {
					t.Errorf("expected error %v, received %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("received error %v", err)
				return
			}
			if !bytes.Equal(out, tc.wantData) {
				t.Errorf("data mismatch, expected %s, received %s", string(tc.wantData), string(out))
			}
		})
	}
}

func TestDescriptorEq(t *testing.T) {
	t.Parallel()
	digA := digest.FromString("test A")
	digB := digest.FromString("test B")
	tt := []struct {
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
			name: "converting OCI media type",
			d1: Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Size:      1234,
				Digest:    digA,
			},
			d2: Descriptor{
				MediaType: MediaTypeOCI1Manifest,
				Size:      1234,
				Digest:    digA,
			},
			expectEqual: false,
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
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.d1.Equal(tc.d2) != tc.expectEqual {
				t.Errorf("equal is not %v", tc.expectEqual)
			}
			if tc.d1.Same(tc.d2) != tc.expectSame {
				t.Errorf("same is not %v", tc.expectSame)
			}
		})
	}
}

func TestDataJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		dJSON    []byte
		wantData []byte
		wantErr  error
	}{
		{
			name: "No Data",
			dJSON: []byte(`{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"digest":    "sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14",
				"size":      941
			}`),
			wantErr: ErrParsingFailed,
		},
		{
			name: "Bad Data",
			dJSON: []byte(`{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"digest":    "sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14",
				"size":      1234,
				"data":      "Invalid data string"
			}`),
			wantErr: fmt.Errorf("illegal base64 data at input byte 7"),
		},
		{
			name: "Bad Digest",
			dJSON: []byte(`{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"digest":    "sha256:e4a380728755139f156563e8b795581d5915dcc947fe937c524c6d52fd604b99",
				"size":      10,
				"data":      "ZXhhbXBsZSBkYXRh"
			}`),
			wantErr: ErrParsingFailed,
		},
		{
			name: "Bad Size",
			dJSON: []byte(`{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"digest":    "sha256:44752f37272e944fd2c913a35342eaccdd1aaf189bae50676b301ab213fc5061",
				"size":      1000,
				"data":      "ZXhhbXBsZSBkYXRh"
			}`),
			wantErr: ErrParsingFailed,
		},
		{
			name: "Good data",
			dJSON: []byte(`{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"digest":    "sha256:44752f37272e944fd2c913a35342eaccdd1aaf189bae50676b301ab213fc5061",
				"size":      12,
				"data":      "ZXhhbXBsZSBkYXRh"
			}`),
			wantData: []byte("example data"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := Descriptor{}
			err := json.Unmarshal(tt.dJSON, &desc)
			if err != nil {
				if tt.wantErr == nil {
					t.Errorf("failed to parse json: %v", err)
				} else if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Errorf("expected error %v, received %v", tt.wantErr, err)
				}
				return
			}
			out, err := desc.GetData()
			if tt.wantErr != nil {
				if err == nil || (!errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error()) {
					t.Errorf("expected error %v, received %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("received error %v", err)
			}
			if !bytes.Equal(out, tt.wantData) {
				t.Errorf("data mismatch, expected %s, received %s", string(tt.wantData), string(out))
			}
		})
	}
}

func TestDescriptorSearch(t *testing.T) {
	t.Parallel()
	dAMD64 := Descriptor{
		MediaType: MediaTypeOCI1Manifest,
		Size:      12345,
		Digest:    EmptyDigest,
		Platform: &platform.Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
	}
	dAMD64Win := Descriptor{
		MediaType: MediaTypeOCI1Manifest,
		Size:      12345,
		Digest:    EmptyDigest,
		Platform: &platform.Platform{
			OS:           "windows",
			Architecture: "amd64",
		},
	}
	dARM64 := Descriptor{
		MediaType: MediaTypeOCI1Manifest,
		Size:      12345,
		Digest:    EmptyDigest,
		Platform: &platform.Platform{
			OS:           "linux",
			Architecture: "arm64",
		},
	}
	dAnnotations := Descriptor{
		MediaType: MediaTypeOCI1Manifest,
		Size:      12345,
		Digest:    EmptyDigest,
		Platform: &platform.Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
		Annotations: map[string]string{
			"runtime": "special runtime",
			"version": "1.2.3",
			"date":    "2022-01-01 12:34:56",
		},
	}
	dAnnotations2 := Descriptor{
		MediaType: MediaTypeOCI1Manifest,
		Size:      12345,
		Digest:    EmptyDigest,
		Platform: &platform.Platform{
			OS:           "linux",
			Architecture: "amd64",
		},
		Annotations: map[string]string{
			"runtime": "special runtime",
			"version": "1.3.0",
			"date":    "2022-04-01 01:02:03",
		},
	}
	dArtifact := Descriptor{
		MediaType:    MediaTypeOCI1Manifest,
		Size:         12345,
		Digest:       EmptyDigest,
		ArtifactType: "application/example.artifact",
		Annotations: map[string]string{
			"version": "1.2.3",
			"date":    "2022-01-01 12:34:56",
		},
	}
	dArtifact2 := Descriptor{
		MediaType:    MediaTypeOCI1Manifest,
		Size:         12345,
		Digest:       EmptyDigest,
		ArtifactType: "application/example.artifact",
		Annotations: map[string]string{
			"version": "1.2.9",
			"date":    "2022-04-01 01:02:03",
			"unique":  "x",
		},
	}
	dArtifact3 := Descriptor{
		MediaType:    MediaTypeOCI1Manifest,
		Size:         12345,
		Digest:       EmptyDigest,
		ArtifactType: "application/example.artifact",
		Annotations: map[string]string{
			"version": "1.3.0",
			"date":    "2022-02-28 02:04:08",
		},
	}
	testDL := []Descriptor{
		dAMD64,
		dARM64,
		dAMD64Win,
		dAnnotations,
		dAnnotations2,
		dArtifact,
		dArtifact2,
		dArtifact3,
	}
	tt := []struct {
		name   string
		dl     []Descriptor
		opt    MatchOpt
		expect Descriptor
		err    error
	}{
		{
			name: "empty",
			err:  ErrNotFound,
		},
		{
			name: "amd64",
			dl:   testDL,
			opt: MatchOpt{
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			expect: dAMD64,
		},
		{
			name: "amd64 compat",
			dl:   testDL,
			opt: MatchOpt{
				Platform: &platform.Platform{
					OS:           "darwin",
					Architecture: "amd64",
				},
			},
			expect: dAMD64,
		},
		{
			name: "amd64 windows",
			dl:   testDL,
			opt: MatchOpt{
				Platform: &platform.Platform{
					OS:           "windows",
					Architecture: "amd64",
				},
			},
			expect: dAMD64Win,
		},
		{
			name: "amd64 annotations",
			dl:   testDL,
			opt: MatchOpt{
				Annotations: map[string]string{
					"runtime": "special runtime",
				},
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			expect: dAnnotations,
		},
		{
			name: "artifact missing",
			dl:   testDL,
			opt: MatchOpt{
				ArtifactType: "application/example.artifact",
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			err: ErrNotFound,
		},
		{
			name: "artifact",
			dl:   testDL,
			opt: MatchOpt{
				ArtifactType: "application/example.artifact",
				Annotations:  map[string]string{},
			},
			expect: dArtifact,
		},
		{
			name: "artifact sort",
			dl:   testDL,
			opt: MatchOpt{
				ArtifactType:   "application/example.artifact",
				Annotations:    map[string]string{},
				SortAnnotation: "date",
			},
			expect: dArtifact,
		},
		{
			name: "artifact sort desc",
			dl:   testDL,
			opt: MatchOpt{
				ArtifactType:   "application/example.artifact",
				Annotations:    map[string]string{},
				SortAnnotation: "date",
				SortDesc:       true,
			},
			expect: dArtifact2,
		},
		{
			name: "artifact sort unset",
			dl:   testDL,
			opt: MatchOpt{
				ArtifactType:   "application/example.artifact",
				Annotations:    map[string]string{},
				SortAnnotation: "unique",
			},
			expect: dArtifact2,
		},
		{
			name: "artifact sort unset desc",
			dl:   testDL,
			opt: MatchOpt{
				ArtifactType:   "application/example.artifact",
				Annotations:    map[string]string{},
				SortAnnotation: "unique",
				SortDesc:       true,
			},
			expect: dArtifact2,
		},
		{
			name: "artifact sort all unique desc",
			dl:   testDL,
			opt: MatchOpt{
				SortAnnotation: "unique",
				SortDesc:       true,
			},
			expect: dArtifact2,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DescriptorListSearch(tc.dl, tc.opt)
			if tc.err != nil {
				if err == nil {
					t.Error("did not fail")
				} else if !errors.Is(err, tc.err) && err.Error() != tc.err.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tc.err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error, received %v", err)
			}
			if !tc.expect.Equal(result) {
				t.Errorf("unexpected result, expected %v, received %v", tc.expect, result)
			}
		})
	}
}
