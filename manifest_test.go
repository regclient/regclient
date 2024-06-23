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

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	repoPath := "testrepo"
	goodTag := "v1"
	deleteTag := "v3"
	noheadTag := "nohead"
	nodigestTag := "nodigest"
	missingTag := "missing"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	m := schema2.Manifest{
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
	mDigest := digest.FromBytes(mBody)
	mLen := len(mBody)
	missingDigest := digest.FromString("missing descriptor")
	ctx := context.Background()
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get",
				Method: "GET",
				Path:   "/v2/" + repoPath + "/manifests/" + goodTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Digest",
				Method: "GET",
				Path:   "/v2/" + repoPath + "/manifests/" + mDigest.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head",
				Method: "HEAD",
				Path:   "/v2/" + repoPath + "/manifests/" + goodTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get nodigest",
				Method: "GET",
				Path:   "/v2/" + repoPath + "/manifests/" + nodigestTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
				Body: mBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head nodigest",
				Method: "HEAD",
				Path:   "/v2/" + repoPath + "/manifests/" + nodigestTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", mLen)},
					"Content-Type":   []string{mediatype.Docker2Manifest},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get nohead",
				Method: "GET",
				Path:   "/v2/" + repoPath + "/manifests/" + noheadTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mLen)},
					"Content-Type":          []string{mediatype.Docker2Manifest},
					"Docker-Content-Digest": []string{mDigest.String()},
				},
				Body: mBody,
			},
		},
	}
	rrs = append(rrs, reqresp.BaseEntries...)
	// create servers
	boolT := true
	tsInternal := httptest.NewServer(reqresp.NewHandler(t, rrs))
	olaregHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
		API: oConfig.ConfigAPI{
			DeleteEnabled: &boolT,
		},
	})
	tsOlareg := httptest.NewServer(olaregHandler)
	tsOlaregURL, _ := url.Parse(tsOlareg.URL)
	tsOlaregHost := tsOlaregURL.Host
	t.Cleanup(func() {
		tsInternal.Close()
		tsOlareg.Close()
		_ = olaregHandler.Close()
	})
	// setup the regclient
	tsInternalURL, _ := url.Parse(tsInternal.URL)
	tsInternalHost := tsInternalURL.Host
	rcHosts := []config.Host{
		{
			Name:      tsOlaregHost,
			Hostname:  tsOlaregHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: 100,
		},
		{
			Name:      "missing." + tsOlaregHost,
			Hostname:  tsOlaregHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: 100,
		},
		{
			Name:      tsInternalHost,
			Hostname:  tsInternalHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: 100,
		},
		{
			Name:     "nohead." + tsInternalHost,
			Hostname: tsInternalHost,
			TLS:      config.TLSDisabled,
			APIOpts: map[string]string{
				"disableHead": "true",
			},
			ReqPerSec: 100,
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
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestGet(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if !m.IsSet() {
			t.Errorf("manifest is not set on a get request")
		}
	})
	t.Run("Head", func(t *testing.T) {
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestHead(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ManifestHead: %v", err)
		}
		if m.IsSet() {
			t.Errorf("manifest is set on a head request")
		}
	})
	t.Run("Head no digest", func(t *testing.T) {
		r, err := ref.New(tsInternalHost + "/" + repoPath + ":" + nodigestTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestHead(ctx, r, WithManifestRequireDigest())
		if err != nil {
			t.Fatalf("Failed running ManifestHead: %v", err)
		}
		if manifest.GetMediaType(m) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(m))
		}
		if m.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", m.GetDescriptor().Digest.String())
		}
	})
	t.Run("Head No Head", func(t *testing.T) {
		r, err := ref.New("nohead." + tsInternalHost + "/" + repoPath + ":" + noheadTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestHead(ctx, r)
		if err == nil {
			t.Errorf("Unexpected successful head on \"no head\" registry: %v", m)
		} else if !errors.Is(err, errs.ErrUnsupportedAPI) {
			t.Errorf("Expected error, expected %v, received %v", errs.ErrUnsupportedAPI, err)
		}
	})
	t.Run("Get No Head", func(t *testing.T) {
		r, err := ref.New("nohead." + tsInternalHost + "/" + repoPath + ":" + noheadTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestGet(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if manifest.GetMediaType(m) != mediatype.Docker2Manifest {
			t.Errorf("Unexpected media type: %s", manifest.GetMediaType(m))
		}
		if m.GetDescriptor().Digest != mDigest {
			t.Errorf("Unexpected digest: %s", m.GetDescriptor().Digest.String())
		}
	})
	t.Run("Missing", func(t *testing.T) {
		r, err := ref.New("missing." + tsOlaregHost + "/" + repoPath + ":" + missingTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestGet(ctx, r)
		if err == nil {
			t.Errorf("Success running ManifestGet on missing ref: %v", m)
			return
		}
	})
	t.Run("Get Platform", func(t *testing.T) {
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		p, err := platform.Parse("linux/amd64")
		if err != nil {
			t.Fatalf("Failed parsing platform: %v", err)
		}
		m, err := rc.ManifestGet(ctx, r, WithManifestPlatform(p))
		if err != nil {
			t.Fatalf("Failed running ManifestGet: %v", err)
		}
		if !m.IsSet() {
			t.Errorf("manifest is not set on a get request")
		}
		if m.IsList() {
			t.Errorf("returned manifest is an index")
		}
	})
	t.Run("Get Missing Platform", func(t *testing.T) {
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		p, err := platform.Parse("linux/ppc64le")
		if err != nil {
			t.Fatalf("Failed parsing platform: %v", err)
		}
		_, err = rc.ManifestGet(ctx, r, WithManifestPlatform(p))
		if err == nil {
			t.Fatalf("Success running ManifestGet on missing platform")
		}
		if !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("Expected error %v, received %v", errs.ErrNotFound, err)
		}
	})
	t.Run("Head Platform", func(t *testing.T) {
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		p, err := platform.Parse("linux/amd64")
		if err != nil {
			t.Fatalf("Failed parsing platform: %v", err)
		}
		m, err := rc.ManifestHead(ctx, r, WithManifestPlatform(p))
		if err != nil {
			t.Fatalf("Failed running ManifestHead: %v", err)
		}
		if m.IsSet() {
			t.Errorf("manifest is set on a head request")
		}
		if m.IsList() {
			t.Errorf("returned manifest is an index")
		}
	})
	t.Run("Head Missing Platform", func(t *testing.T) {
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		p, err := platform.Parse("linux/ppc64le")
		if err != nil {
			t.Fatalf("Failed parsing platform: %v", err)
		}
		_, err = rc.ManifestHead(ctx, r, WithManifestPlatform(p))
		if err == nil {
			t.Fatalf("Success running ManifestHead on missing platform")
		}
		if !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("Expected error %v, received %v", errs.ErrNotFound, err)
		}
	})
	t.Run("Data", func(t *testing.T) {
		r, err := ref.New(tsInternalHost + "/" + repoPath + ":data")
		if err != nil {
			t.Errorf("Failed creating ref: %v", err)
		}
		d := descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
			Data:      mBody,
		}
		m, err := rc.ManifestGet(ctx, r, WithManifestDesc(d))
		if err != nil {
			t.Fatalf("failed running ManifestGet: %v", err)
		}
		mBodyOut, err := m.RawBody()
		if err != nil {
			t.Fatalf("failed running RawBody: %v", err)
		}
		if !bytes.Equal(mBody, mBodyOut) {
			t.Errorf("manifest body mismatch: expected %s, received %s", string(mBody), string(mBodyOut))
		}
	})
	t.Run("Data fallback", func(t *testing.T) {
		r, err := ref.New(tsInternalHost + "/" + repoPath + ":" + goodTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		d := descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
			Data:      []byte("invalid data"),
		}
		_, err = rc.ManifestGet(ctx, r, WithManifestDesc(d))
		if err != nil {
			t.Errorf("Failed running ManifestGet: %v", err)
			return
		}
	})
	t.Run("Bad Data and Found Digest", func(t *testing.T) {
		r, err := ref.New(tsInternalHost + "/" + repoPath + ":" + missingTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		d := descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
			Data:      []byte("invalid data"),
		}
		_, err = rc.ManifestGet(ctx, r, WithManifestDesc(d))
		if err != nil {
			t.Errorf("get with descriptor failed, didn't fall back to digest")
			return
		}
	})
	t.Run("Bad Data and Missing Digest", func(t *testing.T) {
		r, err := ref.New("missing." + tsOlaregHost + "/" + repoPath + ":" + missingTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		d := descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(mLen),
			Digest:    missingDigest,
			Data:      []byte("invalid data"),
		}
		_, err = rc.ManifestGet(ctx, r, WithManifestDesc(d))
		if err == nil {
			t.Errorf("Success running ManifestGet on missing ref")
			return
		}
	})
	t.Run("Invalid ref", func(t *testing.T) {
		r, err := ref.NewHost("registry.example.org")
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		_, err = rc.ManifestGet(ctx, r)
		if !errors.Is(err, errs.ErrInvalidReference) {
			t.Errorf("ManifestGet did not respond with invalid ref: %v", err)
		}
		_, err = rc.ManifestHead(ctx, r)
		if !errors.Is(err, errs.ErrInvalidReference) {
			t.Errorf("ManifestGet did not respond with invalid ref: %v", err)
		}
	})
	t.Run("Delete", func(t *testing.T) {
		r, err := ref.New(tsOlaregHost + "/" + repoPath + ":" + deleteTag)
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		m, err := rc.ManifestGet(ctx, r)
		if err != nil {
			t.Fatalf("failed to run get request: %v", err)
		}
		r = r.SetDigest(m.GetDescriptor().Digest.String())
		err = rc.ManifestDelete(ctx, r, WithManifest(m), WithManifestCheckReferrers())
		if err != nil {
			t.Errorf("ManifestDelete failed: %v", err)
		}
		_, err = rc.ManifestHead(ctx, r)
		if !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("head after delete did not return a non-found: %v", err)
		}

	})
}
