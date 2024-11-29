package reg

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

func TestReferrer(t *testing.T) {
	t.Parallel()
	// setup http server with and without API support
	ctx := context.Background()
	repoPath := "/proj"
	extraAnnot := "org.example.sbom.format"
	extraValue := "json"
	extraValue2 := "x509"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	configMTA := "application/vnd.example.sbom"
	configMTB := "application/vnd.example.sig"
	// manifest being referenced
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
		t.Fatalf("Failed to marshal manifest: %v", err)
	}
	mDigest := digest.FromBytes(mBody)
	mLen := len(mBody)
	// artifact being attached
	artifactAnnot := map[string]string{
		extraAnnot: extraValue,
	}
	artifact := v1.Manifest{
		Versioned: v1.ManifestSchemaVersion,
		MediaType: mediatype.OCI1Manifest,
		Config: descriptor.Descriptor{
			MediaType: configMTA,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []descriptor.Descriptor{
			{
				MediaType: mediatype.OCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifactAnnot,
		Subject: &descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
		},
	}
	artifactM, err := manifest.New(manifest.WithOrig(artifact))
	if err != nil {
		t.Fatalf("failed creating artifact manifest: %v", err)
	}
	artifactBody, err := artifactM.RawBody()
	if err != nil {
		t.Fatalf("failed extracting raw body from artifact: %v", err)
	}
	artifactDigest := digest.FromBytes(artifactBody)
	artifact2Annot := map[string]string{
		extraAnnot: extraValue2,
	}
	artifact2 := v1.ArtifactManifest{
		MediaType:    mediatype.OCI1Artifact,
		ArtifactType: configMTB,
		Blobs: []descriptor.Descriptor{
			{
				MediaType: mediatype.OCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifact2Annot,
		Subject: &descriptor.Descriptor{
			MediaType: mediatype.Docker2Manifest,
			Size:      int64(mLen),
			Digest:    mDigest,
		},
	}
	artifact2M, err := manifest.New(manifest.WithOrig(artifact2))
	if err != nil {
		t.Fatalf("failed creating artifact manifest: %v", err)
	}
	artifact2Body, err := artifact2M.RawBody()
	if err != nil {
		t.Fatalf("failed extracting raw body from artifact: %v", err)
	}
	artifact2Digest := digest.FromBytes(artifact2Body)
	// empty response
	emptyReply := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
	}
	emptyBody, err := json.Marshal(emptyReply)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}
	emptyDigest := digest.FromBytes(emptyBody)
	emptyLen := len(emptyBody)
	// a response
	replyA := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
		Manifests: []descriptor.Descriptor{
			{
				MediaType:    mediatype.OCI1Manifest,
				ArtifactType: configMTA,
				Size:         int64(len(artifactBody)),
				Digest:       artifactM.GetDescriptor().Digest,
				Annotations:  artifactAnnot,
			},
		},
	}
	replyABody, err := json.Marshal(replyA)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}
	replyADig := digest.FromBytes(replyABody)
	replyALen := len(replyABody)
	// a response
	replyB := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
		Manifests: []descriptor.Descriptor{
			{
				MediaType:    mediatype.OCI1Artifact,
				ArtifactType: configMTB,
				Size:         int64(len(artifact2Body)),
				Digest:       artifact2M.GetDescriptor().Digest,
				Annotations:  artifact2Annot,
			},
		},
	}
	replyBBody, err := json.Marshal(replyB)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}
	replyBDig := digest.FromBytes(replyBBody)
	replyBLen := len(replyBBody)
	// full response
	replyBoth := v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
		Manifests: []descriptor.Descriptor{
			{
				MediaType:    mediatype.OCI1Manifest,
				ArtifactType: configMTA,
				Size:         int64(len(artifactBody)),
				Digest:       artifactM.GetDescriptor().Digest,
				Annotations:  artifactAnnot,
			},
			{
				MediaType:    mediatype.OCI1Artifact,
				ArtifactType: configMTB,
				Size:         int64(len(artifact2Body)),
				Digest:       artifact2M.GetDescriptor().Digest,
				Annotations:  artifact2Annot,
			},
		},
	}
	replyBothBody, err := json.Marshal(replyBoth)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}
	replyBothDig := digest.FromBytes(replyBothBody)
	replyBothLen := len(replyBothBody)
	tagNoAPI := fmt.Sprintf("%s-%s", mDigest.Algorithm().String(), mDigest.Hex())
	t.Logf("artifactM digest: %s\n", artifactM.GetDescriptor().Digest.String())
	t.Logf("NoAPI tag: %s\n", tagNoAPI)

	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Head manifest",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest.String(),
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
				Name:   "Get manifest",
				Method: "GET",
				Path:   "/v2" + repoPath + "/manifests/" + mDigest.String(),
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
				Name:    "Get A",
				Method:  "GET",
				Path:    "/v2" + repoPath + "/manifests/" + string(artifactDigest),
				IfState: []string{"putA", "deleteB", "deleteBRef"},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(artifactBody))},
					"Content-Type":          []string{mediatype.OCI1Manifest},
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
					"Content-Type":          []string{mediatype.OCI1Artifact},
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
	rrsNoAPIPut := []reqresp.ReqResp{
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
				Name:     "Put A by digest",
				Method:   "PUT",
				Path:     "/v2" + repoPath + "/manifests/" + string(artifactDigest),
				Body:     artifactBody,
				SetState: "putA",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					OCISubjectHeader: []string{mDigest.String()},
				},
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
				Headers: http.Header{
					OCISubjectHeader: []string{mDigest.String()},
				},
			},
		},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
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
					"Content-Type":          []string{mediatype.OCI1ManifestList},
					"Docker-Content-Digest": []string{replyADig.String()},
					"Link":                  []string{fmt.Sprintf(`</v2%s/referrers/%s?next=1>; rel="next"`, repoPath, mDigest.String())},
				},
				Body: replyABody,
			},
		},
	}
	rrsNoAPI = append(rrsNoAPI, rrs...)
	rrsNoAPI = append(rrsNoAPI, rrsNoAPIPut...)
	rrsNoAPI = append(rrsNoAPI, reqresp.BaseEntries...)
	rrsNoAPIAuth = append(rrsNoAPIAuth, rrs...)
	rrsNoAPIAuth = append(rrsNoAPIAuth, rrsNoAPIPut...)
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
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	reg := New(
		WithConfigHosts(rcHosts),
		WithSlog(log),
		WithDelay(delayInit, delayMax),
		WithCache(time.Minute*5, 500),
	)

	// list empty
	t.Run("List empty NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("descriptors exist")
		}
	})
	t.Run("List empty NoAPIAuth", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPIAuth.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("descriptors exist")
		}
	})
	t.Run("List empty API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("descriptors exist")
		}
	})

	// attach A to v1 image
	t.Run("Put A NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifactM)
		if err != nil {
			t.Fatalf("Failed running ManifestPut: %v", err)
		}
	})
	t.Run("Put A NoAPIAuth", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPIAuth.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifactM)
		if err != nil {
			t.Fatalf("Failed running ManifestPut: %v", err)
		}
	})
	t.Run("Put A API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifactM)
		if err != nil {
			t.Fatalf("Failed running ManifestPut: %v", err)
		}
	})

	// list referrers to v1
	t.Run("List A NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) < 1 {
			t.Fatalf("descriptor list missing")
		}
		if rl.Descriptors[0].MediaType != mediatype.OCI1Manifest ||
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
		r, err := ref.New(tsURLNoAPIAuth.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) < 1 {
			t.Fatalf("descriptor list missing")
		}
		if rl.Descriptors[0].MediaType != mediatype.OCI1Manifest ||
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
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) < 1 {
			t.Fatalf("descriptor list missing")
		}
		if rl.Descriptors[0].MediaType != mediatype.OCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if len(rl.Tags) != 0 {
			t.Errorf("tag list unexpected entries, received: %v", rl.Tags)
		}
	})

	// list referrers to v1 without digest
	t.Run("List A No Digest", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + ":v1")
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		_, err = reg.ReferrerList(ctx, r)
		if err == nil {
			t.Errorf("did not fail when given a tag")
		}
	})

	// attach B to v1 image
	t.Run("Put B NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifact2M)
		if err != nil {
			t.Fatalf("Failed running ManifestPut: %v", err)
		}
	})
	t.Run("Put B API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestPut(ctx, r, artifact2M)
		if err != nil {
			t.Fatalf("Failed running ManifestPut: %v", err)
		}
	})

	// list referrers to v1
	t.Run("List Both NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) != 2 {
			t.Fatalf("descriptor list expected 2, received %d", len(rl.Descriptors))
		}
		if rl.Descriptors[0].MediaType != mediatype.OCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			rl.Descriptors[0].ArtifactType != configMTA ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if rl.Descriptors[1].MediaType != mediatype.OCI1Artifact ||
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
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) != 2 {
			t.Fatalf("descriptor list expected 2, received %d", len(rl.Descriptors))
		}
		if rl.Descriptors[0].MediaType != mediatype.OCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			rl.Descriptors[0].ArtifactType != configMTA ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
		if rl.Descriptors[1].MediaType != mediatype.OCI1Artifact ||
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
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: configMTA}))
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) != 1 {
			t.Fatalf("descriptor list mismatch: %v", rl.Descriptors)
		}
		rl, err = reg.ReferrerList(ctx, r, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: "application/vnd.example.unknown"}))
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("unexpected descriptors: %v", rl.Descriptors)
		}
	})
	t.Run("List with annotation filter", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: map[string]string{extraAnnot: extraValue2}}))
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) != 1 {
			t.Fatalf("descriptor list mismatch: %v", rl.Descriptors)
		}
		rl, err = reg.ReferrerList(ctx, r, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: map[string]string{extraAnnot: "unknown value"}}))
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("unexpected descriptors: %v", rl.Descriptors)
		}
		rl, err = reg.ReferrerList(ctx, r, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: map[string]string{extraAnnot: ""}}))
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) != 2 {
			t.Fatalf("descriptor list mismatch: %v", rl.Descriptors)
		}
	})

	// delete manifest with refers
	t.Run("Delete B NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifestCheckReferrers())
		if err != nil {
			t.Fatalf("Failed running ManifestDelete: %v", err)
		}
	})
	t.Run("Delete B API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifact2M.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifestCheckReferrers())
		if err != nil {
			t.Fatalf("Failed running ManifestDelete: %v", err)
		}
	})

	t.Run("Delete A NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifest(artifactM))
		if err != nil {
			t.Fatalf("Failed running ManifestDelete: %v", err)
		}
	})
	t.Run("Delete A API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + artifactM.GetDescriptor().Digest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		err = reg.ManifestDelete(ctx, r, scheme.WithManifest(artifactM))
		if err != nil {
			t.Fatalf("Failed running ManifestDelete: %v", err)
		}
	})

	// list empty after delete
	t.Run("List empty after delete NoAPI", func(t *testing.T) {
		r, err := ref.New(tsURLNoAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("descriptors exist")
		}
	})
	t.Run("List empty after delete API", func(t *testing.T) {
		r, err := ref.New(tsURLAPI.Host + repoPath + "@" + mDigest.String())
		if err != nil {
			t.Fatalf("Failed creating ref: %v", err)
		}
		rl, err := reg.ReferrerList(ctx, r)
		if err != nil {
			t.Fatalf("Failed running ReferrerList: %v", err)
		}
		if len(rl.Descriptors) > 0 {
			t.Fatalf("descriptors exist")
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
