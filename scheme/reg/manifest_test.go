package reg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	repoPath := "/proj"
	getTag256 := "get256"
	getTag512 := "get512"
	bigTag := "big"
	shortReadTag := "short"
	headTag := "head"
	noheadTag := "nohead"
	missingTag := "missing"
	putTag256 := "put256"
	putTag512 := "put512"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	m := schema2.Manifest{
		Versioned: schema2.ManifestSchemaVersion,
		Config: descriptor.Descriptor{
			MediaType: mediatype.Docker2ImageConfig,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []descriptor.Descriptor{
			{
				MediaType: mediatype.Docker2LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
	}
	mBody, err := json.Marshal(m)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	padding := bytes.Repeat([]byte(" "), defaultManifestMaxPull)
	mBodyBig := append(mBody, padding...)
	mDigest256 := digest.SHA256.FromBytes(mBody)
	mDigest512 := digest.SHA512.FromBytes(mBody)
	mLen := len(mBody)
	ctx := context.Background()
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get tag",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + getTag256,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get 512 tag",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + getTag512,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest512.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get 256 digest",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest256.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get 512 digest",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest512.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest512.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + headTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest256.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head 512",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest512.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest512.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get nohead",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + noheadTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Large Manifest",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + bigTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen+defaultManifestMaxPull)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
				Body: mBodyBig,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Short Length",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + shortReadTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen+10)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Missing",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + missingTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Put tag 256",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + putTag256,
				Headers: http.Header{
					"Content-Type":   []string{mediatype.Docker2Manifest},
					"Content-Length": {fmt.Sprintf("%d", mLen)},
				},
				Body: mBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Put tag 512",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + putTag512,
				Query: map[string][]string{
					paramManifestDigest: {mDigest512.String()},
				},
				Headers: http.Header{
					"Content-Type":   []string{mediatype.Docker2Manifest},
					"Content-Length": {fmt.Sprintf("%d", mLen)},
				},
				Body: mBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Docker-Content-Digest": []string{mDigest512.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Put digest 256",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest256.String(),
				Headers: http.Header{
					"Content-Type":   []string{mediatype.Docker2Manifest},
					"Content-Length": {fmt.Sprintf("%d", mLen)},
				},
				Body: mBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Docker-Content-Digest": []string{mDigest256.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Put digest 512",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest512.String(),
				Headers: http.Header{
					"Content-Type":   []string{mediatype.Docker2Manifest},
					"Content-Length": {fmt.Sprintf("%d", mLen)},
				},
				Body: mBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Docker-Content-Digest": []string{mDigest512.String()},
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
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     "missing." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     "nohead." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			APIOpts: map[string]string{
				"disableHead": "true",
			},
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	reg := New(
		WithConfigHosts(rcHosts),
		WithSlog(log),
		WithDelay(delayInit, delayMax),
		WithRetryLimit(3),
	)
	regCache := New(
		WithConfigHosts(rcHosts),
		WithSlog(log),
		WithDelay(delayInit, delayMax),
		WithCache(time.Minute*5, 500),
	)

	t.Run("Get", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + ":" + getTag256)
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mGet, err := reg.ManifestGet(ctx, getRef)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if manifest.GetMediaType(mGet) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest256 {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head", func(t *testing.T) {
		headRef, err := ref.New(tsURL.Host + repoPath + ":" + headTag)
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mHead, err := reg.ManifestHead(ctx, headRef)
		if err != nil {
			t.Fatalf("Failed running ManifestHead: %v", err)
		}
		if manifest.GetMediaType(mHead) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mHead))
		}
		if mHead.GetDescriptor().Digest != mDigest256 {
			t.Errorf("Unexpected digest: %s", mHead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head No Head", func(t *testing.T) {
		noheadRef, err := ref.New("nohead." + tsURL.Host + repoPath + ":" + noheadTag)
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mNohead, err := reg.ManifestHead(ctx, noheadRef)
		if err == nil {
			t.Errorf("Unexpected successful head on \"no head\" registry: %v", mNohead)
		} else if !errors.Is(err, errs.ErrUnsupportedAPI) {
			t.Errorf("Expected error, expected %v, received %v", errs.ErrUnsupportedAPI, err)
		}
	})
	t.Run("Get No Head", func(t *testing.T) {
		noheadRef, err := ref.New("nohead." + tsURL.Host + repoPath + ":" + noheadTag)
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mNohead, err := reg.ManifestGet(ctx, noheadRef)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if manifest.GetMediaType(mNohead) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mNohead))
		}
		if mNohead.GetDescriptor().Digest != mDigest256 {
			t.Errorf("Unexpected digest: %s", mNohead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Missing", func(t *testing.T) {
		missingRef, err := ref.New("missing." + tsURL.Host + repoPath + ":" + missingTag)
		if err != nil {
			t.Fatalf("Failed creating missingRef: %v", err)
		}
		mMissing, err := reg.ManifestGet(ctx, missingRef)
		if err == nil {
			t.Fatalf("Success running ManifestGet on missing ref: %v", mMissing)
		}
	})
	t.Run("Get Digest", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest256.String())
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mGet, err := regCache.ManifestGet(ctx, getRef)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if manifest.GetMediaType(mGet) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest256 {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head Digest", func(t *testing.T) {
		headRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest256.String())
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mHead, err := regCache.ManifestHead(ctx, headRef)
		if err != nil {
			t.Fatalf("Failed running ManifestHead: %v", err)
		}
		if manifest.GetMediaType(mHead) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mHead))
		}
		if mHead.GetDescriptor().Digest != mDigest256 {
			t.Errorf("Unexpected digest: %s", mHead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Get Digest 512", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest512.String())
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mGet, err := regCache.ManifestGet(ctx, getRef)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if manifest.GetMediaType(mGet) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest512 {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head Digest 512", func(t *testing.T) {
		headRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest512.String())
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mHead, err := regCache.ManifestHead(ctx, headRef)
		if err != nil {
			t.Fatalf("Failed running ManifestHead: %v", err)
		}
		if manifest.GetMediaType(mHead) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mHead))
		}
		if mHead.GetDescriptor().Digest != mDigest512 {
			t.Errorf("Unexpected digest: %s", mHead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Cache Get", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest256.String())
		if err != nil {
			t.Fatalf("Failed creating getRef: %v", err)
		}
		mGet, err := regCache.ManifestGet(ctx, getRef)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if manifest.GetMediaType(mGet) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest256 {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
		_, err = reg.ManifestGet(ctx, getRef)
		if err != nil {
			t.Fatalf("Failed re-running ManifestGet (cache): %v", err)
		}
		_, err = reg.ManifestHead(ctx, getRef)
		if err != nil {
			t.Fatalf("Failed running ManifestHead (cache): %v", err)
		}
	})
	// TODO: get manifest that is larger than Content-Length header
	t.Run("Size Limit", func(t *testing.T) {
		bigRef, err := ref.New(tsURL.Host + repoPath + ":" + bigTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		_, err = reg.ManifestGet(ctx, bigRef)
		if err == nil {
			t.Fatalf("ManifestGet did not fail")
		}
		if !errors.Is(err, errs.ErrSizeLimitExceeded) {
			t.Fatalf("unexpected error, expected %v, received %v", errs.ErrSizeLimitExceeded, err)
		}
	})
	t.Run("Read beyond size", func(t *testing.T) {
		shortRef, err := ref.New(tsURL.Host + repoPath + ":" + shortReadTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		_, err = reg.ManifestGet(ctx, shortRef)
		if err == nil {
			t.Fatalf("ManifestGet did not fail")
		}
		if !errors.Is(err, errs.ErrShortRead) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("unexpected error, expected %v, received %v", errs.ErrShortRead, err)
		}
	})

	t.Run("PUT tag 256", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + ":" + putTag256)
		if err != nil {
			t.Fatalf("failed creating ref: %v", err)
		}
		mm, err := manifest.New(manifest.WithRaw(mBody))
		if err != nil {
			t.Fatalf("failed to create manifest: %v", err)
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err != nil {
			t.Errorf("failed to put manifest: %v", err)
		}
	})
	t.Run("PUT tag 512", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + ":" + putTag512)
		if err != nil {
			t.Fatalf("failed creating ref: %v", err)
		}
		mm, err := manifest.New(manifest.WithRaw(mBody), manifest.WithDesc(descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(len(mBody)),
			Digest:    mDigest512,
		}))
		if err != nil {
			t.Fatalf("failed to create manifest: %v", err)
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err != nil {
			t.Errorf("failed to put manifest: %v", err)
		}
	})
	t.Run("PUT digest 256", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest256.String())
		if err != nil {
			t.Fatalf("failed creating ref: %v", err)
		}
		mm, err := manifest.New(manifest.WithRaw(mBody))
		if err != nil {
			t.Fatalf("failed to create manifest: %v", err)
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err != nil {
			t.Errorf("failed to put manifest: %v", err)
		}
	})
	t.Run("PUT tag 512", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest512.String())
		if err != nil {
			t.Fatalf("failed creating ref: %v", err)
		}
		mm, err := manifest.New(manifest.WithRaw(mBody), manifest.WithDesc(descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(len(mBody)),
			Digest:    mDigest512,
		}))
		if err != nil {
			t.Fatalf("failed to create manifest: %v", err)
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err != nil {
			t.Errorf("failed to put manifest: %v", err)
		}
	})

	t.Run("PUT size limit", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + ":" + putTag256)
		if err != nil {
			t.Fatalf("failed creating ref: %v", err)
		}
		mLarge := make([]byte, mLen+defaultManifestMaxPush)
		copy(mLarge, mBody)
		for i := mLen; i < len(mLarge); i++ {
			mLarge[i] = ' '
		}
		mm, err := manifest.New(manifest.WithRaw(mLarge))
		if err != nil {
			t.Fatalf("failed to create manifest: %v", err)
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err == nil {
			t.Fatalf("put manifest did not fail")
		}
		if !errors.Is(err, errs.ErrSizeLimitExceeded) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrSizeLimitExceeded, err)
		}
	})
}
