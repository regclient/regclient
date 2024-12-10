package reg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
)

func TestTag(t *testing.T) {
	t.Parallel()
	seed := time.Now().UTC().Unix()
	t.Logf("Using seed %d", seed)
	repoPath := "/proj"
	repoPath2 := "/proj2"
	pageLen := 2
	listTagList := []string{"latest", "v1", "v1.1", "v1.1.1"}
	listTagBody := []byte(fmt.Sprintf("{\"name\":\"%s\",\"tags\":[\"%s\"]}",
		strings.TrimLeft(repoPath, "/"),
		strings.Join(listTagList, "\",\"")))
	listTagBody1 := []byte(fmt.Sprintf("{\"name\":\"%s\",\"tags\":[\"%s\"]}",
		strings.TrimLeft(repoPath, "/"),
		strings.Join(listTagList[:pageLen], "\",\"")))
	listTagBody2 := []byte(fmt.Sprintf("{\"name\":\"%s\",\"tags\":[\"%s\"]}",
		strings.TrimLeft(repoPath, "/"),
		strings.Join(listTagList[pageLen:], "\",\"")))
	missingRepo := "/missing"
	delOCITag := "del-oci"
	delFallbackTag := "del-fallback"
	delFallbackManifest := "digest for del-fallback"
	delFallbackDigest := digest.FromString(delFallbackManifest)
	uuid1 := reqresp.NewRandomID(seed)
	ctx := context.Background()
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "tag get page 2",
				Method: "GET",
				Path:   "/v2" + repoPath + "/tags/list",
				Query: map[string][]string{
					"n":    {fmt.Sprintf("%d", pageLen)},
					"last": {listTagList[pageLen-1]},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(listTagBody2))},
					"Content-Type":   {"application/json"},
				},
				Body: listTagBody2,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "tag get page 1",
				Method: "GET",
				Path:   "/v2" + repoPath + "/tags/list",
				Query: map[string][]string{
					"n": {fmt.Sprintf("%d", pageLen)},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(listTagBody1))},
					"Content-Type":   {"application/json"},
				},
				Body: listTagBody1,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "tag get",
				Method: "GET",
				Path:   "/v2" + repoPath + "/tags/list",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(listTagBody))},
					"Content-Type":   {"application/json"},
				},
				Body: listTagBody,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "repo2 tag get page 2",
				Method: "GET",
				Path:   "/v2" + repoPath2 + "/tags/list",
				Query: map[string][]string{
					"next": {fmt.Sprintf("%d", 1)},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(listTagBody2))},
					"Content-Type":   {"application/json"},
				},
				Body: listTagBody2,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "repo2 tag get page 1",
				Method: "GET",
				Path:   "/v2" + repoPath2 + "/tags/list",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length": {fmt.Sprintf("%d", len(listTagBody1))},
					"Content-Type":   {"application/json"},
					"Link":           {fmt.Sprintf(`<%s>; rel="next"`, "/v2"+repoPath2+"/tags/list?next=1")},
				},
				Body: listTagBody1,
			},
		},

		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "tag missing",
				Method: "GET",
				Path:   "/v2" + missingRepo + "/tags/list",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "delete OCI",
				Method: "DELETE",
				Path:   "/v2" + repoPath + "/manifests/" + delOCITag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "delete fallback tag",
				Method: "DELETE",
				Path:   "/v2" + repoPath + "/manifests/" + delFallbackTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadRequest,
				Body:   []byte("DELETE on tag not supported"),
			},
		},
		{
			// this is a loose check, since dummy manifests are unique,
			// we are trusting this matches the dummy manifest uploaded during the test
			ReqEntry: reqresp.ReqEntry{
				Name:   "delete fallback digest",
				Method: "DELETE",
				PathRE: regexp.MustCompile(regexp.QuoteMeta("/v2"+repoPath+"/manifests/sha256:") + ".*"),
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "head fallback",
				Method: "HEAD",
				Path:   "/v2" + repoPath + "/manifests/" + delFallbackTag,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(delFallbackManifest))},
					"Content-Type":          {mediatype.Docker2Manifest},
					"Docker-Content-Digest": {delFallbackDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "POST for fallback blob",
				Method: "POST",
				Path:   "/v2" + repoPath + "/blobs/uploads/",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Range":          {"bytes=0-0"},
					"Location":       {uuid1},
				},
			},
		},
		{
			// accept any blob content since fallback content is unknown
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for fallback blob",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/blobs/uploads/" + uuid1,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Headers: http.Header{
					"Content-Length": {"0"},
					"Location":       {"/v2" + repoPath + "/blobs/" + uuid1},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "PUT for fallback manifest",
				Method: "PUT",
				Path:   "/v2" + repoPath + "/manifests/" + delFallbackTag,
				Headers: http.Header{
					"Content-Type": {mediatype.Docker2Manifest},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
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
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	reg := New(
		WithConfigHosts(rcHosts),
		WithSlog(log),
		WithDelay(delayInit, delayMax),
	)

	// list tags
	t.Run("List", func(t *testing.T) {
		listRef, err := ref.New(tsURL.Host + repoPath)
		if err != nil {
			t.Fatalf("failed creating getRef: %v", err)
		}

		tl, err := reg.TagList(ctx, listRef)
		if err != nil {
			t.Fatalf("failed to list tags: %v", err)
		}
		tags, err := tl.GetTags()
		if err != nil {
			t.Fatalf("failed to extract tag list: %v", err)
		}
		if !stringSliceCmp(tags, listTagList) {
			t.Errorf("returned list mismatch, expected %v, received %v", listTagList, tags)
		}
	})
	// list tags with pagination
	t.Run("Pagination", func(t *testing.T) {
		listRef, err := ref.New(tsURL.Host + repoPath)
		if err != nil {
			t.Fatalf("failed creating getRef: %v", err)
		}
		// page 1
		tl, err := reg.TagList(ctx, listRef,
			scheme.WithTagLimit(pageLen))
		if err != nil {
			t.Fatalf("failed to list tags: %v", err)
		}
		tags, err := tl.GetTags()
		if err != nil {
			t.Fatalf("failed to extract tag list: %v", err)
		}
		if !stringSliceCmp(tags, listTagList[:pageLen]) {
			t.Errorf("returned list mismatch, expected %v, received %v", listTagList[:pageLen], tags)
		}

		// page 2
		tl, err = reg.TagList(ctx, listRef,
			scheme.WithTagLimit(pageLen),
			scheme.WithTagLast(tags[len(tags)-1]))
		if err != nil {
			t.Fatalf("failed to list tags: %v", err)
		}
		tags, err = tl.GetTags()
		if err != nil {
			t.Fatalf("failed to extract tag list: %v", err)
		}
		if !stringSliceCmp(tags, listTagList[pageLen:]) {
			t.Errorf("returned list mismatch, expected %v, received %v", listTagList[:pageLen], tags)
		}
	})
	// list tags with automatic pagination
	t.Run("Pagination automatic", func(t *testing.T) {
		listRef, err := ref.New(tsURL.Host + repoPath2)
		if err != nil {
			t.Fatalf("failed creating getRef: %v", err)
		}
		// page 1
		tl, err := reg.TagList(ctx, listRef)
		if err != nil {
			t.Fatalf("failed to list tags: %v", err)
		}
		tags, err := tl.GetTags()
		if err != nil {
			t.Fatalf("failed to extract tag list: %v", err)
		}
		if !stringSliceCmp(tags, listTagList) {
			t.Errorf("returned list mismatch, expected %v, received %v", listTagList, tags)
		}
	})
	// list tags on missing repos
	t.Run("Missing", func(t *testing.T) {
		listRef, err := ref.New(tsURL.Host + missingRepo)
		if err != nil {
			t.Fatalf("failed creating getRef: %v", err)
		}
		_, err = reg.TagList(ctx, listRef)
		if err == nil {
			t.Fatalf("tag listing succeeded on missing repo")
		} else if !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("unexpected error: expected %v, received %v", errs.ErrNotFound, err)
		}
	})

	// delete tag with OCI API
	t.Run("Delete OCI", func(t *testing.T) {
		delRef, err := ref.New(tsURL.Host + repoPath + ":" + delOCITag)
		if err != nil {
			t.Errorf("failed creating delRef: %v", err)
		}
		err = reg.TagDelete(ctx, delRef)
		if err != nil {
			t.Fatalf("failed to delete tag: %v", err)
		}
	})

	// delete tag with fallback manifest delete
	t.Run("Delete Fallback", func(t *testing.T) {
		delRef, err := ref.New(tsURL.Host + repoPath + ":" + delFallbackTag)
		if err != nil {
			t.Errorf("failed creating delRef: %v", err)
		}
		err = reg.TagDelete(ctx, delRef)
		if err != nil {
			t.Fatalf("failed to delete tag: %v", err)
		}
	})
}
