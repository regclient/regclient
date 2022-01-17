package blob

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

var (
	exRef, _ = ref.New("localhost:5000/library/alpine:latest")
	exBlob   = []byte(`
	{
		"created": "2021-11-24T20:19:40.483367546Z",
		"architecture": "amd64",
		"os": "linux",
		"config": {
			"Env": [
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
			],
			"Cmd": [
				"/bin/sh"
			]
		},
		"rootfs": {
			"type": "layers",
			"diff_ids": [
				"sha256:8d3ac3489996423f53d6087c81180006263b79f206d3fdec9e66f0e27ceb8759"
			]
		},
		"history": [
			{
				"created": "2021-11-24T20:19:40.199700946Z",
				"created_by": "/bin/sh -c #(nop) ADD file:9233f6f2237d79659a9521f7e390df217cec49f1a8aa3a12147bbca1956acdb9 in / "
			},
			{
				"created": "2021-11-24T20:19:40.483367546Z",
				"created_by": "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]",
				"empty_layer": true
			}
		]
	}
	`)
	exLen     = int64(len(exBlob))
	exDigest  = digest.FromBytes(exBlob)
	exMT      = types.MediaTypeDocker2ImageConfig
	exHeaders = http.Header{
		"Content-Type":          {types.MediaTypeDocker2ImageConfig},
		"Content-Length":        {fmt.Sprintf("%d", exLen)},
		"Docker-Content-Digest": {exDigest.String()},
	}
	exResp = http.Response{
		Status:        http.StatusText(http.StatusOK),
		StatusCode:    http.StatusOK,
		Header:        exHeaders,
		ContentLength: exLen,
		Body:          io.NopCloser(bytes.NewReader(exBlob)),
	}
)

func TestCommon(t *testing.T) {
	// create test list
	tests := []struct {
		name     string
		opts     []Opts
		eBytes   []byte
		eDigest  digest.Digest
		eHeaders http.Header
		eLen     int64
		eMT      string
	}{
		{
			name: "empty",
		},
		{
			name:    "reader",
			opts:    []Opts{WithReader(io.NopCloser(bytes.NewReader(exBlob)))},
			eBytes:  exBlob,
			eDigest: exDigest,
			eLen:    exLen,
		},
		{
			name: "descriptor",
			opts: []Opts{
				WithReader(io.NopCloser(bytes.NewReader(exBlob))),
				WithDesc(ociv1.Descriptor{
					MediaType: exMT,
					Digest:    exDigest,
					Size:      exLen,
				}),
				WithRef(exRef),
			},
			eBytes:  exBlob,
			eDigest: exDigest,
			eLen:    exLen,
			eMT:     exMT,
		},
		{
			name: "headers",
			opts: []Opts{
				WithReader(io.NopCloser(bytes.NewReader(exBlob))),
				WithHeader(exHeaders),
				WithRef(exRef),
			},
			eBytes:   exBlob,
			eDigest:  exDigest,
			eHeaders: exHeaders,
			eLen:     exLen,
			eMT:      exMT,
		},
		{
			name: "response",
			opts: []Opts{
				WithResp(&exResp),
				WithRef(exRef),
			},
			eBytes:   exBlob,
			eDigest:  exDigest,
			eHeaders: exHeaders,
			eLen:     exLen,
			eMT:      exMT,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewReader(tt.opts...)
			if len(tt.eBytes) > 0 {
				bb, err := b.RawBody()
				if err != nil {
					t.Errorf("rawbody: %v", err)
					return
				}
				if bytes.Compare(bb, tt.eBytes) != 0 {
					t.Errorf("rawbody, expected %s, received %s", string(tt.eBytes), string(bb))
				}
			}
			if tt.eDigest != "" && b.Digest() != tt.eDigest {
				t.Errorf("digest, expected %s, received %s", tt.eDigest, b.Digest())
			}
			if tt.eLen > 0 && b.Length() != tt.eLen {
				t.Errorf("length, expected %d, received %d", tt.eLen, b.Length())
			}
			if tt.eMT != "" && b.MediaType() != tt.eMT {
				t.Errorf("media type, expected %s, received %s", tt.eMT, b.MediaType())
			}
			if tt.eHeaders != nil {
				bHeader := b.RawHeaders()
				for k, v := range tt.eHeaders {
					if _, ok := bHeader[k]; !ok {
						t.Errorf("missing header: %s", k)
					} else if !cmpSliceString(v, bHeader[k]) {
						t.Errorf("header mismatch for key %s, expected %v, received %v", k, v, bHeader[k])
					}
				}
			}
		})
	}
}

func TestReader(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// create empty blob
		b := NewReader()

		// test read, expect error
		_, err := b.RawBody()
		if err == nil {
			t.Errorf("unexpected success")
			return
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("unexpected err from rawbody: %v", err)
		}
	})

	t.Run("readseek", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(bytes.NewReader(exBlob)),
		)
		// test read and seek on blob1
		bl := 500
		bb := make([]byte, bl)
		i, err := b.Read(bb)
		if err != nil {
			t.Errorf("read err: %v", err)
			return
		}
		if i != bl {
			t.Errorf("read length, expected %d, received %d", bl, i)
		}
		bSeek, ok := b.(io.Seeker)
		if !ok {
			t.Errorf("seek interface missing")
			return
		}
		_, err = bSeek.Seek(5, io.SeekStart)
		if err == nil {
			t.Errorf("seek to non-zero position did not fail")
		}
		pos, err := bSeek.Seek(0, io.SeekStart)
		if err != nil {
			t.Errorf("seek err: %v", err)
			return
		}
		if pos != 0 {
			t.Errorf("seek pos, expected 0, received %d", pos)
		}
		bb, err = io.ReadAll(b)
		if err != nil {
			t.Errorf("readall: %v", err)
			return
		}
		if b.Digest() != exDigest {
			t.Errorf("digest mismatch, expected %s, received %s", exDigest, b.Digest())
		}
		if b.Length() != exLen {
			t.Errorf("length mismatch, expected %d, received %d", exLen, b.Length())
		}
	})

	t.Run("ociconfig", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(io.NopCloser(bytes.NewReader(exBlob))),
			WithDesc(ociv1.Descriptor{
				MediaType: exMT,
				Digest:    exDigest,
				Size:      exLen,
			}),
			WithRef(exRef),
		)
		// test ToOCIConfig on blob 2
		oc, err := b.ToOCIConfig()
		if err != nil {
			t.Errorf("ToOCIConfig: %v", err)
			return
		}
		if exDigest != oc.Digest() {
			t.Errorf("digest, expected %s, received %s", exDigest, oc.Digest())
		}
		ocb, err := oc.RawBody()
		if err != nil {
			t.Errorf("config rawbody: %v", err)
			return
		}
		if bytes.Compare(exBlob, ocb) != 0 {
			t.Errorf("config bytes, expected %s, received %s", string(exBlob), string(ocb))
		}
	})

	t.Run("rawbytes", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(io.NopCloser(bytes.NewReader(exBlob))),
		)
		// test RawBytes on blob 3
		bb, err := b.RawBody()
		if err != nil {
			t.Errorf("rawbody: %v", err)
			return
		}
		if bytes.Compare(exBlob, bb) != 0 {
			t.Errorf("config bytes, expected %s, received %s", string(exBlob), string(bb))
		}
	})
}

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
