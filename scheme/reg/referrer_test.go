package reg

import (
	"context"
	"encoding/json"
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
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/tag"
	"github.com/sirupsen/logrus"
)

func TestReferrer(t *testing.T) {
	// setup http server with and without API support
	ctx := context.Background()
	repoPath := "/proj"
	tagV1 := "v1"
	aType := "sbom"
	extraAnnot := "org.opencontainers.artifact.sbom.format"
	extraValue := "SPDX json"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	// manifest being referenced
	m := schema2.Manifest{
		Versioned: schema2.ManifestSchemaVersion,
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
	// artifact being attached
	artifactAnnot := map[string]string{
		annotType:  aType,
		extraAnnot: extraValue,
	}
	artifact := v1.Manifest{
		Versioned: v1.ManifestSchemaVersion,
		MediaType: types.MediaTypeOCI1Manifest,
		Config: types.Descriptor{
			MediaType: types.MediaTypeOCI1ImageConfig,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []types.Descriptor{
			{
				MediaType: types.MediaTypeOCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifactAnnot,
		Refers: &types.Descriptor{
			MediaType: types.MediaTypeDocker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
		},
	}
	artifactM, err := manifest.New(manifest.WithOrig(artifact))
	if err != nil {
		t.Errorf("failed creating artifact manifest: %v", err)
	}
	artifactBody, err := artifactM.RawBody()
	if err != nil {
		t.Errorf("failed extracting raw body from artifact: %v", err)
	}
	// empty response
	emptyReply := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: types.MediaTypeOCI1ManifestList,
	}
	emptyBody, err := json.Marshal(emptyReply)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	emptyDigest := digest.FromBytes(emptyBody)
	emptyLen := len(emptyBody)
	// full response
	fullReply := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: types.MediaTypeOCI1ManifestList,
		Manifests: []types.Descriptor{
			{
				MediaType:   types.MediaTypeOCI1Manifest,
				Size:        int64(len(artifactBody)),
				Digest:      artifactM.GetDescriptor().Digest,
				Annotations: artifactAnnot,
			},
		},
	}
	fullBody, err := json.Marshal(fullReply)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	fullDigest := digest.FromBytes(fullBody)
	fullLen := len(fullBody)
	// tag listing
	tagNoAPI := fmt.Sprintf("%s-%s.%s.%s", mDigest.Algorithm().String(), mDigest.Hex(), artifactM.GetDescriptor().Digest.Hex()[:16], aType)
	tagListNoAPIData := tag.DockerList{
		Name: repoPath,
		Tags: []string{
			"v1",
			tagNoAPI,
		},
	}
	tagListNoAPI, err := json.Marshal(tagListNoAPIData)
	if err != nil {
		t.Errorf("failed to marshal tag list: %v", err)
		return
	}
	t.Logf("artifactM digest: %s\n", artifactM.GetDescriptor().Digest.String())
	t.Logf("NoAPI tag: %s\n", tagNoAPI)

	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + tagV1,
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
				Name:   "Get",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + tagV1,
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
	}
	rrsNoAPI := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "API 404",
				Method: "GET",
				Path:   "/v2" + repoPath + "/_oci/artifacts/referrers",
				Query: map[string][]string{
					"digest": {mDigest.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Put with tag",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:   artifactBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get tag",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + tagNoAPI,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(artifactBody))},
					"Content-Type":          []string{types.MediaTypeOCI1Manifest},
					"Docker-Content-Digest": []string{artifactM.GetDescriptor().Digest.String()},
				},
				Body: artifactBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head tag",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + tagNoAPI,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(artifactBody))},
					"Content-Type":          []string{types.MediaTypeOCI1Manifest},
					"Docker-Content-Digest": []string{artifactM.GetDescriptor().Digest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Tag list",
				Method: "GET",
				Path:   "/v2" + repoPath + "/tags/list",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   tagListNoAPI,
			},
		},
	}
	rrsAPI := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "API empty",
				Method: "GET",
				Path:   "/v2" + repoPath + "/_oci/artifacts/referrers",
				Query: map[string][]string{
					"digest": {mDigest.String()},
				},
				DelOnUse: true,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", emptyLen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{emptyDigest.String()},
				},
				Body: emptyBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Put",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + artifactM.GetDescriptor().Digest.String(),
				Body:   artifactBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "API with entries",
				Method: "GET",
				Path:   "/v2" + repoPath + "/_oci/artifacts/referrers",
				Query: map[string][]string{
					"digest": {mDigest.String()},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", fullLen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{fullDigest.String()},
				},
				Body: fullBody,
			},
		},
	}
	rrsNoAPI = append(rrsNoAPI, rrs...)
	rrsNoAPI = append(rrsNoAPI, reqresp.BaseEntries...)
	rrsAPI = append(rrsAPI, rrs...)
	rrsAPI = append(rrsAPI, reqresp.BaseEntries...)
	tsNoAPI := httptest.NewServer(reqresp.NewHandler(t, rrsNoAPI))
	defer tsNoAPI.Close()
	tsAPI := httptest.NewServer(reqresp.NewHandler(t, rrsAPI))
	defer tsAPI.Close()

	// setup regclient for http server
	tsURLNoAPI, _ := url.Parse(tsNoAPI.URL)
	tsHostNoAPI := tsURLNoAPI.Host
	tsURLAPI, _ := url.Parse(tsAPI.URL)
	tsHostAPI := tsURLAPI.Host
	rcHosts := []*config.Host{
		{
			Name:     tsHostNoAPI,
			Hostname: tsHostNoAPI,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsHostAPI,
			Hostname: tsHostAPI,
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

	// attach to v1 image
	t.Run("Put NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ReferrerPut(ctx, r, artifactM)
		if err != nil {
			t.Errorf("Failed running ReferrerPut: %v", err)
			return
		}
	})
	t.Run("Put API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ReferrerPut(ctx, r, artifactM)
		if err != nil {
			t.Errorf("Failed running ReferrerPut: %v", err)
			return
		}
	})

	// list referrers to v1
	t.Run("List NoAPI - headers only", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) <= 0 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
	})
	t.Run("List NoAPI - get annotations", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r, scheme.WithReferrerForceGet())
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) <= 0 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
	})
	t.Run("List API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) <= 0 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
	})

}

func mapStringStringEq(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}
