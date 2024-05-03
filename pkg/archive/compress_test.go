package archive

import (
	"bytes"
	"io"
	"testing"
)

func TestMarshal(t *testing.T) {
	for _, algo := range []CompressType{CompressNone, CompressGzip, CompressBzip2, CompressXz, CompressZstd} {
		t.Run(algo.String(), func(t *testing.T) {
			var newAlgo CompressType
			b, err := algo.MarshalText()
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}
			err = newAlgo.UnmarshalText(b)
			if err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			if algo != newAlgo {
				t.Errorf("marshaling round trip failed for %s: %v -> %s -> %v", algo.String(), algo, string(b), newAlgo)
			}
		})
	}
}

func TestRoundtrip(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name    string
		content []byte
	}{
		{
			name:    "empty",
			content: []byte(``),
		},
		{
			name:    "hello-world",
			content: []byte(`hello world`),
		},
	}
	for _, algo := range []CompressType{CompressNone, CompressGzip, CompressXz, CompressZstd} {
		algo := algo
		t.Run(algo.String(), func(t *testing.T) {
			for _, tc := range tt {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					br := bytes.NewReader(tc.content)
					cr, err := Compress(br, algo)
					if err != nil {
						t.Fatalf("failed to compress: %v", err)
					}
					dr, err := Decompress(cr)
					if err != nil {
						t.Fatalf("failed to decompress: %v", err)
					}
					out, err := io.ReadAll(dr)
					if err != nil {
						t.Fatalf("failed to ReadAll: %v", err)
					}
					if !bytes.Equal(tc.content, out) {
						t.Errorf("output mismatch: expected %s, received %s", tc.content, out)
					}
				})
			}
		})
	}
}
