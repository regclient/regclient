package blob

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types"
	v1 "github.com/regclient/regclient/types/oci/v1"
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
	exDesc = types.Descriptor{
		MediaType: exMT,
		Digest:    exDigest,
		Size:      exLen,
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
			opts:    []Opts{WithReader(bytes.NewReader(exBlob))},
			eBytes:  exBlob,
			eDigest: exDigest,
			eLen:    exLen,
		},
		{
			name: "descriptor",
			opts: []Opts{
				WithReader(bytes.NewReader(exBlob)),
				WithDesc(types.Descriptor{
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
				WithReader(bytes.NewReader(exBlob)),
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
				if !bytes.Equal(bb, tt.eBytes) {
					t.Errorf("rawbody, expected %s, received %s", string(tt.eBytes), string(bb))
				}
			}
			if tt.eDigest != "" && b.GetDescriptor().Digest != tt.eDigest {
				t.Errorf("digest, expected %s, received %s", tt.eDigest, b.GetDescriptor().Digest)
			}
			if tt.eLen > 0 && b.GetDescriptor().Size != tt.eLen {
				t.Errorf("length, expected %d, received %d", tt.eLen, b.GetDescriptor().Size)
			}
			if tt.eMT != "" && b.GetDescriptor().MediaType != tt.eMT {
				t.Errorf("media type, expected %s, received %s", tt.eMT, b.GetDescriptor().MediaType)
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
			err := b.Close()
			if err != nil {
				t.Errorf("failed closing blob: %v", err)
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
		_, err = io.ReadAll(b)
		if err != nil {
			t.Errorf("readall: %v", err)
			return
		}
		if b.GetDescriptor().Digest != exDigest {
			t.Errorf("digest mismatch, expected %s, received %s", exDigest, b.GetDescriptor().Digest)
		}
		if b.GetDescriptor().Size != exLen {
			t.Errorf("length mismatch, expected %d, received %d", exLen, b.GetDescriptor().Size)
		}

	})

	t.Run("ociconfig", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(bytes.NewReader(exBlob)),
			WithDesc(types.Descriptor{
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
		if exDigest != oc.GetDescriptor().Digest {
			t.Errorf("digest, expected %s, received %s", exDigest, oc.GetDescriptor().Digest)
		}
		ocb, err := oc.RawBody()
		if err != nil {
			t.Errorf("config rawbody: %v", err)
			return
		}
		if !bytes.Equal(exBlob, ocb) {
			t.Errorf("config bytes, expected %s, received %s", string(exBlob), string(ocb))
		}
	})

	t.Run("rawbytes", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(bytes.NewReader(exBlob)),
		)
		// test RawBytes on blob 3
		bb, err := b.RawBody()
		if err != nil {
			t.Errorf("rawbody: %v", err)
			return
		}
		if !bytes.Equal(exBlob, bb) {
			t.Errorf("config bytes, expected %s, received %s", string(exBlob), string(bb))
		}
	})
}

func TestOCI(t *testing.T) {
	ociConfig := v1.Image{}
	err := json.Unmarshal(exBlob, &ociConfig)
	if err != nil {
		t.Errorf("failed to unmarshal exBlob: %v", err)
		return
	}
	tests := []struct {
		name     string
		opts     []Opts
		wantRaw  []byte
		wantDesc types.Descriptor
	}{
		{
			name: "RawBody",
			opts: []Opts{
				WithRawBody(exBlob),
				WithDesc(exDesc),
			},
			wantDesc: exDesc,
			wantRaw:  exBlob,
		},
		{
			name: "Config with Default Desc",
			opts: []Opts{
				WithImage(ociConfig),
			},
			wantDesc: types.Descriptor{MediaType: types.MediaTypeOCI1ImageConfig},
		},
		{
			name: "Config with Docker Desc",
			opts: []Opts{
				WithImage(ociConfig),
				WithDesc(exDesc),
			},
			wantDesc: types.Descriptor{MediaType: exMT},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oc := NewOCIConfig(tt.opts...)

			if tt.wantDesc.Digest != "" && tt.wantDesc.Digest != oc.GetDescriptor().Digest {
				t.Errorf("digest, expected %s, received %s", tt.wantDesc.Digest, oc.GetDescriptor().Digest)
			}
			if tt.wantDesc.MediaType != "" && tt.wantDesc.MediaType != oc.GetDescriptor().MediaType {
				t.Errorf("media type, expected %s, received %s", tt.wantDesc.MediaType, oc.GetDescriptor().MediaType)
			}
			if tt.wantDesc.Size > 0 && tt.wantDesc.Size != oc.GetDescriptor().Size {
				t.Errorf("size, expected %d, received %d", tt.wantDesc.Size, oc.GetDescriptor().Size)
			}
			if len(tt.wantRaw) > 0 {
				raw, err := oc.RawBody()
				if err != nil {
					t.Errorf("config rawbody: %v", err)
					return
				}
				if !bytes.Equal(tt.wantRaw, raw) {
					t.Errorf("config bytes, expected %s, received %s", string(tt.wantRaw), string(raw))
				}
			}
		})
	}
	t.Run("ModConfig", func(t *testing.T) {
		// create blob
		oc := NewOCIConfig(
			WithRawBody(exBlob),
			WithDesc(types.Descriptor{
				MediaType: exMT,
				Digest:    exDigest,
				Size:      exLen,
			}),
			WithRef(exRef),
		)
		ociC := oc.GetConfig()
		ociC.History = append(ociC.History, v1.History{Comment: "test", EmptyLayer: true})
		oc.SetConfig(ociC)
		// ensure digest and raw body change
		if exDigest == oc.GetDescriptor().Digest {
			t.Errorf("digest did not change, received %s", oc.GetDescriptor().Digest)
		}
		if exMT != oc.GetDescriptor().MediaType {
			t.Errorf("media type changed, expected %s, received %s", exMT, oc.GetDescriptor().MediaType)
		}
		raw, err := oc.RawBody()
		if err != nil {
			t.Errorf("config rawbody: %v", err)
			return
		}
		if bytes.Equal(exBlob, raw) {
			t.Errorf("config bytes unchanged, received %s", string(raw))
		}
	})
}

func TestTarReader(t *testing.T) {
	fh, err := os.Open("../../testdata/layer.tar")
	if err != nil {
		t.Errorf("failed to open test data: %v", err)
		return
	}
	digger := digest.Canonical.Digester()
	fhSize, err := io.Copy(digger.Hash(), fh)
	if err != nil {
		t.Errorf("failed to build digest on test data: %v", err)
		return
	}
	fh.Close()
	dig := digger.Digest()

	tests := []struct {
		name     string
		opts     []Opts
		errClose bool
	}{
		{
			name: "no desc",
			opts: []Opts{},
		},
		{
			name: "good desc",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1Layer,
					Size:      fhSize,
					Digest:    dig,
				}),
			},
		},
		{
			name: "bad desc",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1Layer,
					Size:      fhSize,
					Digest:    digest.FromString("bad digest"),
				}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fh, err := os.Open("../../testdata/layer.tar")
			if err != nil {
				t.Errorf("failed to open test data: %v", err)
				return
			}
			opts := append(tt.opts, WithReader(fh))
			btr := NewTarReader(opts...)
			tr, err := btr.GetTarReader()
			if err != nil {
				t.Errorf("failed to get tar reader: %v", err)
				return
			}
			for {
				th, err := tr.Next()
				if err != nil {
					if err != io.EOF {
						t.Errorf("failed to read tar: %v", err)
						return
					}
					break
				}
				if th.Size != 0 {
					b, err := io.ReadAll(tr)
					if err != nil {
						t.Errorf("failed to read content: %v", err)
						break
					}
					if int64(len(b)) != th.Size {
						t.Errorf("content size mismatch, expected %d, received %d", th.Size, len(b))
					}
				}
			}
			err = btr.Close()
			if !tt.errClose && err != nil {
				t.Errorf("failed to close tar reader: %v", err)
			} else if tt.errClose && err == nil {
				t.Errorf("close did not fail")
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		content   string
		expectErr error
	}{
		{
			name:     "layer1",
			filename: "layer1.txt",
			content:  "1\n",
		},
		{
			name:     "layer1 absolute",
			filename: "/layer1.txt",
			content:  "1\n",
		},
		{
			name:      "layer2",
			filename:  "layer2.txt",
			expectErr: types.ErrFileDeleted,
		},
		{
			name:     "layer3",
			filename: "layer3.txt",
			content:  "3\n",
		},
		{
			name:      "opaque dir",
			filename:  "exdir/test.txt",
			expectErr: types.ErrFileDeleted,
		},
		{
			name:      "missing",
			filename:  "missing.txt",
			expectErr: types.ErrFileNotFound,
		},
		{
			name:      "invalid",
			filename:  ".wh.filename.txt",
			expectErr: fmt.Errorf(".wh. prefix is reserved for whiteout files"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fh, err := os.Open("../../testdata/layer-wh.tar")
			if err != nil {
				t.Errorf("failed to open test data: %v", err)
				return
			}
			btr := NewTarReader(WithReader(fh))
			defer btr.Close()
			th, rdr, err := btr.ReadFile(tt.filename)
			if tt.expectErr != nil {
				if err == nil {
					t.Errorf("ReadFile did not fail")
				} else if !errors.Is(err, tt.expectErr) && err.Error() != tt.expectErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tt.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("ReadFile failed: %v", err)
				return
			}
			if th == nil {
				t.Errorf("tar header is nil")
				return
			}
			if rdr == nil {
				t.Errorf("reader is nil")
				return
			}
			content, err := io.ReadAll(rdr)
			if err != nil {
				t.Errorf("failed reading file: %v", err)
			}
			if tt.content != string(content) {
				t.Errorf("file content mismatch: expected %s, received %s", tt.content, string(content))
			}
		})
	}

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
