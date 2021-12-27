package regclient

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

	"github.com/docker/distribution"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/regclient/types"
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
			MediaType: MediaTypeDocker2ImageConfig,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []distribution.Descriptor{
			{
				MediaType: MediaTypeDocker2Layer,
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
					"Content-Type":          []string{MediaTypeDocker2Manifest},
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
					"Content-Type":          []string{MediaTypeDocker2Manifest},
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
					"Content-Type":          []string{MediaTypeDocker2Manifest},
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
	// setup the regclient
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []ConfigHost{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      TLSDisabled,
		},
		{
			Name:     "missing." + tsHost,
			Hostname: tsHost,
			TLS:      TLSDisabled,
		},
		{
			Name:     "nohead." + tsHost,
			Hostname: tsHost,
			TLS:      TLSDisabled,
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
	rc := NewRegClient(WithConfigHosts(rcHosts), WithLog(log))
	t.Run("Get", func(t *testing.T) {
		getRef, err := types.NewRef(tsURL.Host + repoPath + ":" + getTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mGet, err := rc.ManifestGet(ctx, getRef)
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
		if mGet.GetMediaType() != MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", mGet.GetMediaType())
		}
		if mGet.GetDigest() != mDigest {
			t.Errorf("Unexpected digest: %s", mGet.GetDigest().String())
		}
	})
	t.Run("Head", func(t *testing.T) {
		headRef, err := types.NewRef(tsURL.Host + repoPath + ":" + headTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mHead, err := rc.ManifestHead(ctx, headRef)
		if err != nil {
			t.Errorf("Failed running ManifestHead: %v", err)
			return
		}
		if mHead.GetMediaType() != MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", mHead.GetMediaType())
		}
		if mHead.GetDigest() != mDigest {
			t.Errorf("Unexpected digest: %s", mHead.GetDigest().String())
		}
	})
	t.Run("Head No Head", func(t *testing.T) {
		noheadRef, err := types.NewRef("nohead." + tsURL.Host + repoPath + ":" + noheadTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mNohead, err := rc.ManifestHead(ctx, noheadRef)
		if err == nil {
			t.Errorf("Unexpected successful head on \"no head\" registry: %v", mNohead)
		} else if !errors.Is(err, ErrUnsupportedAPI) {
			t.Errorf("Expected error, expected %v, received %v", ErrUnsupportedAPI, err)
		}
	})
	t.Run("Get No Head", func(t *testing.T) {
		noheadRef, err := types.NewRef("nohead." + tsURL.Host + repoPath + ":" + noheadTag)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		mNohead, err := rc.ManifestGet(ctx, noheadRef)
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
		if mNohead.GetMediaType() != MediaTypeDocker2Manifest {
			t.Errorf("Unexpected media type: %s", mNohead.GetMediaType())
		}
		if mNohead.GetDigest() != mDigest {
			t.Errorf("Unexpected digest: %s", mNohead.GetDigest().String())
		}
	})
	t.Run("Missing", func(t *testing.T) {
		missingRef, err := types.NewRef("missing." + tsURL.Host + repoPath + ":" + missingTag)
		if err != nil {
			t.Errorf("Failed creating missingRef: %v", err)
		}
		mMissing, err := rc.ManifestGet(ctx, missingRef)
		if err == nil {
			t.Errorf("Success running ManifestGet on missing ref: %v", mMissing)
			return
		}
	})
}
