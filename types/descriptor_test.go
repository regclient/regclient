package types

import (
	"bytes"
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
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
				Size:      1234,
				Digest:    digest.Digest("sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14"),
				Data:      []byte("QmFkIGRpZ2VzdCBkYXRhCg=="),
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
