package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func TestManifest(t *testing.T) {
	repoPath := "/proj"
	getTag := "get"
	headTag := "head"
	noheadTag := "nohead"
	nodigestTag := "nodigest"
	missingTag := "missing"
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
	missingDigest := digest.FromString("missing descriptor")
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
				Name:   "Digest",
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
				Name:   "Get nodigest",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + nodigestTag,
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
				Name:   "Head nodigest",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + nodigestTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", mLen)},
					"Content-Type":   []string{types.MediaTypeDocker2Manifest},
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
				Name:   "Missing",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + missingDigest.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
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
	rc := New(
		WithConfigHost(rcHosts...),
		WithLog(log),
		WithRetryDelay(delayInit, delayMax),
	)
	t.Run("Get", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + ":" + getTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mGet, err := rc.ManifestGet(ctx, getRef)
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
		mHead, err := rc.ManifestHead(ctx, headRef)
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
	t.Run("Head no digest", func(t *testing.T) {
		headRef, err := ref.New(tsURL.Host + repoPath + ":" + nodigestTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mHead, err := rc.ManifestHead(ctx, headRef, WithManifestRequireDigest())
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
		mNohead, err := rc.ManifestHead(ctx, noheadRef)
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
		mNohead, err := rc.ManifestGet(ctx, noheadRef)
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
		mMissing, err := rc.ManifestGet(ctx, missingRef)
		if err == nil {
			t.Errorf("Success running ManifestGet on missing ref: %v", mMissing)
			return
		}
	})
	t.Run("Data", func(t *testing.T) {
		dataRef, err := ref.New(tsURL.Host + repoPath + ":data")
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		d := types.Descriptor{
			MediaType: types.MediaTypeDocker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
			Data:      mBody,
		}
		mGet, err := rc.ManifestGet(ctx, dataRef, WithManifestDesc(d))
		if err != nil {
			t.Errorf("failed running ManifestGet: %v", err)
		}
		mBodyOut, err := mGet.RawBody()
		if err != nil {
			t.Errorf("failed running RawBody: %v", err)
		}
		if !bytes.Equal(mBody, mBodyOut) {
			t.Errorf("manifest body mismatch: expected %s, received %s", string(mBody), string(mBodyOut))
		}
	})
	t.Run("Data fallback", func(t *testing.T) {
		getRef, err := ref.New(tsURL.Host + repoPath + ":" + getTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		d := types.Descriptor{
			MediaType: types.MediaTypeDocker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
			Data:      []byte("invalid data"),
		}
		_, err = rc.ManifestGet(ctx, getRef, WithManifestDesc(d))
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
	})
	t.Run("Bad Data and Found Digest", func(t *testing.T) {
		missingRef, err := ref.New("missing." + tsURL.Host + repoPath + ":" + missingTag)
		if err != nil {
			t.Errorf("Failed creating missingRef: %v", err)
		}
		d := types.Descriptor{
			MediaType: types.MediaTypeDocker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
			Data:      []byte("invalid data"),
		}
		_, err = rc.ManifestGet(ctx, missingRef, WithManifestDesc(d))
		if err != nil {
			t.Errorf("get with descriptor failed, didn't fall back to digest")
			return
		}
	})
	t.Run("Bad Data and Missing Digest", func(t *testing.T) {
		missingRef, err := ref.New("missing." + tsURL.Host + repoPath + ":" + missingTag)
		if err != nil {
			t.Errorf("Failed creating missingRef: %v", err)
		}
		d := types.Descriptor{
			MediaType: types.MediaTypeDocker2Manifest,
			Size:      int64(mLen),
			Digest:    missingDigest,
			Data:      []byte("invalid data"),
		}
		_, err = rc.ManifestGet(ctx, missingRef, WithManifestDesc(d))
		if err == nil {
			t.Errorf("Success running ManifestGet on missing ref")
			return
		}
	})

}
