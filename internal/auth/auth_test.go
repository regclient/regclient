package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/regclient/regclient/internal/reqresp"
)

func TestParseAuthHeader(t *testing.T) {
	var tests = []struct {
		name, in string
		wantC    []Challenge
		wantE    error
	}{
		{
			name:  "Bearer to auth.docker.io",
			in:    `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:docker/docker:pull"`,
			wantC: []Challenge{{authType: "bearer", params: map[string]string{"realm": "https://auth.docker.io/token", "service": "registry.docker.io", "scope": "repository:docker/docker:pull"}}},
			wantE: nil,
		},
		{
			name:  "Basic to GitHub",
			in:    `Basic realm="GitHub Package Registry"`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "GitHub Package Registry"}}},
			wantE: nil,
		},
		{
			name:  "Basic case insensitive type and key",
			in:    `BaSiC ReAlM="Case insensitive key"`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "Case insensitive key"}}},
			wantE: nil,
		},
		{
			name:  "Basic unquoted realm",
			in:    `Basic realm=unquoted`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "unquoted"}}},
			wantE: nil,
		},
		{
			name:  "Missing close quote",
			in:    `Basic realm="GitHub Package Registry`,
			wantC: []Challenge{},
			wantE: ErrParseFailure,
		},
		{
			name:  "Missing value after escape",
			in:    `Basic realm="GitHub Package Registry\\`,
			wantC: []Challenge{},
			wantE: ErrParseFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseAuthHeader(tt.in)
			if err != tt.wantE {
				t.Errorf("got error %v, want %v", err, tt.wantE)
			}
			if err != nil || tt.wantE != nil {
				return
			}
			if len(c) != len(tt.wantC) {
				t.Errorf("got number of challenges %d, want %d", len(c), len(tt.wantC))
			}
			for i := range tt.wantC {
				if c[i].authType != tt.wantC[i].authType {
					t.Errorf("c[%d] got authtype %s, want %s", i, c[i].authType, tt.wantC[i].authType)
				}
				for k := range tt.wantC[i].params {
					if c[i].params[k] != tt.wantC[i].params[k] {
						t.Errorf("c[%d] param %s got %s, want %s", i, k, c[i].params[k], tt.wantC[i].params[k])
					}
				}
			}
		})
	}
}

// TestAuth checks the auth interface using a mock http server
func TestAuth(t *testing.T) {
	token1Resp, _ := json.Marshal(BearerToken{
		Token:     "token1",
		ExpiresIn: 900,
		IssuedAt:  time.Now(),
		Scope:     "repository:reponame:pull",
	})
	token2Resp, _ := json.Marshal(BearerToken{
		Token:     "token2",
		ExpiresIn: 900,
		IssuedAt:  time.Now(),
		Scope:     "repository:reponame:pull,push",
	})
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token1",
				Method: "POST",
				Path:   "/token1",
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token1Resp,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token2 POST",
				Method: "POST",
				Path:   "/token2",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token2 GET",
				Method: "GET",
				Path:   "/token2",
				Headers: http.Header{
					"Authorization": {"Basic dXNlcjpwYXNz"},
				},
				Query: map[string][]string{
					"scope": {"repository:reponame:pull,push"},
				},
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token2Resp,
			},
		},
	}
	ts := httptest.NewServer(reqresp.NewHandler(t, rrs))
	defer ts.Close()
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host

	tests := []struct {
		name           string
		auth           Auth
		addScopeHost   string
		addScopeScope  string
		handleResponse *http.Response
		handleRequest  *http.Request
		wantErrScope   error
		wantErrResp    error
		wantErrReq     error
		wantAuthHeader string
	}{
		{
			name: "empty",
			auth: NewAuth(),
			handleResponse: &http.Response{
				Request: &http.Request{
					URL: tsURL,
				},
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{
					"WWW-Authenticate": []string{},
				},
			},
			wantErrResp: ErrEmptyChallenge,
		},
		{
			name: "basic",
			auth: NewAuth(
				WithCreds(func(s string) Cred {
					return Cred{User: "user", Password: "pass"}
				}),
			),
			handleResponse: &http.Response{
				Request: &http.Request{
					URL: tsURL,
				},
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{`Basic realm="test server"`},
				},
			},
			handleRequest: &http.Request{
				URL:    tsURL,
				Header: http.Header{},
			},
			wantAuthHeader: "Basic dXNlcjpwYXNz",
		},
		{
			name: "bearer1",
			auth: NewAuth(
				WithCreds(func(s string) Cred {
					return Cred{User: "user", Password: "pass"}
				}),
			),
			handleResponse: &http.Response{
				Request: &http.Request{
					URL: tsURL,
				},
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="` + tsURL.String() + `/token1",service="` + tsHost + `",scope="repository:reponame:pull"`,
					},
				},
			},
			handleRequest: &http.Request{
				URL:    tsURL,
				Header: http.Header{},
			},
			wantAuthHeader: "Bearer token1",
		},
		{
			name: "bearer2",
			auth: NewAuth(
				WithCreds(func(s string) Cred {
					return Cred{User: "user", Password: "pass"}
				}),
			),
			handleResponse: &http.Response{
				Request: &http.Request{
					URL: tsURL,
				},
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="` + tsURL.String() + `/token2",service="` + tsHost + `",scope="repository:reponame:pull"`,
					},
				},
			},
			addScopeHost:  tsHost,
			addScopeScope: "repository:reponame:pull,push",
			handleRequest: &http.Request{
				URL:    tsURL,
				Header: http.Header{},
			},
			wantAuthHeader: "Bearer token2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.handleResponse != nil {
				err := tt.auth.HandleResponse(tt.handleResponse)
				if tt.wantErrResp != nil {
					if err == nil {
						t.Errorf("HandleResponse did not receive error")
					} else if !errors.Is(err, tt.wantErrResp) && err.Error() != tt.wantErrResp.Error() {
						t.Errorf("HandleResponse unexpected error, expected %v, received %v", tt.wantErrResp, err)
					}
				} else if err != nil {
					t.Errorf("HandleResponse error: %v", err)
				}
			}
			if tt.addScopeScope != "" {
				err := tt.auth.AddScope(tt.addScopeHost, tt.addScopeScope)
				if tt.wantErrScope != nil {
					if err == nil {
						t.Errorf("AddScope did not receive error")
					} else if !errors.Is(err, tt.wantErrScope) && err.Error() != tt.wantErrScope.Error() {
						t.Errorf("AddScope unexpected error, expected %v, received %v", tt.wantErrScope, err)
					}
				} else if err != nil {
					t.Errorf("AddScope error: %v", err)
				}
			}
			if tt.handleRequest != nil {
				err := tt.auth.UpdateRequest(tt.handleRequest)
				if tt.wantErrReq != nil {
					if err == nil {
						t.Errorf("UpdateRequest did not receive error")
					} else if !errors.Is(err, tt.wantErrReq) && err.Error() != tt.wantErrReq.Error() {
						t.Errorf("UpdateRequest unexpected error, expected %v, received %v", tt.wantErrReq, err)
					}
				} else if err != nil {
					t.Errorf("UpdateRequest error: %v", err)
				}
			}
			if tt.wantAuthHeader != "" && tt.handleRequest != nil {
				ah := tt.handleRequest.Header.Get("Authorization")
				if ah != tt.wantAuthHeader {
					t.Errorf("Authorization header, expected %s, received %s", tt.wantAuthHeader, ah)
				}
			}
		})
	}

}
