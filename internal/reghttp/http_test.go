package reghttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/auth"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/warning"
)

// TODO: test for race conditions
// TODO: test rate limits and concurrency

func TestRegHttp(t *testing.T) {
	ctx := context.Background()
	// setup req/resp
	useragent := "regclient/test"
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
	token1GForm := url.Values{}
	token1GForm.Set("scope", "repository:project:pull")
	token1GForm.Set("service", "test")
	token1GForm.Set("client_id", useragent)
	token1GForm.Set("grant_type", "password")
	token1GForm.Set("username", user)
	token1GForm.Set("password", pass)
	token1GBody := token1GForm.Encode()
	token1GValue := "token1GValue"
	token1GResp, _ := json.Marshal(auth.BearerToken{
		Token:        token1GValue,
		ExpiresIn:    900,
		IssuedAt:     time.Now(),
		RefreshToken: "refresh1GValue",
		Scope:        "repository:project:pull",
	})
	token1PForm := url.Values{}
	token1PForm.Set("scope", "repository:project:pull,push")
	token1PForm.Set("service", "test")
	token1PForm.Set("client_id", useragent)
	token1PForm.Set("grant_type", "password")
	token1PForm.Set("username", user)
	token1PForm.Set("password", pass)
	token1PBody := token1PForm.Encode()
	token1PValue := "token1PValue"
	token1PResp, _ := json.Marshal(auth.BearerToken{
		Token:        token1PValue,
		ExpiresIn:    900,
		IssuedAt:     time.Now(),
		RefreshToken: "refresh1PValue",
		Scope:        "repository:project:pull,push",
	})
	token2GForm := url.Values{}
	token2GForm.Set("scope", "repository:project2:pull")
	token2GForm.Set("service", "test")
	token2GForm.Set("client_id", useragent)
	token2GForm.Set("grant_type", "password")
	token2GForm.Set("username", user)
	token2GForm.Set("password", pass)
	token2GBody := token2GForm.Encode()
	token2GValue := "token2GValue"
	token2GResp, _ := json.Marshal(auth.BearerToken{
		Token:        token2GValue,
		ExpiresIn:    900,
		IssuedAt:     time.Now(),
		RefreshToken: "refresh2GValue",
		Scope:        "repository:project2:pull",
	})
	token2PForm := url.Values{}
	token2PForm.Set("scope", "repository:project2:pull,push")
	token2PForm.Set("service", "test")
	token2PForm.Set("client_id", useragent)
	token2PForm.Set("grant_type", "password")
	token2PForm.Set("username", user)
	token2PForm.Set("password", pass)
	token2PBody := token2PForm.Encode()
	token2PValue := "token2PValue"
	token2PResp, _ := json.Marshal(auth.BearerToken{
		Token:        token2PValue,
		ExpiresIn:    900,
		IssuedAt:     time.Now(),
		RefreshToken: "refresh2PValue",
		Scope:        "repository:project2:pull,push",
	})
	warnMsg1 := "test warning 1"
	warnMsg2 := "test warning 2"
	rrsToken := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token1G",
				Method: "POST",
				Path:   "/token",
				Body:   []byte(token1GBody),
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token1GResp,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token1P",
				Method: "POST",
				Path:   "/token",
				Body:   []byte(token1PBody),
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token1PResp,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token2G",
				Method: "POST",
				Path:   "/token",
				Body:   []byte(token2GBody),
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token2GResp,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token2P",
				Method: "POST",
				Path:   "/token",
				Body:   []byte(token2PBody),
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token2PResp,
			},
		},
	}
	tsToken := httptest.NewServer(reqresp.NewHandler(t, rrsToken))
	defer tsToken.Close()
	tsTokenURL, _ := url.Parse(tsToken.URL)
	tsTokenHost := tsTokenURL.Host

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
				Name:   "authorized repoauth get",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-repoauth",
				Headers: http.Header{
					"Authorization": []string{fmt.Sprintf("Bearer %s", token1GValue)},
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
				Name:   "unauthorized repoauth get",
				Method: "GET",
				Path:   "/v2/project/manifests/tag-repoauth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
				Headers: http.Header{
					"WWW-Authenticate": []string{`Bearer realm="http://` + tsTokenHost + `/token",service=test,scope="repository:project:pull"`},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "authorized repoauth put",
				Method: "PUT",
				Path:   "/v2/project/manifests/tag-repoauth",
				Body:   putBody,
				Headers: http.Header{
					"Authorization": []string{fmt.Sprintf("Bearer %s", token1PValue)},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "unauthorized repoauth put",
				Method: "PUT",
				Path:   "/v2/project/manifests/tag-repoauth",
				Body:   putBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
				Headers: http.Header{
					"WWW-Authenticate": []string{`Bearer realm="http://` + tsTokenHost + `/token",service=test,scope="repository:project:pull,push"`},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "authorized project2 repoauth get",
				Method: "GET",
				Path:   "/v2/project2/manifests/tag-repoauth",
				Headers: http.Header{
					"Authorization": []string{fmt.Sprintf("Bearer %s", token2GValue)},
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
				Name:   "unauthorized project2 repoauth get",
				Method: "GET",
				Path:   "/v2/project2/manifests/tag-repoauth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
				Headers: http.Header{
					"WWW-Authenticate": []string{`Bearer realm="http://` + tsTokenHost + `/token",service=test,scope="repository:project2:pull"`},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "authorized project2 repoauth put",
				Method: "PUT",
				Path:   "/v2/project2/manifests/tag-repoauth",
				Body:   putBody,
				Headers: http.Header{
					"Authorization": []string{fmt.Sprintf("Bearer %s", token2PValue)},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "unauthorized project2 repoauth put",
				Method: "PUT",
				Path:   "/v2/project2/manifests/tag-repoauth",
				Body:   putBody,
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
				Headers: http.Header{
					"WWW-Authenticate": []string{`Bearer realm="http://` + tsTokenHost + `/token",service=test,scope="repository:project2:pull,push"`},
				},
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "unauthorized project-missing-auth",
				Method: "GET",
				Path:   "/v2/project-missing-auth/manifests/tag-repoauth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "unauthorized project-bad-auth",
				Method: "GET",
				Path:   "/v2/project-bad-auth/manifests/tag-repoauth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Body:   []byte("Unauthorized"),
				Headers: http.Header{
					"WWW-Authenticate": []string{`Bearer realm="http://` + tsTokenHost + `/token`},
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
				Name:     "put manifest fail",
				Method:   "PUT",
				Path:     "/v2/project/manifests/tag-put-fail",
				Body:     putBody,
				IfState:  []string{"", "ok"},
				SetState: "put-fail",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusCreated,
				Fail:   true,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:     "put manifest retry",
				Method:   "PUT",
				Path:     "/v2/project/manifests/tag-put-fail",
				Body:     putBody,
				IfState:  []string{"put-fail"},
				SetState: "ok",
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
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "get warnings",
				Method: "GET",
				Path:   "/v2/project/manifests/warning",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   getBody,
				Headers: http.Header{
					"Content-Length":        {fmt.Sprintf("%d", len(getBody))},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
					"Docker-Content-Digest": []string{getDigest.String()},
					"Warning": []string{
						`199 - "ignore warning"`,
						`299 - "` + warnMsg1 + `"`,
						`299 - "` + warnMsg2 + `"`,
						`299 - "` + warnMsg1 + `"`,
					},
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
			Name:     "repoauth." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			User:     user,
			Pass:     pass,
			RepoAuth: true,
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
	hc := NewClient(
		WithConfigHosts(configHosts),
		WithDelay(delayInit, delayMax),
		WithUserAgent(useragent),
	)

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
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Seek", func(t *testing.T) {
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
		b := make([]byte, 2)
		l, err := resp.Read(b)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		}
		if l != 2 {
			t.Errorf("unexpected length, expected 2, received %d", l)
		}
		if !bytes.Equal(b, getBody[:2]) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody[:2], b)
		}
		cur, err := resp.Seek(0, io.SeekStart)
		if err != nil {
			t.Errorf("seek failure: %v", err)
		}
		if cur != 0 {
			t.Errorf("seek to unexpected offset, expected 0, received %d", cur)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Direct", func(t *testing.T) {
		u, _ := url.Parse(ts.URL)
		u.Path = "/v2/project/manifests/tag-get"
		apiGet := map[string]ReqAPI{
			"": {
				Method:    "GET",
				DirectURL: u,
				Headers:   headers,
				Digest:    getDigest,
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
		} else if !bytes.Equal(body, getBody) {
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
		} else if !errors.Is(err, types.ErrDigestMismatch) {
			t.Errorf("unexpected error from digest mismatch: %v", err)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Direct Digest", func(t *testing.T) {
		u, _ := url.Parse(ts.URL)
		u.Path = "/v2/project/manifests/tag-get"
		apiGet := map[string]ReqAPI{
			"": {
				Method:    "GET",
				DirectURL: u,
				Headers:   headers,
				Digest:    digest.FromString("bad digest"),
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
		if err == nil {
			t.Errorf("body read unexpectedly succeeded: %s", body)
		} else if !errors.Is(err, types.ErrDigestMismatch) {
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
		} else if !bytes.Equal(body, getBody) {
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
	t.Run("Bad auth", func(t *testing.T) {
		apiAuth := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project-bad-auth",
				Path:       "manifests/tag-repoauth",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		authReq := &Req{
			Host: "repoauth." + tsHost,
			APIs: apiAuth,
		}
		resp, err := hc.Do(ctx, authReq)
		if err == nil {
			t.Errorf("unexpected success with bad auth header")
			resp.Close()
			return
		} else if !errors.Is(err, types.ErrParsingFailed) {
			t.Errorf("expected error %v, received error %v", types.ErrParsingFailed, err)
		}
	})
	t.Run("Missing auth", func(t *testing.T) {
		apiAuth := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project-missing-auth",
				Path:       "manifests/tag-repoauth",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		authReq := &Req{
			Host: "repoauth." + tsHost,
			APIs: apiAuth,
		}
		resp, err := hc.Do(ctx, authReq)
		if err == nil {
			t.Errorf("unexpected success with missing auth header")
			resp.Close()
			return
		} else if !errors.Is(err, types.ErrEmptyChallenge) {
			t.Errorf("expected error %v, received error %v", types.ErrEmptyChallenge, err)
		}
	})
	// test repoauth
	t.Run("RepoAuth", func(t *testing.T) {
		apiAuth1G := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/tag-repoauth",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		authReq1G := &Req{
			Host: "repoauth." + tsHost,
			APIs: apiAuth1G,
		}
		resp, err := hc.Do(ctx, authReq1G)
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
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}

		apiAuth1P := map[string]ReqAPI{
			"": {
				Method:     "PUT",
				Repository: "project",
				Path:       "manifests/tag-repoauth",
				Headers:    headers,
				Digest:     getDigest,
				BodyBytes:  putBody,
			},
		}
		authReq1P := &Req{
			Host: "repoauth." + tsHost,
			APIs: apiAuth1P,
		}
		resp, err = hc.Do(ctx, authReq1P)
		if err != nil {
			t.Errorf("failed to run put: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 201 {
			t.Errorf("invalid status code, expected 201, received %d", resp.HTTPResponse().StatusCode)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}

		apiAuth2G := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project2",
				Path:       "manifests/tag-repoauth",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		authReq2G := &Req{
			Host: "repoauth." + tsHost,
			APIs: apiAuth2G,
		}
		resp, err = hc.Do(ctx, authReq2G)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err = io.ReadAll(resp)
		if err != nil {
			t.Errorf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}

		apiAuth2P := map[string]ReqAPI{
			"": {
				Method:     "PUT",
				Repository: "project2",
				Path:       "manifests/tag-repoauth",
				Headers:    headers,
				Digest:     getDigest,
				BodyBytes:  putBody,
			},
		}
		authReq2P := &Req{
			Host: "repoauth." + tsHost,
			APIs: apiAuth2P,
		}
		resp, err = hc.Do(ctx, authReq2P)
		if err != nil {
			t.Errorf("failed to run put: %v", err)
			return
		}
		if resp.HTTPResponse().StatusCode != 201 {
			t.Errorf("invalid status code, expected 201, received %d", resp.HTTPResponse().StatusCode)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
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
	t.Run("Put body retry", func(t *testing.T) {
		apiPut := map[string]ReqAPI{
			"": {
				Method:     "PUT",
				Repository: "project",
				Path:       "manifests/tag-put-fail",
				BodyBytes:  putBody,
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
		} else if !bytes.Equal(body, getBody) {
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
		} else if !errors.Is(err, types.ErrNotFound) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrNotFound, err)
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
		} else if !errors.Is(err, types.ErrHTTPUnauthorized) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrHTTPUnauthorized, err)
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
		} else if !errors.Is(err, types.ErrHTTPStatus) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrHTTPStatus, err)
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
		} else if !errors.Is(err, types.ErrHTTPStatus) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrHTTPStatus, err)
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
		} else if !errors.Is(err, types.ErrHTTPStatus) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrHTTPStatus, err)
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
		} else if !bytes.Equal(body, retryBody) {
			t.Errorf("body read mismatch, expected %s, received %s", retryBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Warning", func(t *testing.T) {
		apiGet := map[string]ReqAPI{
			"": {
				Method:     "GET",
				Repository: "project",
				Path:       "manifests/warning",
				Headers:    headers,
				Digest:     getDigest,
			},
		}
		getReq := &Req{
			Host: tsHost,
			APIs: apiGet,
		}
		w := &warning.Warning{}
		wCtx := warning.NewContext(ctx, w)
		resp, err := hc.Do(wCtx, getReq)
		if err != nil {
			t.Errorf("failed to run get: %v", err)
			return
		}
		if len(w.List) != 2 {
			t.Errorf("warning count, expected 2, received %d", len(w.List))
		} else {
			if w.List[0] != warnMsg1 {
				t.Errorf("warning 1, expected %s, received %s", warnMsg1, w.List[0])
			}
			if w.List[1] != warnMsg2 {
				t.Errorf("warning 2, expected %s, received %s", warnMsg2, w.List[1])
			}
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// TODO: test various TLS configs (custom root for all hosts, custom root for one host, insecure)
}
