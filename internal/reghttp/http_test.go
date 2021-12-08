package reghttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/auth"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/regclient/config"
)

// TODO: test for race conditions

func TestRegHttp(t *testing.T) {
	ctx := context.Background()
	// setup req/resp
	getBody := []byte("get body")
	getDigest := digest.FromBytes(getBody)
	postBody := []byte("{\"message\": \"Body\"}")
	putBody := []byte("{\"message\": \"Another Body\"}")
	retryBody1 := []byte("retry body 1\n")
	retryBody2 := []byte("retry body 2\n")
	retryBody3 := []byte("retry body 3\n")
	retryBody := bytes.Join([][]byte{retryBody1, retryBody2, retryBody3}, []byte{})
	retryDigest := digest.FromBytes(retryBody)
	user := "user"
	pass := "testpass"
	userAuth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "get manifest",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   getBody,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(getBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{getDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "head manifest",
				Method: "HEAD",
				Path:   "/v2/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(getBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{getDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "authorized req",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-auth",
				Headers: http.Header{
					"Authorization": []string{fmt.Sprintf("Basic %s", userAuth)},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   getBody,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(getBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{getDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "unauthorized req",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-auth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
				Headers: http.Header{
					"WWW-Authenticate": []string{"Basic realm=\"test\""},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "post manifest",
				Method: "POST",
				Path:   "/v2/project/manifests/tag-post",
				Body:   postBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "put manifest",
				Method: "PUT",
				Path:   "/v2/project/manifests/tag-put",
				Body:   putBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "delete manifest",
				Method: "DELETE",
				Path:   "/v2/project/manifests/tag-delete",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusAccepted,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "missing manifest",
				Method: "GET",
				Path:   "/v2/mirror-missing/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "forbidden manifest",
				Method: "GET",
				Path:   "/v2/mirror-forbidden/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusForbidden,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "bad-gw manifest",
				Method: "GET",
				Path:   "/v2/mirror-bad-gw/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "gw-timeout manifest",
				Method: "GET",
				Path:   "/v2/mirror-gw-timeout/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusGatewayTimeout,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "server-error manifest",
				Method: "GET",
				Path:   "/v2/mirror-server-error/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusInternalServerError,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "rate-limit manifest",
				Method: "GET",
				Path:   "/v2/mirror-rate-limit/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusTooManyRequests,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "rate-limit upstream manifest",
				Method:   "GET",
				Path:     "/v2/mirror-upstream/project/manifests/tag-get",
				DelOnUse: true,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusTooManyRequests,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "upstream manifest",
				Method: "GET",
				Path:   "/v2/mirror-upstream/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   getBody,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(getBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{getDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "short read manifest",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-short",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   retryBody1,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(retryBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{retryDigest.String()},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "retry 3 manifest",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-retry",
				Headers: http.Header{
					"Range": []string{fmt.Sprintf("bytes=%d-%d", len(retryBody1)+len(retryBody2), len(retryBody))},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   retryBody3,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(retryBody3))},
					"Content-Range":         {fmt.Sprintf("bytes %d-%d/%d", len(retryBody1)+len(retryBody2), len(retryBody), len(retryBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{retryDigest.String()},
					"Accept-Ranges":         []string{"bytes"},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "retry 2 manifest",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-retry",
				Headers: http.Header{
					"Range": []string{fmt.Sprintf("bytes=%d-%d", len(retryBody1), len(retryBody))},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   retryBody2,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(retryBody)-len(retryBody1))},
					"Content-Range":         {fmt.Sprintf("bytes %d-%d/%d", len(retryBody1), len(retryBody), len(retryBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{retryDigest.String()},
					"Accept-Ranges":         []string{"bytes"},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "retry 1 manifest",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-retry",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   retryBody1,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(retryBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{retryDigest.String()},
					"Accept-Ranges":         []string{"bytes"},
				},
			},
		},
	}
	// create a server
	ts := httptest.NewServer(reqresp.NewHandler(t, rrs))
	defer ts.Close()
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host

	// create config for various hosts, different host pointing to same dns hostname
	configHosts := []*config.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     "auth." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			User:     user,
			Pass:     pass,
		},
		{
			Name:     "unauth." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			User:     user,
			Pass:     "bad" + pass,
		},
		{
			Name:     "nohead." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			APIOpts: map[string]string{
				"disableHead": "true",
			},
		},
		{
			Name:       "missing." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-missing",
			Priority:   5,
		},
		{
			Name:       "forbidden." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-forbidden",
			Priority:   6,
		},
		{
			Name:       "bad-gw." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-bad-gw",
			Priority:   7,
		},
		{
			Name:       "gw-timeout." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-gw-timeout",
			Priority:   8,
		},
		{
			Name:       "server-error." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-server-error",
			Priority:   4,
		},
		{
			Name:       "rate-limit." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-rate-limit",
			Priority:   1,
		},
		{
			Name:       "mirrors." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-upstream",
			Priority:   8,
			Mirrors: []string{
				"missing." + tsHost,
				"bad-gw." + tsHost,
				"forbidden." + tsHost,
				"gw-timeout." + tsHost,
				"server-error." + tsHost,
			},
		},
	}

	// create APIs for requests to run
	headers := http.Header{
		"Accept": []string{
			"application/vnd.docker.distribution.manifest.v2+json",
			"application/vnd.docker.distribution.manifest.list.v2+json",
		},
	}

	// create http client
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	hc := NewClient(WithConfigHosts(configHosts), WithDelay(delayInit, delayMax))

	// test standard get
	// test getting http response
	// test read/closer
	t.Run("Get", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if bytes.Compare(body, getBody) != 0 {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test digest validation
	t.Run("Bad Digest", func(t *testing.T) {
		apiBadDigest := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     digest.FromString("bad digest"),
			},
		}
		badDigestReq := &Req{
			Host: tsHost,
			APIs: apiBadDigest,
		}
		resp, err := hc.Do(ctx, badDigestReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err == nil {
			t.Errorf("body read unexpectedly succeeded: %s", body)
		} else if !errors.Is(err, ErrDigestMismatch) {
			t.Errorf("unexpected error from digest mismatch: %v", err)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test context already expired
	t.Run("Expired Context", func(t *testing.T) {
		deadline := time.Now().Add(-1 * time.Second)
		expCtx, cancelFunc := context.WithDeadline(ctx, deadline)
		defer cancelFunc()

		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(expCtx, getReq)
		if err == nil {
			t.Errorf("get unexpectedly succeeded")
			resp.Close()
			return
		}
	})
	// test head requests
	t.Run("Head", func(t *testing.T) {
		apiHead := map[string]ReqAPI{
			"": {
				Method:     "HEAD",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		headReq := &Req{
			Host: tsHost,
			APIs: apiHead,
		}
		resp, err := hc.Do(ctx, headReq)
		if err != nil {
			t.Errorf("failed to run head: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if len(body) > 0 {
			t.Errorf("body read mismatch, expected empty body, received %s", body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test disabled head request
	t.Run("Disabled Head", func(t *testing.T) {
		apiHead := map[string]ReqAPI{
			"": {
				Method:     "HEAD",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		headReq := &Req{
			Host: "nohead." + tsHost,
			APIs: apiHead,
		}
		resp, err := hc.Do(ctx, headReq)
		if err == nil {
			t.Errorf("unexpected success running head request")
			resp.Close()
			return
		}
	})
	// test auth
	t.Run("Auth", func(t *testing.T) {
		apiAuth := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-auth",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		authReq := &Req{
			Host: "auth." + tsHost,
			APIs: apiAuth,
		}
		resp, err := hc.Do(ctx, authReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if bytes.Compare(body, getBody) != 0 {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Unauth", func(t *testing.T) {
		apiAuth := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-auth",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		authReq := &Req{
			Host: "unauth." + tsHost,
			APIs: apiAuth,
		}
		resp, err := hc.Do(ctx, authReq)
		if err == nil {
			t.Errorf("unexpected success with bad password")
			resp.Close()
			return
		} else if !errors.Is(err, auth.ErrUnauthorized) {
			t.Errorf("expected error %v, received error %v", auth.ErrUnauthorized, err)
		}
	})
	// test body func and body string
	t.Run("Post body string", func(t *testing.T) {
		apiPost := map[string]ReqAPI{
			"": {
				Method:     "POST",
				Repository: "project",
				Path:       "manifests/tag-post",
				BodyBytes:  postBody,
				BodyLen:    int64(len(postBody)),
			},
		}
		postReq := &Req{
			Host: tsHost,
			APIs: apiPost,
		}
		resp, err := hc.Do(ctx, postReq)
		if err != nil {
			t.Errorf("failed to run post: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != http.StatusAccepted {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusAccepted, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if len(body) > 0 {
			t.Errorf("body read mismatch, expected empty body, received %s", body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Put body func", func(t *testing.T) {
		apiPut := map[string]ReqAPI{
			"": {
				Method:     "PUT",
				Repository: "project",
				Path:       "manifests/tag-put",
				BodyFunc:   func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(putBody)), nil },
				BodyLen:    int64(len(putBody)),
			},
		}
		putReq := &Req{
			Host: tsHost,
			APIs: apiPut,
		}
		resp, err := hc.Do(ctx, putReq)
		if err != nil {
			t.Errorf("failed to run put: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != http.StatusCreated {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusCreated, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if len(body) > 0 {
			t.Errorf("body read mismatch, expected empty body, received %s", body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test delete
	t.Run("Delete", func(t *testing.T) {
		apiDelete := map[string]ReqAPI{
			"": {
				Method:     "DELETE",
				Repository: "project",
				Path:       "manifests/tag-delete",
			},
		}
		deleteReq := &Req{
			Host: tsHost,
			APIs: apiDelete,
		}
		resp, err := hc.Do(ctx, deleteReq)
		if err != nil {
			t.Errorf("failed to run delete: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != http.StatusAccepted {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusAccepted, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if len(body) > 0 {
			t.Errorf("body read mismatch, expected empty body, received %s", body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test a list of bad mirrors to ensure fall back
	t.Run("Mirrors", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
				// IgnoreErr:  true,
			},
		}
		getReq := &Req{
			Host: "mirrors." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if bytes.Compare(body, getBody) != 0 {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test error statuses (404, rate limit, timeout, server error)
	t.Run("Missing", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: "missing." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			t.Errorf("unexpected success on get for missing manifest")
			resp.Close()
			return
		} else if !errors.Is(err, ErrNotFound) {
			t.Errorf("unexpected error, expected %v, received %v", ErrNotFound, err)
		}
	})
	t.Run("Forbidden", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: "forbidden." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			t.Errorf("unexpected success on get for missing manifest")
			resp.Close()
			return
		} else if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("unexpected error, expected %v, received %v", ErrUnauthorized, err)
		}
	})
	t.Run("Bad GW", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: "bad-gw." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			t.Errorf("unexpected success on get for missing manifest")
			resp.Close()
			return
		} else if !errors.Is(err, ErrHttpStatus) {
			t.Errorf("unexpected error, expected %v, received %v", ErrHttpStatus, err)
		}
	})
	t.Run("GW Timeout", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: "gw-timeout." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			t.Errorf("unexpected success on get for missing manifest")
			resp.Close()
			return
		} else if !errors.Is(err, ErrHttpStatus) {
			t.Errorf("unexpected error, expected %v, received %v", ErrHttpStatus, err)
		}
	})
	t.Run("Server error", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: "server-error." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			t.Errorf("unexpected success on get for missing manifest")
			resp.Close()
			return
		} else if !errors.Is(err, ErrHttpStatus) {
			t.Errorf("unexpected error, expected %v, received %v", ErrHttpStatus, err)
		}
	})
	// test context expire during retries
	t.Run("Rate limit and timeout", func(t *testing.T) {
		ctxTimeout, cancel := context.WithTimeout(ctx, delayInit*2)
		defer cancel()
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-get",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: "rate-limit." + tsHost,
			APIs: apiGet,
		}
		resp, err := hc.Do(ctxTimeout, getReq)
		if err == nil {
			t.Errorf("unexpected success on get for missing manifest")
			resp.Close()
			return
		} else if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("unexpected error, expected %v, received %v", context.DeadlineExceeded, err)
		}
	})
	// test connection dropping during read and retry
	t.Run("Short", func(t *testing.T) {
		apiShort := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-short",
				Headers:    headers,
			},
		}
		shortReq := &Req{
			Host: tsHost,
			APIs: apiShort,
		}
		resp, err := hc.Do(ctx, shortReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err == nil {
			t.Errorf("body read unexpectedly succeeded: %s", body)
		} else if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("unexpected error, expected %v, received %v", io.ErrUnexpectedEOF, err)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Retry", func(t *testing.T) {
		apiRetry := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-retry",
				Headers:    headers,
				Digest:     retryDigest,
			},
		}
		retryReq := &Req{
			Host: tsHost,
			APIs: apiRetry,
		}
		resp, err := hc.Do(ctx, retryReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if bytes.Compare(body, retryBody) != 0 {
			t.Errorf("body read mismatch, expected %s, received %s", retryBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
}
