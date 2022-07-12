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
	"github.com/sirupsen/logrus"
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
			name:  "Basic unquoted token",
			in:    `Basic realm=/`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "/"}}},
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

func TestBearer(t *testing.T) {
	useragent := "regclient/test"
	user := "user"
	pass := "testpass"
	token1Resp, _ := json.Marshal(BearerToken{
		Token:        "token1",
		ExpiresIn:    900,
		IssuedAt:     time.Now().Add(-900 * time.Second), // testing time skew handling
		Scope:        "repository:reponame:pull",
		RefreshToken: "refresh-token-value",
	})
	tokenPassForm := url.Values{}
	tokenPassForm.Set("scope", "repository:reponame:pull")
	tokenPassForm.Set("service", "test")
	tokenPassForm.Set("client_id", useragent)
	tokenPassForm.Set("grant_type", "password")
	tokenPassForm.Set("username", user)
	tokenPassForm.Set("password", pass)
	tokenPassBody := tokenPassForm.Encode()
	tokenRefreshForm := url.Values{}
	tokenRefreshForm.Set("scope", "repository:reponame:pull")
	tokenRefreshForm.Set("service", "test")
	tokenRefreshForm.Set("client_id", useragent)
	tokenRefreshForm.Set("grant_type", "refresh_token")
	tokenRefreshForm.Set("refresh_token", "refresh-token-value")
	tokenRefreshBody := tokenRefreshForm.Encode()
	token2Resp, _ := json.Marshal(BearerToken{
		Token:        "token2",
		ExpiresIn:    10,                                // testing short expiration
		IssuedAt:     time.Now().Add(900 * time.Second), // testing time skew handling
		Scope:        "repository:reponame:pull,push",
		RefreshToken: "refresh-token-value",
	})
	token2PassForm := url.Values{}
	token2PassForm.Set("scope", "repository:reponame:pull,push")
	token2PassForm.Set("service", "test")
	token2PassForm.Set("client_id", useragent)
	token2PassForm.Set("grant_type", "password")
	token2PassForm.Set("username", user)
	token2PassForm.Set("password", pass)
	token2PassBody := token2PassForm.Encode()
	rrs := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token1",
				Method: "POST",
				Path:   "/tokens",
				Body:   []byte(tokenPassBody),
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token1Resp,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token1 refresh",
				Method: "POST",
				Path:   "/tokens",
				Body:   []byte(tokenRefreshBody),
			},
			RespEntry: reqresp.RespEntry{
				Status: 200,
				Body:   token1Resp,
			},
		},
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "req token2",
				Method: "POST",
				Path:   "/tokens",
				Body:   []byte(token2PassBody),
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
	bearer := NewBearerHandler(&http.Client{}, useragent, tsHost,
		func(h string) Cred { return Cred{User: user, Password: pass} },
		&logrus.Logger{},
	).(*BearerHandler)

	// handle token1, verify expired token gets current time and isn't expired
	err := bearer.AddScope("repository:reponame:pull")
	if err != nil {
		t.Errorf("failed adding scope: %v", err)
	}
	c, err := ParseAuthHeader(
		`Bearer realm="` + tsURL.String() +
			`/tokens",service="test"` +
			`,scope="repository:reponame:pull"`)
	if err != nil {
		t.Errorf("failed on parse challenge 1: %v", err)
	}
	err = bearer.ProcessChallenge(c[0])
	if err != nil {
		t.Errorf("failed on response to token1: %v", err)
	}
	resp1, err := bearer.GenerateAuth()
	if err != nil {
		t.Errorf("failed to generate auth response1: %v", err)
	}
	if resp1 != "Bearer token1" {
		t.Errorf("token1 is invalid, expected %s, received %s", "Bearer token1", resp1)
	}
	if bearer.isExpired() {
		t.Errorf("token1 is already expired")
	}

	// send a second request without another challenge
	err = bearer.AddScope("repository:reponame:pull")
	if err != nil && !errors.Is(err, ErrNoNewChallenge) {
		t.Errorf("failed adding scope: %v", err)
	}
	resp1a, err := bearer.GenerateAuth()
	if err != nil {
		t.Errorf("failed to generate auth response1 (rerun): %v", err)
	}
	if resp1a != "Bearer token1" {
		t.Errorf("token1 (rerun) is invalid, expected %s, received %s", "Bearer token1", resp1a)
	}
	if bearer.isExpired() {
		t.Errorf("token1 (rerun) is already expired")
	}

	// send a third request with same challenge after token expires
	bearer.token.IssuedAt = time.Now().Add(-900 * time.Second)
	err = bearer.AddScope("repository:reponame:pull")
	if err != nil && !errors.Is(err, ErrNoNewChallenge) {
		t.Errorf("failed adding scope: %v", err)
	}
	err = bearer.ProcessChallenge(c[0])
	if err != nil {
		t.Errorf("failed reprocess challenge on expired token: %v", err)
	}
	resp1b, err := bearer.GenerateAuth()
	if err != nil {
		t.Errorf("failed to generate auth response1 (expired): %v", err)
	}
	if resp1b != "Bearer token1" {
		t.Errorf("token1 (expired) is invalid, expected %s, received %s", "Bearer token1", resp1b)
	}
	if bearer.isExpired() {
		t.Errorf("token1 (expired) is already expired")
	}

	// send a request for a new scope
	err = bearer.AddScope("repository:reponame:pull,push")
	if err != nil {
		t.Errorf("failed adding scope: %v", err)
	}
	resp2, err := bearer.GenerateAuth()
	if err != nil {
		t.Errorf("failed to generate auth response2 (push): %v", err)
	}
	if resp2 != "Bearer token2" {
		t.Errorf("token2 (push) is invalid, expected %s, received %s", "Bearer token2", resp2)
	}
	if bearer.isExpired() {
		t.Errorf("token2 (push) is already expired")
	}
	if bearer.token.IssuedAt.After(time.Now().UTC()) {
		t.Errorf("token2 (push) is after current time")
	}
	if bearer.token.ExpiresIn < minTokenLife {
		t.Errorf("token2 (push) expires early, expected %d, received %d", minTokenLife, bearer.token.ExpiresIn)
	}
}
