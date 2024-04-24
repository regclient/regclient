package reg

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
)

func TestBlobGet(t *testing.T) {
	t.Parallel()
	blobRepo := "/proj/repo"
	externalRepo := "/proj/external"
	privateRepo := "/proj/private"
	ctx := context.Background()
	// include a random blob
	seed := time.Now().UTC().Unix()
	t.Logf("Using seed %d", seed)
	blobLen := 1024 // must be greater than 512 for retry test
	d1, blob1 := reqresp.NewRandomBlob(blobLen, seed)
	d2, blob2 := reqresp.NewRandomBlob(blobLen, seed+1)
	dMissing := digest.FromBytes([]byte("missing"))
	blob1Desc := descriptor.Descriptor{
		MediaType: mediatype.OCI1ImageConfig,
		Digest:    d1,
		Size:      int64(len(blob1)),
	}
	// define req/resp entries
	rrs := []reqresp.ReqResp{
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "HEAD for d1",
				Method: "HEAD",
				Path:   "/v2" + blobRepo + "/blobs/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d1.String()},
				},
			},
		},
		// get
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET for d1",
				Method: "GET",
				Path:   "/v2" + blobRepo + "/blobs/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob1,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d1.String()},
				},
			},
		},
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "external direct HEAD for d1",
				Method: "HEAD",
				Path:   "/v2" + externalRepo + "/blobs/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		// get
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "external direct GET for d1",
				Method: "GET",
				Path:   "/v2" + externalRepo + "/blobs/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		}, // external head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "external HEAD for d1",
				Method: "HEAD",
				Path:   "/external/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen)},
					"Content-Type":   {"application/octet-stream"},
				},
			},
		},
		// external get
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "external GET for d1",
				Method: "GET",
				Path:   "/external/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob1,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen)},
					"Content-Type":   {"application/octet-stream"},
				},
			},
		},
		// missing
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET Missing",
				Method: "GET",
				Path:   "/v2" + blobRepo + "/blobs/" + dMissing.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		// TODO: test unauthorized
		// TODO: test range read
		// head for d2
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "HEAD for d2",
				Method: "HEAD",
				Path:   "/v2" + blobRepo + "/blobs/" + d2.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Accept-Ranges":         {"bytes"},
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},
		// get range
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET for d2, range for second part",
				Method: "GET",
				Path:   "/v2" + blobRepo + "/blobs/" + d2.String(),
				Headers: http.Header{
					"Range": {fmt.Sprintf("bytes=512-%d", blobLen)},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob2[512:],
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen-512)},
					"Content-Range":         {fmt.Sprintf("bytes %d-%d/%d", 512, blobLen, blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},
		// get that stops early
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET for d2, short read",
				Method: "GET",
				Path:   "/v2" + blobRepo + "/blobs/" + d2.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob2[0:512],
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},
		// forbidden
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET Forbidden",
				Method: "GET",
				Path:   "/v2" + privateRepo + "/blobs/" + d1.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusForbidden,
			},
		},
	}
	rrs = append(rrs, reqresp.BaseEntries...)
	// create a server
	ts := httptest.NewServer(reqresp.NewHandler(t, rrs))
	defer ts.Close()
	// setup the reg
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []*config.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
	}
	log := &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	reg := New(
		WithConfigHosts(rcHosts),
		WithLog(log),
		WithDelay(delayInit, delayMax),
	)

	// Test successful blob
	t.Run("get-descriptor", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobGet(ctx, r, blob1Desc)
		if err != nil {
			t.Fatalf("Failed running BlobGet: %v", err)
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed reading blob: %v", err)
		}
		if !bytes.Equal(blob1, brBlob) {
			t.Errorf("Blob does not match")
		}
		brDesc := br.GetDescriptor()
		if !brDesc.Equal(blob1Desc) {
			t.Errorf("descriptor mismatch: expect %v, receive %v", blob1Desc, brDesc)
		}
	})

	t.Run("get-digest", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobGet(ctx, r, descriptor.Descriptor{Digest: d1})
		if err != nil {
			t.Fatalf("Failed running BlobGet: %v", err)
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed reading blob: %v", err)
		}
		if !bytes.Equal(blob1, brBlob) {
			t.Errorf("Blob does not match")
		}
		brDesc := br.GetDescriptor()
		if brDesc.Digest != d1 || brDesc.Size != blob1Desc.Size {
			t.Errorf("descriptor mismatch: expect %v, receive %v", blob1Desc, brDesc)
		}
	})

	t.Run("head-descriptor", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobHead(ctx, r, blob1Desc)
		if err != nil {
			t.Fatalf("Failed running BlobHead: %v", err)
		}
		defer br.Close()
		brDesc := br.GetDescriptor()
		if !brDesc.Equal(blob1Desc) {
			t.Errorf("descriptor mismatch: expect %v, receive %v", blob1Desc, brDesc)
		}
	})

	t.Run("head-digest", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobHead(ctx, r, descriptor.Descriptor{Digest: d1})
		if err != nil {
			t.Fatalf("Failed running BlobHead: %v", err)
		}
		defer br.Close()
		brDesc := br.GetDescriptor()
		if brDesc.Digest != d1 || brDesc.Size != blob1Desc.Size {
			t.Errorf("descriptor mismatch: expect %v, receive %v", blob1Desc, brDesc)
		}
	})

	// Test successful blob
	t.Run("External Get", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + externalRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobGet(ctx, r, descriptor.Descriptor{Digest: d1, URLs: []string{tsURL.Scheme + "://" + tsURL.Host + "/external/" + d1.String()}})
		if err != nil {
			t.Fatalf("Failed running external BlobGet: %v", err)
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed reading external blob: %v", err)
		}
		if !bytes.Equal(blob1, brBlob) {
			t.Errorf("External blob does not match")
		}
	})

	t.Run("External Head", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + externalRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobHead(ctx, r, descriptor.Descriptor{Digest: d1, URLs: []string{tsURL.Scheme + "://" + tsURL.Host + "/external/" + d1.String()}})
		if err != nil {
			t.Fatalf("Failed running external BlobHead: %v", err)
		}
		defer br.Close()
		if br.GetDescriptor().Size != int64(blobLen) {
			t.Errorf("Failed comparing external blob length")
		}
	})

	t.Run("Missing", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobGet(ctx, r, descriptor.Descriptor{Digest: dMissing})
		if err == nil {
			defer br.Close()
			t.Fatalf("Unexpected success running BlobGet")
		}
		if !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("Error does not match \"ErrNotFound\": %v", err)
		}
	})

	t.Run("Retry", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobGet(ctx, r, descriptor.Descriptor{Digest: d2})
		if err != nil {
			t.Fatalf("Failed running BlobGet: %v", err)
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed reading blob: %v", err)
		}
		if !bytes.Equal(blob2, brBlob) {
			t.Errorf("Blob does not match")
		}
	})

	t.Run("Forbidden", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + privateRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br, err := reg.BlobGet(ctx, r, descriptor.Descriptor{Digest: d1})
		if err == nil {
			defer br.Close()
			t.Fatalf("Unexpected success running BlobGet")
		}
		if !errors.Is(err, errs.ErrHTTPUnauthorized) {
			t.Errorf("Error does not match \"ErrUnauthorized\": %v", err)
		}
	})
}

func TestBlobPut(t *testing.T) {
	t.Parallel()
	blobRepo := "/proj/repo"
	blobRepo5 := "/proj/repo5"
	blobRepo6 := "/proj/repo6"
	// privateRepo := "/proj/private"
	ctx := context.Background()
	// include a random blob
	seed := time.Now().UTC().Unix()
	t.Logf("Using seed %d", seed)
	blobChunk := 512
	blobLen := 1024  // must be blobChunk < blobLen <= blobChunk * 2
	blobLen3 := 1000 // blob without a full final chunk
	blobLen4 := 2048 // must be blobChunk < blobLen <= blobChunk * 2
	blobLen5 := 500  // single chunk
	d1, blob1 := reqresp.NewRandomBlob(blobLen, seed)
	d2, blob2 := reqresp.NewRandomBlob(blobLen, seed+1)
	d2Bad := digest.Canonical.FromString("digest 2 bad")
	d3, blob3 := reqresp.NewRandomBlob(blobLen3, seed+2)
	d4, blob4 := reqresp.NewRandomBlob(blobLen4, seed+3)
	d5, blob5 := reqresp.NewRandomBlob(blobLen5, seed+4)
	blob6 := []byte{}
	d6 := digest.Canonical.FromBytes(blob6)
	uuid1 := reqresp.NewRandomID(seed + 10)
	uuid2 := reqresp.NewRandomID(seed + 11)
	uuid2Bad := reqresp.NewRandomID(seed + 12)
	uuid3 := reqresp.NewRandomID(seed + 13)
	uuid4 := reqresp.NewRandomID(seed + 14)
	uuid5 := reqresp.NewRandomID(seed + 15)
	uuid6 := reqresp.NewRandomID(seed + 16)
	// dMissing := digest.FromBytes([]byte("missing"))
	user := "testing"
	pass := "password"

	// create an external blob server (e.g. S3 storage)
	blobRRS := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for d1",
				Method: "PUT",
				Path:   "/v2" + blobRepo + "/blobs/uploads/" + uuid1,
				Query: map[string][]string{
					"digest": {d1.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob1))},
					"Content-Type":   {"application/octet-stream"},
					"Authorization":  {fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))},
				},
				Body: blob1,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo + "/blobs/" + d1.String()},
					"Docker-Content-Digest": {d1.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for d1 unauth",
				Method: "PUT",
				Path:   "/v2" + blobRepo + "/blobs/uploads/" + uuid1,
				Query: map[string][]string{
					"digest": {d1.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob1))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob1,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Headers: http.Header{
					"WWW-Authenticate": {"Basic realm=\"testing\""},
				},
			},
		},
	}
	blobTS := httptest.NewServer(reqresp.NewHandler(t, blobRRS))
	defer blobTS.Close()
	blobURL, _ := url.Parse(blobTS.URL)
	blobHost := blobURL.Host

	// define req/resp entries
	rrs := []reqresp.ReqResp{
		// get upload location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d1",
				Method: "POST",
				Path:   "/v2" + blobRepo + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d1.String()},
				},
				Headers: http.Header{
					"Authorization": {fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {fmt.Sprintf("http://%s/v2%s/blobs/uploads/%s", blobHost, blobRepo, uuid1)},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d1 unauth",
				Method: "POST",
				Path:   "/v2" + blobRepo + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d1.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Headers: http.Header{
					"Content-Length":   {"0"},
					"WWW-Authenticate": {"Basic realm=\"testing\""},
				},
			},
		},
		// get upload2 location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d2",
				Method: "POST",
				Path:   "/v2" + blobRepo + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d2.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid2},
				},
			},
		},
		// upload put for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for patched d2",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2,
				Query: map[string][]string{
					"digest": {d2.String()},
					"chunk":  {"3"},
				},
				Headers: http.Header{
					"Content-Length": {"0"},
					"Content-Type":   {"application/octet-stream"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo + "/blobs/" + d2.String()},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},
		// upload patch 2b for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2b for d2",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2,
				Query: map[string][]string{
					"chunk": {"2b"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen-blobChunk-20)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk+20, blobLen-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2[blobChunk+20:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen-1)},
					"Location":       {uuid2 + "?chunk=3"},
				},
			},
		},
		// upload patch 2 for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2 for d2",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2,
				Query: map[string][]string{
					"chunk": {"2"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen-blobChunk)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk, blobLen-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2[blobChunk:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusRequestedRangeNotSatisfiable,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobChunk+20-1)},
					"Location":       {uuid2 + "?chunk=2b"},
				},
			},
		},
		// upload patch 1 for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 1 for d2",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2,
				Query:    map[string][]string{},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobChunk)},
					"Content-Range":  {fmt.Sprintf("0-%d", blobChunk-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2[0:blobChunk],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobChunk-1)},
					"Location":       {uuid2 + "?chunk=2"},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for d2",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2,
				Query: map[string][]string{
					"digest": {d2.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob2))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusGatewayTimeout,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
				},
			},
		},
		// cancel the d2 upload
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "DELETE for d2",
				Method:   "DELETE",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
		// get upload2 location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d2Bad",
				Method: "POST",
				Path:   "/v2" + blobRepo + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d2Bad.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid2Bad},
				},
			},
		},
		// upload put for d2Bad
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for patched d2Bad",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2Bad,
				Query: map[string][]string{
					"digest": {d2Bad.String()},
					"chunk":  {"3"},
				},
				Headers: http.Header{
					"Content-Length": {"0"},
					"Content-Type":   {"application/octet-stream"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadRequest,
				Headers: http.Header{
					"Content-Length": {"0"},
					// "Location":              {"/v2" + blobRepo + "/blobs/" + d2Bad.String()},
					// "Docker-Content-Digest": {d2Bad.String()},
				},
			},
		},
		// upload patch 2b for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2b for d2Bad",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2Bad,
				Query: map[string][]string{
					"chunk": {"2b"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen-blobChunk-20)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk+20, blobLen-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2[blobChunk+20:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen-1)},
					"Location":       {uuid2Bad + "?chunk=3"},
				},
			},
		},
		// upload patch 2 for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2 for d2Bad",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2Bad,
				Query: map[string][]string{
					"chunk": {"2"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen-blobChunk)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk, blobLen-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2[blobChunk:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusRequestedRangeNotSatisfiable,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobChunk+20-1)},
					"Location":       {uuid2Bad + "?chunk=2b"},
				},
			},
		},
		// upload patch 1 for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 1 for d2Bad",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2Bad,
				Query:    map[string][]string{},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobChunk)},
					"Content-Range":  {fmt.Sprintf("0-%d", blobChunk-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2[0:blobChunk],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobChunk-1)},
					"Location":       {uuid2Bad + "?chunk=2"},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for d2Bad",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2Bad,
				Query: map[string][]string{
					"digest": {d2Bad.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob2))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob2,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusGatewayTimeout,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "DELETE for d2Bad",
				Method:   "DELETE",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2Bad,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},

		// get upload3 location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d3",
				Method: "POST",
				Path:   "/v2" + blobRepo + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d3.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid3},
				},
			},
		},
		// upload put for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for patched d3",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3,
				Query: map[string][]string{
					"digest": {d3.String()},
					"chunk":  {"3"},
				},
				Headers: http.Header{
					"Content-Length": {"0"},
					"Content-Type":   {"application/octet-stream"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo + "/blobs/" + d3.String()},
					"Docker-Content-Digest": {d3.String()},
				},
			},
		},
		// upload patch 2 for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2 for d3",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3,
				Query: map[string][]string{
					"chunk": {"2"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen3-blobChunk)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk, blobLen3-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob3[blobChunk:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusTooManyRequests,
			},
		},
		// get status for d3 after failed attempt of chunk 2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "GET 2 for d3",
				Method:   "GET",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3,
				Query: map[string][]string{
					"chunk": {"2"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNoContent,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobChunk-1)},
					"Location":       {uuid3 + "?chunk=2b"},
				},
			},
		},
		// upload patch 2b for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2b for d3",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3,
				Query: map[string][]string{
					"chunk": {"2b"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen3-blobChunk)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk, blobLen3-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob3[blobChunk:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen3-1)},
					"Location":       {uuid3 + "?chunk=3"},
				},
			},
		},
		// upload patch 1 for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 1 for d3",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3,
				Query:    map[string][]string{},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobChunk)},
					"Content-Range":  {fmt.Sprintf("0-%d", blobChunk-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob3[0:blobChunk],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobChunk-1)},
					"Location":       {uuid3 + "?chunk=2"},
				},
			},
		},
		// upload blob d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for d3",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3,
				Query: map[string][]string{
					"digest": {d3.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob3))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob3,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusGatewayTimeout,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
				},
			},
		},

		// get upload4 location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d4",
				Method: "POST",
				Path:   "/v2" + blobRepo + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d4.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length":   {"0"},
					"Location":         {uuid4},
					blobChunkMinHeader: {fmt.Sprintf("%d", blobLen4/2)},
				},
			},
		},
		// upload put for d4
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for patched d4",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid4,
				Query: map[string][]string{
					"digest": {d4.String()},
					"chunk":  {"3"},
				},
				Headers: http.Header{
					"Content-Length": {"0"},
					"Content-Type":   {"application/octet-stream"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo + "/blobs/" + d4.String()},
					"Docker-Content-Digest": {d4.String()},
				},
			},
		},
		// upload patch 2 for d4
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2 for d4",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid4,
				Query: map[string][]string{
					"chunk": {"2"},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen4/2)},
					"Content-Range":  {fmt.Sprintf("%d-%d", blobLen4/2, blobLen4-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob4[blobLen4/2:],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen4-1)},
					"Location":       {uuid4 + "?chunk=3"},
				},
			},
		},
		// upload patch 1 for d4
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 1 for d4",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid4,
				Query:    map[string][]string{},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen4/2)},
					"Content-Range":  {fmt.Sprintf("0-%d", blobLen4/2-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob4[0 : blobLen4/2],
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen4/2-1)},
					"Location":       {uuid4 + "?chunk=2"},
				},
			},
		},
		// get upload5 location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d5",
				Method: "POST",
				Path:   "/v2" + blobRepo5 + "/blobs/uploads/",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid5},
				},
			},
		},
		// upload put for d5
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for chunked d5",
				Method:   "PUT",
				Path:     "/v2" + blobRepo5 + "/blobs/uploads/" + uuid5,
				Query: map[string][]string{
					"digest": {d5.String()},
					"chunk":  {"1"},
				},
				Headers: http.Header{
					"Content-Length": {"0"},
					"Content-Type":   {"application/octet-stream"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo5 + "/blobs/" + d5.String()},
					"Docker-Content-Digest": {d5.String()},
				},
			},
		},
		// upload patch d5
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH for d5",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo5 + "/blobs/uploads/" + uuid5,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", blobLen5)},
					"Content-Range":  {fmt.Sprintf("%d-%d", 0, blobLen5-1)},
					"Content-Type":   {"application/octet-stream"},
				},
				Body: blob5,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen5-1)},
					"Location":       {uuid5 + "?chunk=1"},
				},
			},
		},
		// get upload6 location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for d6",
				Method: "POST",
				Path:   "/v2" + blobRepo6 + "/blobs/uploads/",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid6},
				},
			},
		},
		// upload put for d6
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for d6",
				Method:   "PUT",
				Path:     "/v2" + blobRepo6 + "/blobs/uploads/" + uuid6,
				Query: map[string][]string{
					"digest": {d6.String()},
				},
				Headers: http.Header{
					"Content-Length": {"0"},
					"Content-Type":   {"application/octet-stream"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo6 + "/blobs/" + d6.String()},
					"Docker-Content-Digest": {d6.String()},
				},
			},
		},
	}
	rrs = append(rrs, reqresp.BaseEntries...)
	// create a server
	ts := httptest.NewServer(reqresp.NewHandler(t, rrs))
	defer ts.Close()
	// setup the reg
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []*config.Host{
		{
			Name:      tsHost,
			Hostname:  tsHost,
			TLS:       config.TLSDisabled,
			BlobChunk: int64(blobChunk),
			BlobMax:   int64(-1),
			User:      user,
			Pass:      pass,
		},
		{
			Name:      "chunked." + tsHost,
			Hostname:  tsHost,
			TLS:       config.TLSDisabled,
			BlobChunk: int64(blobChunk),
			BlobMax:   int64(blobChunk * 3),
			User:      user,
			Pass:      pass,
		},
		{
			Name:      "retry." + tsHost,
			Hostname:  tsHost,
			TLS:       config.TLSDisabled,
			BlobChunk: int64(blobChunk),
			BlobMax:   int64(-1),
		},
	}
	log := &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}
	// use short delays for fast tests
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	reg := New(
		WithConfigHosts(rcHosts),
		WithLog(log),
		WithDelay(delayInit, delayMax),
	)

	t.Run("Put", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob1)
		dp, err := reg.BlobPut(ctx, r, descriptor.Descriptor{Digest: d1, Size: int64(len(blob1))}, br)
		if err != nil {
			t.Fatalf("Failed running BlobPut: %v", err)
		}
		if dp.Digest.String() != d1.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d1.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob1)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob1), dp.Size)
		}
	})

	t.Run("Retry", func(t *testing.T) {
		r, err := ref.New("retry." + tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob2)
		mt := "application/vnd.example.test"
		dp, err := reg.BlobPut(ctx, r, descriptor.Descriptor{MediaType: mt, Digest: d2, Size: int64(len(blob2))}, br)
		if err != nil {
			t.Fatalf("Failed running BlobPut: %v", err)
		}
		if dp.Digest.String() != d2.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d2.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob2)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob2), dp.Size)
		}
		if dp.MediaType != mt {
			t.Errorf("Blob put did not preserve descriptor media type: expected %s, received %s", mt, dp.MediaType)
		}
	})

	t.Run("Not retryable", func(t *testing.T) {
		r, err := ref.New("retry." + tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob2)
		_, err = reg.BlobPut(ctx, r, descriptor.Descriptor{Digest: d2, Size: int64(len(blob2))}, io.NopCloser(br))
		if err == nil {
			t.Fatalf("Blob put succeeded on a gateway timeout")
		}
		if !errors.Is(err, errs.ErrHTTPStatus) {
			t.Errorf("unexpected err, expected %v, received %v", errs.ErrHTTPStatus, err)
		}
	})

	t.Run("Invalid digest", func(t *testing.T) {
		r, err := ref.New("retry." + tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob2)
		mt := "application/vnd.example.test"
		_, err = reg.BlobPut(ctx, r, descriptor.Descriptor{MediaType: mt, Digest: d2Bad, Size: int64(len(blob2))}, br)
		if err == nil || !errors.Is(err, errs.ErrDigestMismatch) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrDigestMismatch, err)
		}
	})

	t.Run("Invalid size", func(t *testing.T) {
		r, err := ref.New("retry." + tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob2)
		mt := "application/vnd.example.test"
		_, err = reg.BlobPut(ctx, r, descriptor.Descriptor{MediaType: mt, Digest: d2, Size: int64(len(blob2) - 2)}, br)
		if err == nil || !errors.Is(err, errs.ErrMismatch) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrMismatch, err)
		}
	})

	t.Run("PartialChunk", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob3)
		dp, err := reg.BlobPut(ctx, r, descriptor.Descriptor{Digest: d3, Size: int64(len(blob3))}, br)
		if err != nil {
			t.Fatalf("Failed running BlobPut: %v", err)
		}
		if dp.Digest.String() != d3.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d3.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob3)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob3), dp.Size)
		}
	})

	t.Run("Chunk resized", func(t *testing.T) {
		r, err := ref.New("chunked." + tsURL.Host + blobRepo)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob4)
		dp, err := reg.BlobPut(ctx, r, descriptor.Descriptor{Digest: d4, Size: int64(len(blob4))}, br)
		if err != nil {
			t.Fatalf("Failed running BlobPut: %v", err)
		}
		if dp.Digest.String() != d4.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d4.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob4)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob4), dp.Size)
		}
	})

	// test put without a descriptor
	t.Run("No descriptor", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo5)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob5)
		dp, err := reg.BlobPut(ctx, r, descriptor.Descriptor{}, br)
		if err != nil {
			t.Fatalf("Failed running BlobPut: %v", err)
		}
		if dp.Digest.String() != d5.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d5.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob5)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob5), dp.Size)
		}
	})

	// test put of a zero length blob
	t.Run("Empty blob", func(t *testing.T) {
		r, err := ref.New(tsURL.Host + blobRepo6)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob6)
		dp, err := reg.BlobPut(ctx, r, descriptor.Descriptor{Digest: d6, Size: int64(len(blob6))}, br)
		if err != nil {
			t.Fatalf("Failed running BlobPut: %v", err)
		}
		if dp.Digest.String() != d6.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d6.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob6)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob6), dp.Size)
		}
	})

	// TODO: test failed mount (blobGetUploadURL)
}
