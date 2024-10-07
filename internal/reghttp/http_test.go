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
	"os"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/warning"
)

// TODO: test for race conditions

type testBearerToken struct {
	Token        string    `json:"token"`
	AccessToken  string    `json:"access_token"`
	ExpiresIn    int       `json:"expires_in"`
	IssuedAt     time.Time `json:"issued_at"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
}

func TestRegHttp(t *testing.T) {
	t.Parallel()
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
	reqPerSec := 50.0
	token1GForm := url.Values{}
	token1GForm.Set("scope", "repository:project:pull")
	token1GForm.Set("service", "test")
	token1GForm.Set("client_id", useragent)
	token1GForm.Set("grant_type", "password")
	token1GForm.Set("username", user)
	token1GForm.Set("password", pass)
	token1GBody := token1GForm.Encode()
	token1GValue := "token1GValue"
	token1GResp, _ := json.Marshal(testBearerToken{
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
	token1PResp, _ := json.Marshal(testBearerToken{
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
	token2GResp, _ := json.Marshal(testBearerToken{
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
	token2PResp, _ := json.Marshal(testBearerToken{
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
				Name:   "redirect req",
				Method: "GET",
				Path:   "/v2/project-redirect/manifests/tag-auth",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusTemporaryRedirect,
				Headers: http.Header{
					"Location": []string{"/v2/project/manifests/tag-auth"},
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
				Name:   "mirror-limit manifest",
				Method: "GET",
				Path:   "/v2/mirror-limit/project/manifests/tag-get",
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
				Name:   "mirror-1 down",
				Method: "GET",
				Path:   "/v2/mirror-1/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "mirror-2 down",
				Method: "GET",
				Path:   "/v2/mirror-2/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "mirror-3 down",
				Method: "GET",
				Path:   "/v2/mirror-3/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "mirror-4 down",
				Method: "GET",
				Path:   "/v2/mirror-4/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "mirror-5 down",
				Method: "GET",
				Path:   "/v2/mirror-5/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "mirror-6 down",
				Method: "GET",
				Path:   "/v2/mirror-6/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "mirror-7 down",
				Method: "GET",
				Path:   "/v2/mirror-7/project/manifests/tag-get",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusBadGateway,
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
	configHosts := map[string]*config.Host{
		tsHost: {
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		"auth." + tsHost: {
			Name:     "auth." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			User:     user,
			Pass:     pass,
		},
		"unauth." + tsHost: {
			Name:     "unauth." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			User:     user,
			Pass:     "bad" + pass,
		},
		"repoauth." + tsHost: {
			Name:     "repoauth." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			User:     user,
			Pass:     pass,
			RepoAuth: true,
		},
		"nohead." + tsHost: {
			Name:     "nohead." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
			APIOpts: map[string]string{
				"disableHead": "true",
			},
		},
		"missing." + tsHost: {
			Name:       "missing." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-missing",
			Priority:   5,
		},
		"forbidden." + tsHost: {
			Name:       "forbidden." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-forbidden",
			Priority:   6,
		},
		"bad-gw." + tsHost: {
			Name:       "bad-gw." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-bad-gw",
			Priority:   7,
		},
		"gw-timeout." + tsHost: {
			Name:       "gw-timeout." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-gw-timeout",
			Priority:   8,
		},
		"server-error." + tsHost: {
			Name:       "server-error." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-server-error",
			Priority:   4,
		},
		"rate-limit." + tsHost: {
			Name:       "rate-limit." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-rate-limit",
			Priority:   1,
		},
		"mirrors." + tsHost: {
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
		"mirror-limit." + tsHost: {
			Name:       "mirror-limit." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-limit",
			Priority:   20,
			Mirrors: []string{
				"missing." + tsHost,
				"bad-gw." + tsHost,
				"forbidden." + tsHost,
				"gw-timeout." + tsHost,
				"mirror-1." + tsHost,
				"mirror-2." + tsHost,
				"mirror-3." + tsHost,
				"mirror-4." + tsHost,
				"mirror-5." + tsHost,
				"mirror-6." + tsHost,
				"mirror-7." + tsHost,
			},
		},
		"mirror-1." + tsHost: {
			Name:       "mirror-1." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-1",
			Priority:   1,
		},
		"mirror-2." + tsHost: {
			Name:       "mirror-2." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-2",
			Priority:   1,
		},
		"mirror-3." + tsHost: {
			Name:       "mirror-3." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-3",
			Priority:   1,
		},
		"mirror-4." + tsHost: {
			Name:       "mirror-4." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-4",
			Priority:   1,
		},
		"mirror-5." + tsHost: {
			Name:       "mirror-5." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-5",
			Priority:   1,
		},
		"mirror-6." + tsHost: {
			Name:       "mirror-6." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-6",
			Priority:   1,
		},
		"mirror-7." + tsHost: {
			Name:       "mirror-7." + tsHost,
			Hostname:   tsHost,
			TLS:        config.TLSDisabled,
			PathPrefix: "mirror-7",
			Priority:   1,
		},
		"retry." + tsHost: {
			Name:     "retry." + tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		"req-per-sec." + tsHost: {
			Name:      "req-per-sec." + tsHost,
			Hostname:  tsHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: reqPerSec,
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
	delayInit, _ := time.ParseDuration("0.0005s")
	delayMax, _ := time.ParseDuration("0.0010s")
	hc := NewClient(
		WithConfigHostFn(func(name string) *config.Host {
			if configHosts[name] == nil {
				configHosts[name] = config.HostNewName(name)
			}
			return configHosts[name]
		}),
		WithDelay(delayInit, delayMax),
		WithRetryLimit(10),
		WithUserAgent(useragent),
	)

	// test standard get
	// test getting http response
	// test read/closer
	t.Run("Get", func(t *testing.T) {
		getReq := &Req{
			Host:       tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Seek", func(t *testing.T) {
		getReq := &Req{
			Host:       tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		b := make([]byte, 2)
		l, err := resp.Read(b)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
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
			t.Fatalf("body read failure: %v", err)
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
		getReq := &Req{
			Host:      tsHost,
			Method:    "GET",
			DirectURL: u,
			Headers:   headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
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

		getReq := &Req{
			Host:       tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(expCtx, getReq)
		if err == nil {
			resp.Close()
			t.Errorf("get unexpectedly succeeded")
		}
	})
	// test head requests
	t.Run("Head", func(t *testing.T) {
		headReq := &Req{
			Host:       tsHost,
			Method:     "HEAD",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, headReq)
		if err != nil {
			t.Errorf("failed to run head: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
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
		headReq := &Req{
			Host:       "nohead." + tsHost,
			Method:     "HEAD",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, headReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success running head request")
		}
	})
	// test auth
	t.Run("Auth", func(t *testing.T) {
		authReq := &Req{
			Host:       "auth." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-auth",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, authReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Unauth", func(t *testing.T) {
		authReq := &Req{
			Host:       "unauth." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-auth",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, authReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success with bad password")
		} else if !errors.Is(err, errs.ErrHTTPUnauthorized) {
			t.Errorf("expected error %v, received error %v", errs.ErrHTTPUnauthorized, err)
		}
	})
	t.Run("Bad auth", func(t *testing.T) {
		authReq := &Req{
			Host:       "repoauth." + tsHost,
			Method:     "GET",
			Repository: "project-bad-auth",
			Path:       "manifests/tag-repoauth",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, authReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success with bad auth header")
		} else if !errors.Is(err, errs.ErrParsingFailed) {
			t.Errorf("expected error %v, received error %v", errs.ErrParsingFailed, err)
		}
	})
	t.Run("Missing auth", func(t *testing.T) {
		authReq := &Req{
			Host:       "repoauth." + tsHost,
			Method:     "GET",
			Repository: "project-missing-auth",
			Path:       "manifests/tag-repoauth",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, authReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success with missing auth header")
		} else if !errors.Is(err, errs.ErrEmptyChallenge) {
			t.Errorf("expected error %v, received error %v", errs.ErrEmptyChallenge, err)
		}
	})
	// test redirect with auth
	t.Run("redirect-auth", func(t *testing.T) {
		authReq := &Req{
			Host:       "auth." + tsHost,
			Method:     "GET",
			Repository: "project-redirect",
			Path:       "manifests/tag-auth",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, authReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test repoauth
	t.Run("RepoAuth", func(t *testing.T) {
		authReq1G := &Req{
			Host:       "repoauth." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-repoauth",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, authReq1G)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}

		authReq1P := &Req{
			Host:       "repoauth." + tsHost,
			Method:     "PUT",
			Repository: "project",
			Path:       "manifests/tag-repoauth",
			Headers:    headers,
			BodyBytes:  putBody,
		}
		resp, err = hc.Do(ctx, authReq1P)
		if err != nil {
			t.Fatalf("failed to run put: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 201 {
			t.Errorf("invalid status code, expected 201, received %d", resp.HTTPResponse().StatusCode)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}

		authReq2G := &Req{
			Host:       "repoauth." + tsHost,
			Method:     "GET",
			Repository: "project2",
			Path:       "manifests/tag-repoauth",
			Headers:    headers,
		}
		resp, err = hc.Do(ctx, authReq2G)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err = io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}

		authReq2P := &Req{
			Host:       "repoauth." + tsHost,
			Method:     "PUT",
			Repository: "project2",
			Path:       "manifests/tag-repoauth",
			Headers:    headers,
			BodyBytes:  putBody,
		}
		resp, err = hc.Do(ctx, authReq2P)
		if err != nil {
			t.Fatalf("failed to run put: %v", err)
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
		postReq := &Req{
			Host:       tsHost,
			Method:     "POST",
			Repository: "project",
			Path:       "manifests/tag-post",
			BodyBytes:  postBody,
			BodyLen:    int64(len(postBody)),
		}
		resp, err := hc.Do(ctx, postReq)
		if err != nil {
			t.Fatalf("failed to run post: %v", err)
		}
		if resp.HTTPResponse().StatusCode != http.StatusAccepted {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusAccepted, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if len(body) > 0 {
			t.Errorf("body read mismatch, expected empty body, received %s", body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Put body retry", func(t *testing.T) {
		putReq := &Req{
			Host:       tsHost,
			Method:     "PUT",
			Repository: "project",
			Path:       "manifests/tag-put-fail",
			BodyBytes:  putBody,
		}
		resp, err := hc.Do(ctx, putReq)
		if err != nil {
			t.Fatalf("failed to run put: %v", err)
		}
		if resp.HTTPResponse().StatusCode != http.StatusCreated {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusCreated, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if len(body) > 0 {
			t.Errorf("body read mismatch, expected empty body, received %s", body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Put body func", func(t *testing.T) {
		putReq := &Req{
			Host:       tsHost,
			Method:     "PUT",
			Repository: "project",
			Path:       "manifests/tag-put",
			BodyFunc:   func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(putBody)), nil },
			BodyLen:    int64(len(putBody)),
		}
		resp, err := hc.Do(ctx, putReq)
		if err != nil {
			t.Fatalf("failed to run put: %v", err)
		}
		if resp.HTTPResponse().StatusCode != http.StatusCreated {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusCreated, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
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
		deleteReq := &Req{
			Host:       tsHost,
			Method:     "DELETE",
			Repository: "project",
			Path:       "manifests/tag-delete",
		}
		resp, err := hc.Do(ctx, deleteReq)
		if err != nil {
			t.Fatalf("failed to run delete: %v", err)
		}
		if resp.HTTPResponse().StatusCode != http.StatusAccepted {
			t.Errorf("invalid status code, expected %d, received %d", http.StatusAccepted, resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
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
		getReq := &Req{
			Host:       "mirrors." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	// test the retry limit on a specific request with a long list of mirrors
	t.Run("retry-limit", func(t *testing.T) {
		getReq := &Req{
			Host:       "mirror-limit." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			_ = resp.Close()
			t.Fatalf("retry limit was not reached")
		}
		if !errors.Is(err, errs.ErrRetryLimitExceeded) {
			t.Errorf("unexpected error: expected %v, received %v", errs.ErrRetryLimitExceeded, err)
		}
	})
	// test error statuses (404, rate limit, timeout, server error)
	t.Run("Missing", func(t *testing.T) {
		getReq := &Req{
			Host:       "missing." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success on get for missing manifest")
		} else if !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrNotFound, err)
		}
	})
	t.Run("Forbidden", func(t *testing.T) {
		getReq := &Req{
			Host:       "forbidden." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success on get for missing manifest")
		} else if !errors.Is(err, errs.ErrHTTPUnauthorized) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrHTTPUnauthorized, err)
		}
	})
	t.Run("Bad GW", func(t *testing.T) {
		getReq := &Req{
			Host:       "bad-gw." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success on get for missing manifest")
		} else if !errors.Is(err, errs.ErrHTTPStatus) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrHTTPStatus, err)
		}
	})
	t.Run("GW Timeout", func(t *testing.T) {
		getReq := &Req{
			Host:       "gw-timeout." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success on get for missing manifest")
		} else if !errors.Is(err, errs.ErrHTTPStatus) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrHTTPStatus, err)
		}
	})
	t.Run("Server error", func(t *testing.T) {
		getReq := &Req{
			Host:       "server-error." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success on get for missing manifest")
		} else if !errors.Is(err, errs.ErrHTTPStatus) {
			t.Errorf("unexpected error, expected %v, received %v", errs.ErrHTTPStatus, err)
		}
	})
	// test context expire during retries
	t.Run("Rate limit and timeout", func(t *testing.T) {
		ctxTimeout, cancel := context.WithTimeout(ctx, delayInit*2)
		defer cancel()
		getReq := &Req{
			Host:       "rate-limit." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctxTimeout, getReq)
		if err == nil {
			resp.Close()
			t.Fatalf("unexpected success on get for missing manifest")
		} else if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("unexpected error, expected %v, received %v", context.DeadlineExceeded, err)
		}
	})
	// test connection dropping during read and retry
	t.Run("Short", func(t *testing.T) {
		shortReq := &Req{
			Host:       tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-short",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, shortReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
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
		retryReq := &Req{
			Host:       "retry." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-retry",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, retryReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, retryBody) {
			t.Errorf("body read mismatch, expected %s, received %s", retryBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Warning", func(t *testing.T) {
		getReq := &Req{
			Host:       tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/warning",
			Headers:    headers,
		}
		w := &warning.Warning{}
		wCtx := warning.NewContext(ctx, w)
		resp, err := hc.Do(wCtx, getReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
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
	t.Run("Host Normalized", func(t *testing.T) {
		getReq := &Req{
			Host:       tsHost + "/path",
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		resp, err := hc.Do(ctx, getReq)
		if err != nil {
			t.Fatalf("failed to run get: %v", err)
		}
		if resp.HTTPResponse().StatusCode != 200 {
			t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
		}
		body, err := io.ReadAll(resp)
		if err != nil {
			t.Fatalf("body read failure: %v", err)
		} else if !bytes.Equal(body, getBody) {
			t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
		}
		err = resp.Close()
		if err != nil {
			t.Errorf("error closing request: %v", err)
		}
	})
	t.Run("Concurrent errors", func(t *testing.T) {
		count := 3
		ctxTimeout, cancel := context.WithTimeout(ctx, delayInit*4)
		defer cancel()
		getReq := &Req{
			Host:       "rate-limit." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		chResults := make(chan error)
		for i := 0; i < count; i++ {
			go func() {
				resp, err := hc.Do(ctxTimeout, getReq)
				if err == nil {
					resp.Close()
				}
				chResults <- err
			}()
		}
		for i := 0; i < count; i++ {
			err := <-chResults
			if err == nil {
				t.Errorf("unexpected success on get for missing manifest")
			}
		}
	})
	t.Run("req-per-sec", func(t *testing.T) {
		getReq := &Req{
			Host:       "req-per-sec." + tsHost,
			Method:     "GET",
			Repository: "project",
			Path:       "manifests/tag-get",
			Headers:    headers,
		}
		start := time.Now()
		count := 10
		for i := 0; i < count; i++ {
			resp, err := hc.Do(ctx, getReq)
			if err != nil {
				t.Fatalf("failed to run get: %v", err)
			}
			if resp.HTTPResponse().StatusCode != 200 {
				t.Errorf("invalid status code, expected 200, received %d", resp.HTTPResponse().StatusCode)
			}
			body, err := io.ReadAll(resp)
			if err != nil {
				t.Fatalf("body read failure: %v", err)
			} else if !bytes.Equal(body, getBody) {
				t.Errorf("body read mismatch, expected %s, received %s", getBody, body)
			}
			err = resp.Close()
			if err != nil {
				t.Errorf("error closing request: %v", err)
			}
		}
		dur := time.Since(start)
		expectMin := (time.Second / time.Duration(reqPerSec)) * time.Duration(count-1)
		if dur < expectMin {
			t.Errorf("requests finished faster than expected time, expected %s, received %s", expectMin.String(), dur.String())
		}
	})
	// TODO: test various TLS configs (custom root for all hosts, custom root for one host, insecure)
}

// separate test class as these must run sequentially
func TestProxy(t *testing.T) {
	t.Run("empty environment", func(t *testing.T) {
		for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
			os.Unsetenv(name)
		}
		proxy, err := proxyUrlFromEnvironment()
		if err == nil || proxy != nil {
			t.Errorf("Proxy returned with empty environment: %v", proxy)
		}
	})
	t.Run("bad URL", func(t *testing.T) {
		for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
			os.Unsetenv(name)
		}
		os.Setenv("http_proxy", "://servercom")
		proxy, err := proxyUrlFromEnvironment()
		if err == nil || proxy != nil {
			t.Errorf("Proxy returned with invalid url: %v", proxy)
		}
	})
	t.Run("valid URL", func(t *testing.T) {
		for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
			os.Unsetenv(name)
		}
		os.Setenv("HTTPS_PROXY", "https://server.com")
		proxy, err := proxyUrlFromEnvironment()
		if err != nil || proxy == nil {
			t.Errorf("Failed to set up a proxy with a valid url: %v", err)
		}
		if proxy.Scheme != "https" || proxy.Host != "server.com" {
			t.Errorf("Unexpected proxy url: %v", proxy)
		}
	})
}
