package reg

import (
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

	"github.com/docker/distribution"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func TestManifest(t *testing.T) {
	repoPath := "/proj"
	getTag := "get"
	headTag := "head"
	noheadTag := "nohead"
	missingTag := "missing"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	m := dockerSchema2.Manifest{
		Config: distribution.Descriptor{
			MediaType: types.MediaTypeDocker2ImageConfig,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []distribution.Descriptor{
			{
				MediaType: types.MediaTypeDocker2Layer,
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
		if mGet.GetMediaType() != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", mGet.GetMediaType())
		}
		if mGet.GetDigest() != mDigest {
			t.Errorf("Unexpected digest: %s", mGet.GetDigest().String())
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
		if mHead.GetMediaType() != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", mHead.GetMediaType())
		}
		if mHead.GetDigest() != mDigest {
			t.Errorf("Unexpected digest: %s", mHead.GetDigest().String())
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
		if mNohead.GetMediaType() != types.MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", mNohead.GetMediaType())
		}
		if mNohead.GetDigest() != mDigest {
			t.Errorf("Unexpected digest: %s", mNohead.GetDigest().String())
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
}
