package reg

import (
	"context"
	"encoding/json"
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
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	repoPath := "/proj"
	getTag := "get"
	bigTag := "big"
	shortReadTag := "short"
	headTag := "head"
	noheadTag := "nohead"
	missingTag := "missing"
	putTag := "put"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	m := schema2.Manifest{
		Config: types.Descriptor{
			MediaType: types.MediaTypeDocker2ImageConfig,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []types.Descriptor{
			{
				MediaType: types.MediaTypeDocker2LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
	}
	mBody, err := json.Marshal(m)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	mDigest := digest.FromBytes(mBody)
	mLen := len(mBody)
	ctx := context.Background()
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + getTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
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
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
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
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
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
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
				Body: mBody,
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
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
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
				Name:   "Put",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + putTag,
				Headers: http.Header{
					"Content-Type":   []string{types.MediaTypeDocker2Manifest},
					"Content-Length": {fmt.Sprintf("%d", mLen)},
				},
				Body: mBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Docker-Content-Digest": []string{mDigest.String()},
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
		WithRetryLimit(3),
	)
	regCache := New(
		WithConfigHosts(rcHosts),
		WithLog(log),
		WithDelay(delayInit, delayMax),
		WithCache(time.Minute*5, 500),
	)

	t.Run("Get", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + ":" + getTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mGet, err := reg.ManifestGet(ctx, getRef)
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
		if manifest.GetMediaType(mGet) != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head", func(t *testing.T) {
		headRef, err := ref.New(tsURL.Host + repoPath + ":" + headTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mHead, err := reg.ManifestHead(ctx, headRef)
		if err != nil {
			t.Errorf("Failed running ManifestHead: %v", err)
			return
		}
		if manifest.GetMediaType(mHead) != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mHead))
		}
		if mHead.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", mHead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head No Head", func(t *testing.T) {
		noheadRef, err := ref.New("nohead." + tsURL.Host + repoPath + ":" + noheadTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mNohead, err := reg.ManifestHead(ctx, noheadRef)
		if err == nil {
			t.Errorf("Unexpected successful head on \"no head\" registry: %v", mNohead)
		} else if !errors.Is(err, types.ErrUnsupportedAPI) {
			t.Errorf("Expected error, expected %v, received %v", types.ErrUnsupportedAPI, err)
		}
	})
	t.Run("Get No Head", func(t *testing.T) {
		noheadRef, err := ref.New("nohead." + tsURL.Host + repoPath + ":" + noheadTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mNohead, err := reg.ManifestGet(ctx, noheadRef)
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
		if manifest.GetMediaType(mNohead) != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mNohead))
		}
		if mNohead.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", mNohead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Missing", func(t *testing.T) {
		missingRef, err := ref.New("missing." + tsURL.Host + repoPath + ":" + missingTag)
		if err != nil {
			t.Errorf("Failed creating missingRef: %v", err)
		}
		mMissing, err := reg.ManifestGet(ctx, missingRef)
		if err == nil {
			t.Errorf("Success running ManifestGet on missing ref: %v", mMissing)
			return
		}
	})
	t.Run("Get Digest", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mGet, err := regCache.ManifestGet(ctx, getRef)
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
		if manifest.GetMediaType(mGet) != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head Digest", func(t *testing.T) {
		headRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mHead, err := regCache.ManifestHead(ctx, headRef)
		if err != nil {
			t.Errorf("Failed running ManifestHead: %v", err)
			return
		}
		if manifest.GetMediaType(mHead) != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mHead))
		}
		if mHead.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", mHead.GetDescriptor().Digest.String())
		}
	})
	t.Run("Cache Get", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mGet, err := regCache.ManifestGet(ctx, getRef)
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
		if manifest.GetMediaType(mGet) != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(mGet))
		}
		if mGet.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", mGet.GetDescriptor().Digest.String())
		}
		_, err = reg.ManifestGet(ctx, getRef)
		if err != nil {
			t.Errorf("Failed re-running ManifestGet (cache): %v", err)
			return
		}
		_, err = reg.ManifestHead(ctx, getRef)
		if err != nil {
			t.Errorf("Failed running ManifestHead (cache): %v", err)
			return
		}
	})
	// TODO: get manifest that is larger than Content-Length header
	t.Run("Size Limit", func(t *testing.T) {
		bigRef, err := ref.New(tsURL.Host + repoPath + ":" + bigTag)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		_, err = reg.ManifestGet(ctx, bigRef)
		if err == nil {
			t.Errorf("ManifestGet did not fail")
			return
		}
		if !errors.Is(err, types.ErrSizeLimitExceeded) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrSizeLimitExceeded, err)
			return
		}
	})
	t.Run("Read beyond size", func(t *testing.T) {
		shortRef, err := ref.New(tsURL.Host + repoPath + ":" + shortReadTag)
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		_, err = reg.ManifestGet(ctx, shortRef)
		if err == nil {
			t.Errorf("ManifestGet did not fail")
			return
		}
		if !errors.Is(err, types.ErrShortRead) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrShortRead, err)
			return
		}
	})

	t.Run("PUT", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + ":" + putTag)
		if err != nil {
			t.Errorf("failed creating ref: %v", err)
		}
		mm, err := manifest.New(manifest.WithRaw(mBody))
		if err != nil {
			t.Errorf("failed to create manifest: %v", err)
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err != nil {
			t.Errorf("failed to put manifest: %v", err)
		}
	})
	t.Run("PUT size limit", func(t *testing.T) {
		putRef, err := ref.New(tsURL.Host + repoPath + ":" + putTag)
		if err != nil {
			t.Errorf("failed creating ref: %v", err)
			return
		}
		mLarge := make([]byte, mLen+defaultManifestMaxPush)
		copy(mLarge, mBody)
		for i := mLen; i < len(mLarge); i++ {
			mLarge[i] = ' '
		}
		mm, err := manifest.New(manifest.WithRaw(mLarge))
		if err != nil {
			t.Errorf("failed to create manifest: %v", err)
			return
		}
		err = reg.ManifestPut(ctx, putRef, mm)
		if err == nil {
			t.Errorf("put manifest did not fail")
			return
		}
		if !errors.Is(err, types.ErrSizeLimitExceeded) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrSizeLimitExceeded, err)
		}
	})
}
