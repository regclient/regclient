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
	exHeadersShort = http.Header{
		"Content-Type":          {types.MediaTypeDocker2ImageConfig},
		"Content-Length":        {fmt.Sprintf("%d", exLen-5)},
		"Docker-Content-Digest": {exDigest.String()},
	}
	exHeadersLong = http.Header{
		"Content-Type":          {types.MediaTypeDocker2ImageConfig},
		"Content-Length":        {fmt.Sprintf("%d", exLen+5)},
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
	fileLayer   = "../../testdata/layer.tar"
	fileLayerWH = "../../testdata/layer-wh.tar"
)

func TestCommon(t *testing.T) {
	// create test list
	tt := []struct {
		name     string
		opts     []Opts
		eBytes   []byte
		eDigest  digest.Digest
		eHeaders http.Header
		eLen     int64
		eMT      string
		eErr     error
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
		{
			name: "length exceeded",
			opts: []Opts{
				WithReader(bytes.NewReader(exBlob)),
				WithHeader(exHeadersShort),
				WithRef(exRef),
			},
			eBytes:   exBlob,
			eDigest:  exDigest,
			eHeaders: exHeadersShort,
			eLen:     exLen,
			eMT:      exMT,
			eErr:     types.ErrSizeLimitExceeded,
		},
		{
			name: "short read",
			opts: []Opts{
				WithReader(bytes.NewReader(exBlob)),
				WithHeader(exHeadersLong),
				WithRef(exRef),
			},
			eBytes:   exBlob,
			eDigest:  exDigest,
			eHeaders: exHeadersLong,
			eLen:     exLen,
			eMT:      exMT,
			eErr:     types.ErrShortRead,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			b := NewReader(tc.opts...)
			if len(tc.eBytes) > 0 {
				bb, err := b.RawBody()
				if tc.eErr != nil {
					if err == nil {
						t.Errorf("read did not fail")
					} else if err.Error() != tc.eErr.Error() && !errors.Is(err, tc.eErr) {
						t.Errorf("unexpected error, expected %v, received %v", tc.eErr, err)
					}
					return
				}
				if err != nil {
					t.Errorf("rawbody: %v", err)
					return
				}
				if !bytes.Equal(bb, tc.eBytes) {
					t.Errorf("rawbody, expected %s, received %s", string(tc.eBytes), string(bb))
				}
			}
			if tc.eDigest != "" && b.GetDescriptor().Digest != tc.eDigest {
				t.Errorf("digest, expected %s, received %s", tc.eDigest, b.GetDescriptor().Digest)
			}
			if tc.eLen > 0 && b.GetDescriptor().Size != tc.eLen {
				t.Errorf("length, expected %d, received %d", tc.eLen, b.GetDescriptor().Size)
			}
			if tc.eMT != "" && b.GetDescriptor().MediaType != tc.eMT {
				t.Errorf("media type, expected %s, received %s", tc.eMT, b.GetDescriptor().MediaType)
			}
			if tc.eHeaders != nil {
				bHeader := b.RawHeaders()
				for k, v := range tc.eHeaders {
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
			WithHeader(exHeaders),
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
		_, err = b.Seek(5, io.SeekStart)
		if err == nil {
			t.Errorf("seek to non-zero position did not fail")
		}
		pos, err := b.Seek(0, io.SeekStart)
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
		// test limit exceeded with read partial / seek 0 /read all pattern
		b = NewReader(
			WithReader(bytes.NewReader(exBlob)),
			WithHeader(exHeadersShort),
		)
		i, err = b.Read(bb)
		if err != nil {
			t.Errorf("read err: %v", err)
			return
		}
		if i != bl {
			t.Errorf("read length, expected %d, received %d", bl, i)
		}
		_, err = b.Seek(0, io.SeekStart)
		if err != nil {
			t.Errorf("seek err: %v", err)
			return
		}
		_, err = io.ReadAll(b)
		if err == nil {
			t.Errorf("readall did not fail")
			return
		}
		if !errors.Is(err, types.ErrSizeLimitExceeded) {
			t.Errorf("unexpected error on readall, expected %v, received %v", types.ErrSizeLimitExceeded, err)
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
	tt := []struct {
		name     string
		opts     []Opts
		fromJSON []byte
		wantRaw  []byte
		wantJSON []byte
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
			wantJSON: exBlob,
		},
		{
			name: "JSONMarshal",
			opts: []Opts{
				WithDesc(exDesc),
			},
			fromJSON: exBlob,
			wantDesc: exDesc,
			wantJSON: exBlob,
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

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			oc := NewOCIConfig(tc.opts...)

			if len(tc.fromJSON) > 0 {
				err := oc.UnmarshalJSON(tc.fromJSON)
				if err != nil {
					t.Errorf("failed to unmarshal json: %v", err)
				}
			}
			if tc.wantDesc.Digest != "" && tc.wantDesc.Digest != oc.GetDescriptor().Digest {
				t.Errorf("digest, expected %s, received %s", tc.wantDesc.Digest, oc.GetDescriptor().Digest)
			}
			if tc.wantDesc.MediaType != "" && tc.wantDesc.MediaType != oc.GetDescriptor().MediaType {
				t.Errorf("media type, expected %s, received %s", tc.wantDesc.MediaType, oc.GetDescriptor().MediaType)
			}
			if tc.wantDesc.Size > 0 && tc.wantDesc.Size != oc.GetDescriptor().Size {
				t.Errorf("size, expected %d, received %d", tc.wantDesc.Size, oc.GetDescriptor().Size)
			}
			if len(tc.wantRaw) > 0 {
				raw, err := oc.RawBody()
				if err != nil {
					t.Errorf("config rawbody: %v", err)
					return
				}
				if !bytes.Equal(tc.wantRaw, raw) {
					t.Errorf("config bytes, expected %s, received %s", string(tc.wantRaw), string(raw))
				}
			}
			if len(tc.wantJSON) > 0 {
				ocJSON, err := oc.MarshalJSON()
				if err != nil {
					t.Errorf("json marshal: %v", err)
					return
				}
				if !bytes.Equal(tc.wantJSON, ocJSON) {
					t.Errorf("json marshal, expected %s, received %s", string(tc.wantJSON), string(ocJSON))
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
	fh, err := os.Open(fileLayer)
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

	tt := []struct {
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

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			fh, err := os.Open(fileLayer)
			if err != nil {
				t.Errorf("failed to open test data: %v", err)
				return
			}
			opts := append(tc.opts, WithReader(fh))
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
			if !tc.errClose && err != nil {
				t.Errorf("failed to close tar reader: %v", err)
			} else if tc.errClose && err == nil {
				t.Errorf("close did not fail")
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	tt := []struct {
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
	fileBytes, err := os.ReadFile(fileLayerWH)
	if err != nil {
		t.Errorf("failed to open test data: %v", err)
		return
	}
	blobDigest := digest.FromBytes(fileBytes)
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			fh, err := os.Open(fileLayerWH)
			if err != nil {
				t.Errorf("failed to open test data: %v", err)
				return
			}
			btr := NewTarReader(WithReader(fh), WithDesc(types.Descriptor{Size: int64(len(fileBytes)), Digest: blobDigest, MediaType: types.MediaTypeOCI1Layer}))
			th, rdr, err := btr.ReadFile(tc.filename)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("ReadFile did not fail")
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tc.expectErr, err)
				}
				err = btr.Close()
				if err != nil {
					t.Errorf("failed to close tar reader: %v", err)
				}
				return
			}
			if err != nil {
				t.Errorf("ReadFile failed: %v", err)
				btr.Close()
				return
			}
			if th == nil {
				t.Errorf("tar header is nil")
				btr.Close()
				return
			}
			if rdr == nil {
				t.Errorf("reader is nil")
				btr.Close()
				return
			}
			content, err := io.ReadAll(rdr)
			if err != nil {
				t.Errorf("failed reading file: %v", err)
			}
			if tc.content != string(content) {
				t.Errorf("file content mismatch: expected %s, received %s", tc.content, string(content))
			}
			err = btr.Close()
			if err != nil {
				t.Errorf("failed to close tar reader: %v", err)
			}
		})
	}
	t.Run("bad digest", func(t *testing.T) {
		fh, err := os.Open(fileLayerWH)
		if err != nil {
			t.Errorf("failed to open test data: %v", err)
			return
		}
		btr := NewTarReader(WithReader(fh), WithDesc(types.Descriptor{Size: int64(len(fileBytes)), Digest: digest.FromString("bad digest"), MediaType: types.MediaTypeOCI1Layer}))
		_, _, err = btr.ReadFile("missing.txt")
		if err == nil {
			t.Errorf("ReadFile did not fail")
		} else if !errors.Is(err, types.ErrDigestMismatch) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrDigestMismatch, err)
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
