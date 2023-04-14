package regclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func TestBlobGet(t *testing.T) {
	blobRepo := "/proj/repo"
	privateRepo := "/proj/private"
	ctx := context.Background()
	// include a random blob
	seed := time.Now().UTC().Unix()
	t.Logf("Using seed %d", seed)
	blobLen := 1024 // must be greater than 512 for retry test
	d1, blob1 := reqresp.NewRandomBlob(blobLen, seed)
	d2, blob2 := reqresp.NewRandomBlob(blobLen, seed+1)
	bMissing := []byte("missing")
	dMissing := digest.FromBytes(bMissing)
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
	// setup the regclient
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []config.Host{
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
	rc := New(
		WithConfigHost(rcHosts...),
		WithLog(log),
		WithRetryDelay(delayInit, delayMax),
	)
	// Test successful blob
	t.Run("Get", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br, err := rc.BlobGet(ctx, ref, types.Descriptor{Digest: d1, Size: int64(len(blob1))})
		if err != nil {
			t.Errorf("Failed running BlobGet: %v", err)
			return
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Errorf("Failed reading blob: %v", err)
			return
		}
		if !bytes.Equal(blob1, brBlob) {
			t.Errorf("Blob does not match")
		}
	})

	// Test data
	t.Run("Data", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo + "/data")
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		desc := types.Descriptor{
			Digest: d1,
			Size:   int64(len(blob1)),
			Data:   blob1,
		}
		br, err := rc.BlobGet(ctx, ref, desc)
		if err != nil {
			t.Errorf("Failed running BlobGet: %v", err)
			return
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Errorf("Failed reading blob: %v", err)
			return
		}
		if !bytes.Equal(blob1, brBlob) {
			t.Errorf("Blob does not match")
		}
	})

	t.Run("Head", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br, err := rc.BlobHead(ctx, ref, types.Descriptor{Digest: d1})
		if err != nil {
			t.Errorf("Failed running BlobHead: %v", err)
			return
		}
		defer br.Close()
		if br.GetDescriptor().Size != int64(blobLen) {
			t.Errorf("Failed comparing blob length")
		}
	})

	t.Run("Missing", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br, err := rc.BlobGet(ctx, ref, types.Descriptor{Digest: dMissing})
		if err == nil {
			defer br.Close()
			t.Errorf("Unexpected success running BlobGet")
			return
		}
		if !errors.Is(err, types.ErrNotFound) {
			t.Errorf("Error does not match \"ErrNotFound\": %v", err)
		}
	})

	t.Run("Retry", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br, err := rc.BlobGet(ctx, ref, types.Descriptor{Digest: d2})
		if err != nil {
			t.Errorf("Failed running BlobGet: %v", err)
			return
		}
		defer br.Close()
		brBlob, err := io.ReadAll(br)
		if err != nil {
			t.Errorf("Failed reading blob: %v", err)
			return
		}
		if !bytes.Equal(blob2, brBlob) {
			t.Errorf("Blob does not match")
		}
	})

	t.Run("Forbidden", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + privateRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br, err := rc.BlobGet(ctx, ref, types.Descriptor{Digest: d1})
		if err == nil {
			defer br.Close()
			t.Errorf("Unexpected success running BlobGet")
			return
		}
		if !errors.Is(err, types.ErrHTTPUnauthorized) {
			t.Errorf("Error does not match \"ErrUnauthorized\": %v", err)
		}
	})

}

func TestBlobPut(t *testing.T) {
	blobRepo := "/proj/repo"
	// privateRepo := "/proj/private"
	ctx := context.Background()
	// include a random blob
	seed := time.Now().UTC().Unix()
	t.Logf("Using seed %d", seed)
	blobChunk := 512
	blobLen := 1024  // must be blobChunk < blobLen <= blobChunk * 2
	blobLen3 := 1000 // blob without a full final chunk
	d1, blob1 := reqresp.NewRandomBlob(blobLen, seed)
	uuid1 := uuid.New()
	d2, blob2 := reqresp.NewRandomBlob(blobLen, seed+1)
	uuid2 := uuid.New()
	d3, blob3 := reqresp.NewRandomBlob(blobLen3, seed+2)
	uuid3 := uuid.New()
	// dMissing := digest.FromBytes([]byte("missing"))
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
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid1.String()},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for d1",
				Method: "PUT",
				Path:   "/v2" + blobRepo + "/blobs/uploads/" + uuid1.String(),
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
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepo + "/blobs/" + d1.String()},
					"Docker-Content-Digest": {d1.String()},
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
					"Location":       {uuid2.String()},
				},
			},
		},
		// upload put for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for patched d2",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2.String(),
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
		// upload patch 2 fail for d2
		// {
		// 	ReqEntry: reqresp.ReqEntry{
		// 		DelOnUse: true,
		// 		Name:     "PATCH 2 fail for d2",
		// 		Method:   "PATCH",
		// 		Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2.String(),
		// 		Query: map[string][]string{
		// 			"chunk": {"2"},
		// 		},
		// 		Headers: http.Header{
		// 			"Content-Length": {fmt.Sprintf("%d", blobLen-blobChunk)},
		// 			"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk, blobLen)},
		// 			"Content-Type":   {"application/octet-stream"},
		// 		},
		// 		Body: blob2[blobChunk:],
		// 	},
		// 	RespEntry: reqresp.RespEntry{
		// 		Status: http.StatusGatewayTimeout,
		// 		Headers: http.Header{
		// 			"Content-Length": {fmt.Sprintf("%d", 0)},
		// 		},
		// 	},
		// },
		// upload patch 2 for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2 for d2",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2.String(),
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
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen-1)},
					"Location":       {uuid2.String() + "?chunk=3"},
				},
			},
		},
		// upload patch 1 for d2
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 1 for d2",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2.String(),
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
					"Location":       {uuid2.String() + "?chunk=2"},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for d2",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid2.String(),
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
					"Location":       {uuid3.String()},
				},
			},
		},
		// upload put for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for patched d3",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3.String(),
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
		// upload patch 2 fail for d3
		// {
		// 	ReqEntry: reqresp.ReqEntry{
		// 		DelOnUse: true,
		// 		Name:     "PATCH 2 fail for d3",
		// 		Method:   "PATCH",
		// 		Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3.String(),
		// 		Query: map[string][]string{
		// 			"chunk": {"2"},
		// 		},
		// 		Headers: http.Header{
		// 			"Content-Length": {fmt.Sprintf("%d", blobLen3-blobChunk)},
		// 			"Content-Range":  {fmt.Sprintf("%d-%d", blobChunk, blobLen3)},
		// 			"Content-Type":   {"application/octet-stream"},
		// 		},
		// 		Body: blob2[blobChunk:],
		// 	},
		// 	RespEntry: reqresp.RespEntry{
		// 		Status: http.StatusGatewayTimeout,
		// 		Headers: http.Header{
		// 			"Content-Length": {fmt.Sprintf("%d", 0)},
		// 		},
		// 	},
		// },
		// upload patch 2 for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 2 for d3",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3.String(),
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
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", 0)},
					"Range":          {fmt.Sprintf("bytes=0-%d", blobLen3-1)},
					"Location":       {uuid3.String() + "?chunk=3"},
				},
			},
		},
		// upload patch 1 for d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PATCH 1 for d3",
				Method:   "PATCH",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3.String(),
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
					"Location":       {uuid3.String() + "?chunk=2"},
				},
			},
		},
		// upload blob d3
		{
			ReqEntry: reqresp.ReqEntry{
				DelOnUse: false,
				Name:     "PUT for d3",
				Method:   "PUT",
				Path:     "/v2" + blobRepo + "/blobs/uploads/" + uuid3.String(),
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
	}
	rrs = append(rrs, reqresp.BaseEntries...)
	// create a server
	ts := httptest.NewServer(reqresp.NewHandler(t, rrs))
	defer ts.Close()
	// setup the regclient
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []config.Host{
		{
			Name:      tsHost,
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
	rc := New(
		WithConfigHost(rcHosts...),
		WithLog(log),
		WithRetryDelay(delayInit, delayMax),
	)

	t.Run("Put", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob1)
		dp, err := rc.BlobPut(ctx, ref, types.Descriptor{Digest: d1, Size: int64(len(blob1))}, br)
		if err != nil {
			t.Errorf("Failed running BlobPut: %v", err)
			return
		}
		if dp.Digest.String() != d1.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d1.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob1)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob1), dp.Size)
		}

	})

	t.Run("Retry", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob2)
		dp, err := rc.BlobPut(ctx, ref, types.Descriptor{Digest: d2, Size: int64(len(blob2))}, br)
		if err != nil {
			t.Errorf("Failed running BlobPut: %v", err)
			return
		}
		if dp.Digest.String() != d2.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d2.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob2)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob2), dp.Size)
		}

	})

	t.Run("PartialChunk", func(t *testing.T) {
		ref, err := ref.New(tsURL.Host + blobRepo)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		br := bytes.NewReader(blob3)
		dp, err := rc.BlobPut(ctx, ref, types.Descriptor{Digest: d3, Size: int64(len(blob3))}, br)
		if err != nil {
			t.Errorf("Failed running BlobPut: %v", err)
			return
		}
		if dp.Digest.String() != d3.String() {
			t.Errorf("Digest mismatch, expected %s, received %s", d3.String(), dp.Digest.String())
		}
		if dp.Size != int64(len(blob3)) {
			t.Errorf("Content length mismatch, expected %d, received %d", len(blob3), dp.Size)
		}

	})
}

func TestBlobCopy(t *testing.T) {
	blobRepoA := "/proj/repo-a"
	blobRepoB := "/proj/repo-b"
	ctx := context.Background()
	blobChunk := 512
	// include a random blob
	seed := time.Now().UTC().Unix()
	t.Logf("Using seed %d", seed)
	blobLen := 1024 // must be greater than 512 for retry test
	d1, blob1 := reqresp.NewRandomBlob(blobLen, seed)
	d2, blob2 := reqresp.NewRandomBlob(blobLen, seed+1)
	d3, blob3 := reqresp.NewRandomBlob(blobLen, seed+2)
	uuid1 := uuid.New()
	uuid2 := uuid.New()
	uuid3 := uuid.New()

	// define req/resp entries
	rrs := []reqresp.ReqResp{
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "HEAD for repo a - d1",
				Method: "HEAD",
				Path:   "/v2" + blobRepoA + "/blobs/" + d1.String(),
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
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "HEAD for repo b - d1",
				Method:  "HEAD",
				Path:    "/v2" + blobRepoB + "/blobs/" + d1.String(),
				IfState: []string{""},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		// get
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET for repo a - d1",
				Method: "GET",
				Path:   "/v2" + blobRepoA + "/blobs/" + d1.String(),
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
		// get upload location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for repo b - d1",
				Method: "POST",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d1.String()},
				},
				IfState: []string{""},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid1.String()},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for repo b - d1",
				Method: "PUT",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/" + uuid1.String(),
				Query: map[string][]string{
					"digest": {d1.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob1))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body:     blob1,
				IfState:  []string{""},
				SetState: "d1",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepoB + "/blobs/" + d1.String()},
					"Docker-Content-Digest": {d1.String()},
				},
			},
		},
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "HEAD for repo b - d1",
				Method:  "HEAD",
				Path:    "/v2" + blobRepoB + "/blobs/" + d1.String(),
				IfState: []string{"d1"},
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
				Name:    "GET for repo b - d1",
				Method:  "GET",
				Path:    "/v2" + blobRepoB + "/blobs/" + d1.String(),
				IfState: []string{"d1"},
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
				Name:   "HEAD for repo a - d2",
				Method: "HEAD",
				Path:   "/v2" + blobRepoA + "/blobs/" + d2.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "HEAD for repo b - d2",
				Method: "HEAD",
				Path:   "/v2" + blobRepoB + "/blobs/" + d2.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		// get
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "GET for repo a - d2 fail",
				Method:   "GET",
				Path:     "/v2" + blobRepoA + "/blobs/" + d2.String(),
				IfState:  []string{"d1"},
				SetState: "d2fail",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob2[:blobChunk],
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d2.String()},
				},
				Fail: true,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "GET for repo a - d2",
				Method:  "GET",
				Path:    "/v2" + blobRepoA + "/blobs/" + d2.String(),
				IfState: []string{"d2fail"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob2,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},
		// get upload location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for repo b - d2",
				Method: "POST",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d2.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid2.String()},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for repo b - d2",
				Method: "PUT",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/" + uuid2.String(),
				Query: map[string][]string{
					"digest": {d2.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob2))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body:     blob2,
				SetState: "d2",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepoB + "/blobs/" + d2.String()},
					"Docker-Content-Digest": {d2.String()},
				},
			},
		},

		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "HEAD for repo a - d3",
				Method: "HEAD",
				Path:   "/v2" + blobRepoA + "/blobs/" + d3.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d3.String()},
				},
			},
		},
		// head
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "HEAD for repo b - d3",
				Method: "HEAD",
				Path:   "/v2" + blobRepoB + "/blobs/" + d3.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		// get
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "GET for repo a - d3",
				Method: "GET",
				Path:   "/v2" + blobRepoA + "/blobs/" + d3.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   blob3,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", blobLen)},
					"Content-Type":          {"application/octet-stream"},
					"Docker-Content-Digest": {d3.String()},
				},
			},
		},
		// get upload location
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for repo b - d3",
				Method: "POST",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/",
				Query: map[string][]string{
					"mount": {d3.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {uuid3.String()},
				},
			},
		},
		// upload blob
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for repo b - d3",
				Method: "PUT",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/" + uuid3.String(),
				Query: map[string][]string{
					"digest": {d3.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob3))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body:    blob3,
				IfState: []string{"d3fail"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepoB + "/blobs/" + d3.String()},
					"Docker-Content-Digest": {d3.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for repo b - d3 fail",
				Method: "PUT",
				Path:   "/v2" + blobRepoB + "/blobs/uploads/" + uuid3.String(),
				Query: map[string][]string{
					"digest": {d3.String()},
				},
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(blob3))},
					"Content-Type":   {"application/octet-stream"},
				},
				Body:     blob3,
				SetState: "d3fail",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length":        {"0"},
					"Location":              {"/v2" + blobRepoB + "/blobs/" + d3.String()},
					"Docker-Content-Digest": {d3.String()},
				},
				Fail: true,
			},
		},
	}
	rrs = append(rrs, reqresp.BaseEntries...)
	// create a server
	ts := httptest.NewServer(reqresp.NewHandler(t, rrs))
	defer ts.Close()
	// setup the regclient
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []config.Host{
		{
			Name:      tsHost,
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
	rc := New(
		WithConfigHost(rcHosts...),
		WithLog(log),
		WithRetryDelay(delayInit, delayMax),
	)

	refA, err := ref.New(tsURL.Host + blobRepoA)
	if err != nil {
		t.Errorf("Failed creating ref: %v", err)
	}
	refB, err := ref.New(tsURL.Host + blobRepoB)
	if err != nil {
		t.Errorf("Failed creating ref: %v", err)
	}

	// same repo
	t.Run("Copy Same Repo", func(t *testing.T) {
		err = rc.BlobCopy(ctx, refA, refA, types.Descriptor{Digest: d1})
		if err != nil {
			t.Errorf("Failed to copy d1 from repo a to a: %v", err)
		}
	})

	// copy blob
	t.Run("Copy Diff Repo", func(t *testing.T) {
		err = rc.BlobCopy(ctx, refA, refB, types.Descriptor{Digest: d1})
		if err != nil {
			t.Errorf("Failed to copy d1 from repo a to b: %v", err)
		}
	})

	// blob exists
	t.Run("Copy Existing Blob", func(t *testing.T) {
		err = rc.BlobCopy(ctx, refA, refB, types.Descriptor{Digest: d1})
		if err != nil {
			t.Errorf("Failed to copy d1 from repo a to b: %v", err)
		}
	})

	// copy fails on get, retry succeeds
	t.Run("Copy Flaky Get", func(t *testing.T) {
		err = rc.BlobCopy(ctx, refA, refB, types.Descriptor{Digest: d2})
		if err != nil {
			t.Errorf("Failed to copy d2 from repo a to b: %v", err)
		}
	})

	// copy fails on put, retry succeeds
	t.Run("Copy Flaky Put", func(t *testing.T) {
		err = rc.BlobCopy(ctx, refA, refB, types.Descriptor{Digest: d3})
		if err != nil {
			t.Errorf("Failed to copy d3 from repo a to b: %v", err)
		}
	})

}
