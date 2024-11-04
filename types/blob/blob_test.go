package blob

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/mediatype"
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
	exMT      = mediatype.Docker2ImageConfig
	exHeaders = http.Header{
		"Content-Type":          {mediatype.Docker2ImageConfig},
		"Content-Length":        {fmt.Sprintf("%d", exLen)},
		"Docker-Content-Digest": {exDigest.String()},
	}
	exHeadersShort = http.Header{
		"Content-Type":          {mediatype.Docker2ImageConfig},
		"Content-Length":        {fmt.Sprintf("%d", exLen-5)},
		"Docker-Content-Digest": {exDigest.String()},
	}
	exHeadersLong = http.Header{
		"Content-Type":          {mediatype.Docker2ImageConfig},
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
	exDesc = descriptor.Descriptor{
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
				WithDesc(descriptor.Descriptor{
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
			eErr:     errs.ErrSizeLimitExceeded,
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
			eErr:     errs.ErrShortRead,
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
					t.Fatalf("rawbody: %v", err)
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
	t.Run("nil", func(t *testing.T) {
		var b *BReader
		_, err := b.Read([]byte{})
		if err == nil {
			t.Errorf("nil read did not fail")
		}
		err = b.Close()
		if err != nil {
			t.Errorf("nil close failed: %v", err)
		}
		_, err = b.ToOCIConfig()
		if err == nil {
			t.Errorf("nil convert ot OCI did not fail")
		}
		_, err = b.ToTarReader()
		if err == nil {
			t.Errorf("nil convert to tar reader did not fail")
		}
		_, err = b.Seek(0, io.SeekStart)
		if err == nil {
			t.Errorf("nil seek did not fail")
		}
	})
	t.Run("empty", func(t *testing.T) {
		// create empty blob
		b := NewReader()

		// test read, expect error
		_, err := b.RawBody()
		if err == nil {
			t.Fatalf("unexpected success")
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
			t.Fatalf("read err: %v", err)
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
			t.Fatalf("seek err: %v", err)
		}
		if pos != 0 {
			t.Errorf("seek pos, expected 0, received %d", pos)
		}
		_, err = io.ReadAll(b)
		if err != nil {
			t.Fatalf("readall: %v", err)
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
			t.Fatalf("read err: %v", err)
		}
		if i != bl {
			t.Errorf("read length, expected %d, received %d", bl, i)
		}
		_, err = b.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatalf("seek err: %v", err)
		}
		_, err = io.ReadAll(b)
		if err == nil {
			t.Fatalf("readall did not fail")
		}
		if !errors.Is(err, errs.ErrSizeLimitExceeded) {
			t.Errorf("unexpected error on readall, expected %v, received %v", errs.ErrSizeLimitExceeded, err)
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(bytes.NewReader(exBlob)),
			WithHeader(exHeaders),
		)
		chunkCount := 4
		chunkLen := exLen / int64(chunkCount)
		chunkLast := exLen - (chunkLen * int64(chunkCount-1))
		var wg sync.WaitGroup
		wg.Add(2)
		// run multiple read and seek (cur position) in goroutines
		go func() {
			defer wg.Done()
			out := make([]byte, exLen)
			for i := 0; i < chunkCount-1; i++ {
				l, err := b.Read(out[i*int(chunkLen) : (i+1)*int(chunkLen)])
				if l != int(chunkLen) {
					t.Errorf("did not read enough bytes: expected %d, received %d", chunkLen, l)
				}
				if err != nil {
					t.Errorf("read failed: %v", err)
				}
			}
			l, err := b.Read(out[(chunkCount-1)*int(chunkLen):])
			if l != int(chunkLast) {
				t.Errorf("did not read enough bytes: expected %d, received %d", chunkLast, l)
			}
			if err != nil {
				t.Errorf("read failed: %v", err)
			}
			if !bytes.Equal(out, exBlob) {
				t.Errorf("read mismatch, expected output %s, received %s", exBlob, out)
			}
		}()
		go func() {
			defer wg.Done()
			for {
				cur, err := b.Seek(0, io.SeekCurrent)
				if err != nil {
					t.Errorf("failed to seek blob: %v", err)
				}
				if cur >= exLen {
					break
				}
			}
		}()
		// wait for both to finish
		wg.Wait()
	})

	t.Run("ociconfig", func(t *testing.T) {
		// create blob
		b := NewReader(
			WithReader(bytes.NewReader(exBlob)),
			WithDesc(descriptor.Descriptor{
				MediaType: exMT,
				Digest:    exDigest,
				Size:      exLen,
			}),
			WithRef(exRef),
		)
		// test ToOCIConfig on blob 2
		oc, err := b.ToOCIConfig()
		if err != nil {
			t.Fatalf("ToOCIConfig: %v", err)
		}
		if exDigest != oc.GetDescriptor().Digest {
			t.Errorf("digest, expected %s, received %s", exDigest, oc.GetDescriptor().Digest)
		}
		ocb, err := oc.RawBody()
		if err != nil {
			t.Fatalf("config rawbody: %v", err)
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
			t.Fatalf("rawbody: %v", err)
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
		t.Fatalf("failed to unmarshal exBlob: %v", err)
	}
	tt := []struct {
		name     string
		opts     []Opts
		fromJSON []byte
		wantRaw  []byte
		wantJSON []byte
		wantDesc descriptor.Descriptor
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
			wantDesc: descriptor.Descriptor{MediaType: mediatype.OCI1ImageConfig},
		},
		{
			name: "Config with Docker Desc",
			opts: []Opts{
				WithImage(ociConfig),
				WithDesc(exDesc),
			},
			wantDesc: descriptor.Descriptor{MediaType: exMT},
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
			WithDesc(descriptor.Descriptor{
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
		t.Fatalf("failed to open test data: %v", err)
	}
	digger := digest.Canonical.Digester()
	fhSize, err := io.Copy(digger.Hash(), fh)
	if err != nil {
		t.Fatalf("failed to build digest on test data: %v", err)
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
				WithDesc(descriptor.Descriptor{
					MediaType: mediatype.OCI1Layer,
					Size:      fhSize,
					Digest:    dig,
				}),
			},
		},
		{
			name: "bad desc",
			opts: []Opts{
				WithDesc(descriptor.Descriptor{
					MediaType: mediatype.OCI1Layer,
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
				t.Fatalf("failed to open test data: %v", err)
			}
			opts := append(tc.opts, WithReader(fh))
			btr := NewTarReader(opts...)
			tr, err := btr.GetTarReader()
			if err != nil {
				t.Fatalf("failed to get tar reader: %v", err)
			}
			for {
				th, err := tr.Next()
				if err != nil {
					if err != io.EOF {
						t.Fatalf("failed to read tar: %v", err)
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
			expectErr: errs.ErrFileDeleted,
		},
		{
			name:     "layer3",
			filename: "layer3.txt",
			content:  "3\n",
		},
		{
			name:      "opaque dir",
			filename:  "exdir/test.txt",
			expectErr: errs.ErrFileDeleted,
		},
		{
			name:      "missing",
			filename:  "missing.txt",
			expectErr: errs.ErrFileNotFound,
		},
		{
			name:      "invalid",
			filename:  ".wh.filename.txt",
			expectErr: fmt.Errorf(".wh. prefix is reserved for whiteout files"),
		},
	}
	fileBytes, err := os.ReadFile(fileLayerWH)
	if err != nil {
		t.Fatalf("failed to open test data: %v", err)
	}
	blobDigest := digest.FromBytes(fileBytes)
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			fh, err := os.Open(fileLayerWH)
			if err != nil {
				t.Fatalf("failed to open test data: %v", err)
			}
			btr := NewTarReader(WithReader(fh), WithDesc(descriptor.Descriptor{Size: int64(len(fileBytes)), Digest: blobDigest, MediaType: mediatype.OCI1Layer}))
			defer btr.Close()
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
				t.Fatalf("ReadFile failed: %v", err)
			}
			if th == nil {
				t.Fatalf("tar header is nil")
			}
			if rdr == nil {
				t.Fatalf("reader is nil")
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
			t.Fatalf("failed to open test data: %v", err)
		}
		btr := NewTarReader(WithReader(fh), WithDesc(descriptor.Descriptor{Size: int64(len(fileBytes)), Digest: digest.FromString("bad digest"), MediaType: mediatype.OCI1Layer}))
		_, _, err = btr.ReadFile("missing.txt")
		if err == nil {
			t.Errorf("ReadFile did not fail")
		} else if !errors.Is(err, errs.ErrDigestMismatch) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrDigestMismatch, err)
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
