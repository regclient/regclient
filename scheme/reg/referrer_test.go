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
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func TestReferrer(t *testing.T) {
	// setup http server with and without API support
	ctx := context.Background()
	repoPath := "/proj"
	tagV1 := "v1"
	tagV1List := "v1-list"
	extraAnnot := "org.opencontainers.artifact.sbom.format"
	extraValue := "json"
	extraValue2 := "x509"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	configMTA := "application/vnd.example.sbom"
	configMTB := "application/vnd.example.sig"
	platStr := "linux/amd64"
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
	// manifest list
	mList := schema2.ManifestList{
		Versioned: schema2.ManifestListSchemaVersion,
		Manifests: []types.Descriptor{
			{
				MediaType: types.MediaTypeDocker2Manifest,
				Digest:    mDigest,
				Size:      int64(mLen),
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
			{
				MediaType: types.MediaTypeDocker2Manifest,
				Digest:    digest.FromString("missing"),
				Size:      int64(1234),
				Platform: &platform.Platform{
					OS:           "linux",
					Architecture: "arm64",
				},
			},
		},
	}
	mlBody, err := json.Marshal(mList)
	if err != nil {
		t.Errorf("Failed to marshal manifest list: %v", err)
	}
	mlDigest := digest.FromBytes(mlBody)
	mlLen := len(mlBody)
	// artifact being attached
	artifactAnnot := map[string]string{
		extraAnnot: extraValue,
	}
	artifact := v1.Manifest{
		Versioned: v1.ManifestSchemaVersion,
		MediaType: types.MediaTypeOCI1Manifest,
		Config: types.Descriptor{
			MediaType: configMTA,
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
		Subject: &types.Descriptor{
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
	artifactDigest := digest.FromBytes(artifactBody)
	artifact2Annot := map[string]string{
		extraAnnot: extraValue2,
	}
	artifact2 := v1.ArtifactManifest{
		MediaType:    types.MediaTypeOCI1Artifact,
		ArtifactType: configMTB,
		Blobs: []types.Descriptor{
			{
				MediaType: types.MediaTypeOCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifact2Annot,
		Subject: &types.Descriptor{
			MediaType: types.MediaTypeDocker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
		},
	}
	artifact2M, err := manifest.New(manifest.WithOrig(artifact2))
	if err != nil {
		t.Errorf("failed creating artifact manifest: %v", err)
	}
	artifact2Body, err := artifact2M.RawBody()
	if err != nil {
		t.Errorf("failed extracting raw body from artifact: %v", err)
	}
	artifact2Digest := digest.FromBytes(artifact2Body)
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
	// a response
	replyA := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: types.MediaTypeOCI1ManifestList,
		Manifests: []types.Descriptor{
			{
				MediaType:    types.MediaTypeOCI1Manifest,
				ArtifactType: configMTA,
				Size:         int64(len(artifactBody)),
				Digest:       artifactM.GetDescriptor().Digest,
				Annotations:  artifactAnnot,
			},
		},
	}
	replyABody, err := json.Marshal(replyA)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	replyADig := digest.FromBytes(replyABody)
	replyALen := len(replyABody)
	// a response
	replyB := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: types.MediaTypeOCI1ManifestList,
		Manifests: []types.Descriptor{
			{
				MediaType:    types.MediaTypeOCI1Artifact,
				ArtifactType: configMTB,
				Size:         int64(len(artifact2Body)),
				Digest:       artifact2M.GetDescriptor().Digest,
				Annotations:  artifact2Annot,
			},
		},
	}
	replyBBody, err := json.Marshal(replyB)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	replyBDig := digest.FromBytes(replyBBody)
	replyBLen := len(replyBBody)
	// full response
	replyBoth := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: types.MediaTypeOCI1ManifestList,
		Manifests: []types.Descriptor{
			{
				MediaType:    types.MediaTypeOCI1Manifest,
				ArtifactType: configMTA,
				Size:         int64(len(artifactBody)),
				Digest:       artifactM.GetDescriptor().Digest,
				Annotations:  artifactAnnot,
			},
			{
				MediaType:    types.MediaTypeOCI1Artifact,
				ArtifactType: configMTB,
				Size:         int64(len(artifact2Body)),
				Digest:       artifact2M.GetDescriptor().Digest,
				Annotations:  artifact2Annot,
			},
		},
	}
	replyBothBody, err := json.Marshal(replyBoth)
	if err != nil {
		t.Errorf("Failed to marshal manifest: %v", err)
	}
	replyBothDig := digest.FromBytes(replyBothBody)
	replyBothLen := len(replyBothBody)
	tagNoAPI := fmt.Sprintf("%s-%s", mDigest.Algorithm().String(), mDigest.Hex())
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
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + tagV1List,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mlLen)},
					"Content-Type":          []string{types.MediaTypeDocker2ManifestList},
					"Docker-Content-Digest": []string{mlDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + tagV1List,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", mlLen)},
					"Content-Type":          []string{types.MediaTypeDocker2ManifestList},
					"Docker-Content-Digest": []string{mlDigest.String()},
				},
				Body: mlBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put A by digest",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + string(artifactDigest),
				Body:     artifactBody,
				SetState: "putA",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put B by digest",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + string(artifact2Digest),
				Body:     artifact2Body,
				IfState:  []string{"putA", "putARef"},
				SetState: "putBoth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get A",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + string(artifactDigest),
				IfState: []string{"putA", "deleteB", "deleteBRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(artifactBody))},
					"Content-Type":          []string{types.MediaTypeOCI1Manifest},
					"Docker-Content-Digest": []string{string(artifactDigest)},
				},
				Body: artifactBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get B",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + string(artifact2Digest),
				IfState: []string{"putBoth", "putBothRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(artifact2Body))},
					"Content-Type":          []string{types.MediaTypeOCI1Artifact},
					"Docker-Content-Digest": []string{string(artifact2Digest)},
				},
				Body: artifact2Body,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Delete B",
				Method:   "DELETE",
				Path:     "/v2" + repoPath + "/manifests/" + string(artifact2Digest),
				IfState:  []string{"deleteBRef", "putBoth"},
				SetState: "deleteB",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Delete A",
				Method:   "DELETE",
				Path:     "/v2" + repoPath + "/manifests/" + string(artifactDigest),
				IfState:  []string{"deleteBothRef", "deleteB"},
				SetState: "deleteBoth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
	}
	rrsNoAPI := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "API 404",
				Method: "GET",
				Path:   "/v2" + repoPath + "/referrers/" + mDigest.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get tag 404",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + tagNoAPI,
				IfState: []string{"", "putA", "deleteBothRef", "deleteBoth"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get tag A",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + tagNoAPI,
				IfState: []string{"putARef", "putBoth", "deleteB", "deleteBRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyALen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyADig.String()},
				},
				Body: replyABody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get tag Both",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + tagNoAPI,
				IfState: []string{"putBothRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyBothLen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyBothDig.String()},
				},
				Body: replyBothBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put A Ref",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:     replyABody,
				SetState: "putARef",
				IfState:  []string{"putA"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put Both Ref",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:     replyBothBody,
				SetState: "putBothRef",
				IfState:  []string{"putBoth"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put A Ref",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:     replyABody,
				SetState: "deleteBRef",
				IfState:  []string{"putBothRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Delete Ref",
				Method:   "DELETE",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				SetState: "deleteBothRef",
				IfState:  []string{"deleteB"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
	}
	rrsNoAPIAuth := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "API 401",
				Method: "GET",
				Path:   "/v2" + repoPath + "/referrers/" + mDigest.String(),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get tag 404",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + tagNoAPI,
				IfState: []string{"", "putA", "deleteBothRef", "deleteBoth"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get tag A",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + tagNoAPI,
				IfState: []string{"putARef", "putBoth", "deleteB", "deleteBRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyALen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyADig.String()},
				},
				Body: replyABody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "Get tag Both",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + tagNoAPI,
				IfState: []string{"putBothRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyBothLen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyBothDig.String()},
				},
				Body: replyBothBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put A Ref",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:     replyABody,
				SetState: "putARef",
				IfState:  []string{"putA"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put Both Ref",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:     replyBothBody,
				SetState: "putBothRef",
				IfState:  []string{"putBoth"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Put A Ref",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				Body:     replyABody,
				SetState: "deleteBRef",
				IfState:  []string{"putBothRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "Delete Ref",
				Method:   "DELETE",
				Path:     "/v2" + repoPath + "/manifests/" + tagNoAPI,
				SetState: "deleteBothRef",
				IfState:  []string{"deleteB"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
	}
	rrsAPI := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "API empty",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/referrers/" + mDigest.String(),
				IfState: []string{"", "deleteBoth"},
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
				Name:    "API with A",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/referrers/" + mDigest.String(),
				IfState: []string{"putA", "deleteB"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyALen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyADig.String()},
				},
				Body: replyABody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "API with Both Part 2",
				Method: "GET",
				Path:   "/v2" + repoPath + "/referrers/" + mDigest.String(),
				Query: map[string][]string{
					"next": {"1"},
				},
				IfState: []string{"putBoth"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyBLen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyBDig.String()},
				},
				Body: replyBBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:    "API with Both Part 1",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/referrers/" + mDigest.String(),
				IfState: []string{"putBoth"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", replyALen)},
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Docker-Content-Digest": []string{replyADig.String()},
					"Link":                  []string{fmt.Sprintf(`</v2%s/referrers/%s?next=1>; rel="next"`, repoPath, mDigest.String())},
				},
				Body: replyABody,
			},
		},
	}
	rrsNoAPI = append(rrsNoAPI, rrs...)
	rrsNoAPI = append(rrsNoAPI, reqresp.BaseEntries...)
	rrsNoAPIAuth = append(rrsNoAPIAuth, rrs...)
	rrsNoAPIAuth = append(rrsNoAPIAuth, reqresp.BaseEntries...)
	rrsAPI = append(rrsAPI, rrs...)
	rrsAPI = append(rrsAPI, reqresp.BaseEntries...)
	tsNoAPI := httptest.NewServer(reqresp.NewHandler(t, rrsNoAPI))
	defer tsNoAPI.Close()
	tsNoAPIAuth := httptest.NewServer(reqresp.NewHandler(t, rrsNoAPIAuth))
	defer tsNoAPIAuth.Close()
	tsAPI := httptest.NewServer(reqresp.NewHandler(t, rrsAPI))
	defer tsAPI.Close()

	// setup regclient for http server
	tsURLNoAPI, _ := url.Parse(tsNoAPI.URL)
	tsHostNoAPI := tsURLNoAPI.Host
	tsURLNoAPIAuth, _ := url.Parse(tsNoAPIAuth.URL)
	tsHostNoAPIAuth := tsURLNoAPIAuth.Host
	tsURLAPI, _ := url.Parse(tsAPI.URL)
	tsHostAPI := tsURLAPI.Host
	rcHosts := []*config.Host{
		{
			Name:     tsHostNoAPI,
			Hostname: tsHostNoAPI,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsHostNoAPIAuth,
			Hostname: tsHostNoAPIAuth,
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

	// list empty
	t.Run("List empty NoAPI", func(t *testing.T) {
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
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
		}
	})
	t.Run("List empty NoAPIAuth", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPIAuth.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
		}
	})
	t.Run("List empty API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
		}
	})

	// attach A to v1 image
	t.Run("Put A NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifactM)
		if err != nil {
			t.Errorf("Failed running ManifestPut: %v", err)
			return
		}
	})
	t.Run("Put A NoAPIAuth", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPIAuth.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifactM)
		if err != nil {
			t.Errorf("Failed running ManifestPut: %v", err)
			return
		}
	})
	t.Run("Put A API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifactM)
		if err != nil {
			t.Errorf("Failed running ManifestPut: %v", err)
			return
		}
	})

	// list referrers to v1
	t.Run("List A NoAPI", func(t *testing.T) {
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
		if len(rl.Descriptors) < 1 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if len(rl.Tags) != 1 || rl.Tags[0] != tagNoAPI {
			t.Errorf("tag list missing entries, received: %v", rl.Tags)
		}
	})
	t.Run("List A NoAPIAuth", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPIAuth.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) < 1 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if len(rl.Tags) != 1 || rl.Tags[0] != tagNoAPI {
			t.Errorf("tag list missing entries, received: %v", rl.Tags)
		}
	})
	t.Run("List A API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) < 1 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if len(rl.Tags) != 0 {
			t.Errorf("tag list unexpected entries, received: %v", rl.Tags)
		}
	})

	// attach B to v1 image
	t.Run("Put B NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifact2M)
		if err != nil {
			t.Errorf("Failed running ManifestPut: %v", err)
			return
		}
	})
	t.Run("Put B API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifact2M)
		if err != nil {
			t.Errorf("Failed running ManifestPut: %v", err)
			return
		}
	})

	// list referrers to v1
	t.Run("List Both NoAPI", func(t *testing.T) {
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
		if len(rl.Descriptors) != 2 {
			t.Errorf("descriptor list expected 2, received %d", len(rl.Descriptors))
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			rl.Descriptors[0].ArtifactType != configMTA ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if rl.Descriptors[1].MediaType != types.MediaTypeOCI1Artifact ||
			rl.Descriptors[1].Size != int64(len(artifact2Body)) ||
			rl.Descriptors[1].Digest != artifact2M.GetDescriptor().Digest ||
			rl.Descriptors[1].ArtifactType != configMTB ||
			!mapStringStringEq(rl.Descriptors[1].Annotations, artifact2Annot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[1])
		}
		if len(rl.Tags) != 1 || rl.Tags[0] != tagNoAPI {
			t.Errorf("tag list missing entries, received: %v", rl.Tags)
		}
	})
	t.Run("List Both API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 2 {
			t.Errorf("descriptor list expected 2, received %d", len(rl.Descriptors))
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			rl.Descriptors[0].ArtifactType != configMTA ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if rl.Descriptors[1].MediaType != types.MediaTypeOCI1Artifact ||
			rl.Descriptors[1].Size != int64(len(artifact2Body)) ||
			rl.Descriptors[1].Digest != artifact2M.GetDescriptor().Digest ||
			rl.Descriptors[1].ArtifactType != configMTB ||
			!mapStringStringEq(rl.Descriptors[1].Annotations, artifact2Annot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[1])
		}
		if len(rl.Tags) != 0 {
			t.Errorf("tag list unexpected entries, received: %v", rl.Tags)
		}
	})

	t.Run("List with artifact filter API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r, scheme.WithReferrerAT(configMTA))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 1 {
			t.Errorf("descriptor list mismatch: %v", rl.Descriptors)
			return
		}
		rl, err = reg.ReferrerList(ctx, r, scheme.WithReferrerAT("application/vnd.example.unknown"))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("unexpected descriptors: %v", rl.Descriptors)
			return
		}
	})
	t.Run("List with annotation filter", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r, scheme.WithReferrerAnnotations(map[string]string{extraAnnot: extraValue2}))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 1 {
			t.Errorf("descriptor list mismatch: %v", rl.Descriptors)
			return
		}
		rl, err = reg.ReferrerList(ctx, r, scheme.WithReferrerAnnotations(map[string]string{extraAnnot: "unknown value"}))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("unexpected descriptors: %v", rl.Descriptors)
			return
		}
		rl, err = reg.ReferrerList(ctx, r, scheme.WithReferrerAnnotations(map[string]string{extraAnnot: ""}))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 2 {
			t.Errorf("descriptor list mismatch: %v", rl.Descriptors)
			return
		}
	})

	t.Run("List for platform", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1List)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := reg.ReferrerList(ctx, r, scheme.WithReferrerPlatform(platStr))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 2 {
			t.Errorf("descriptor list expected 2, received %d", len(rl.Descriptors))
			return
		}
	})

	// delete manifest with refers
	t.Run("Delete B NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifestCheckReferrers())
		if err != nil {
			t.Errorf("Failed running ManifestDelete: %v", err)
			return
		}
	})
	t.Run("Delete B API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifestCheckReferrers())
		if err != nil {
			t.Errorf("Failed running ManifestDelete: %v", err)
			return
		}
	})

	t.Run("Delete A NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifest(artifactM))
		if err != nil {
			t.Errorf("Failed running ManifestDelete: %v", err)
			return
		}
	})
	t.Run("Delete A API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifest(artifactM))
		if err != nil {
			t.Errorf("Failed running ManifestDelete: %v", err)
			return
		}
	})

	// list empty after delete
	t.Run("List empty after delete NoAPI", func(t *testing.T) {
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
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
		}
	})
	t.Run("List empty after delete API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + ":" + tagV1)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
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
