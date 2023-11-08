package reg

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

func TestPing(t *testing.T) {
	t.Parallel()
	respOkay := "{}"
	respUnauth := `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required","detail":null}]}`
	ctx := context.Background()
	contentType := "application/json"
	rrsOkay := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get Okay",
				Method: "GET",
				Path:   "/v2/",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Headers: http.Header{
					"Content-Length":                  {fmt.Sprintf("%d", len(respOkay))},
					"Content-Type":                    []string{contentType},
					"Docker-Distribution-API-Version": {"registry/2.0"},
				},
				Body: []byte(respOkay),
			},
		},
	}
	rrsUnauth := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get Unauth",
				Method: "GET",
				Path:   "/v2/",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusUnauthorized,
				Headers: http.Header{
					"WWW-Authenticate":                []string{"Basic realm=\"test\""},
					"Content-Length":                  {fmt.Sprintf("%d", len(respUnauth))},
					"Content-Type":                    []string{contentType},
					"Docker-Distribution-API-Version": {"registry/2.0"},
				},
				Body: []byte(respUnauth),
			},
		},
	}
	rrsNotFound := []reqresp.ReqResp{
		{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Get NotFound",
				Method: "GET",
				Path:   "/v2/",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotFound,
				Headers: http.Header{
					"Content-Length": {"0"},
				},
				Body: []byte(""),
			},
		},
	}
	// create a server
	tsOkay := httptest.NewServer(reqresp.NewHandler(t, rrsOkay))
	defer tsOkay.Close()
	tsUnauth := httptest.NewServer(reqresp.NewHandler(t, rrsUnauth))
	defer tsUnauth.Close()
	tsNotFound := httptest.NewServer(reqresp.NewHandler(t, rrsNotFound))
	defer tsNotFound.Close()
	// setup the reg
	tsOkayURL, _ := url.Parse(tsOkay.URL)
	tsOkayHost := tsOkayURL.Host
	tsUnauthURL, _ := url.Parse(tsUnauth.URL)
	tsUnauthHost := tsUnauthURL.Host
	tsNotFoundURL, _ := url.Parse(tsNotFound.URL)
	tsNotFoundHost := tsNotFoundURL.Host
	rcHosts := []*config.Host{
		{
			Name:     tsOkayHost,
			Hostname: tsOkayHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsUnauthHost,
			Hostname: tsUnauthHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsNotFoundHost,
			Hostname: tsNotFoundHost,
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
		WithRetryLimit(3),
	)
	t.Run("Okay", func(t *testing.T) {
		r, err := ref.NewHost(tsOkayHost)
		if err != nil {
			t.Errorf("failed to create ref \"%s\": %v", tsOkayHost, err)
			return
		}
		result, err := reg.Ping(ctx, r)
		if err != nil {
			t.Errorf("failed to ping registry: %v", err)
		}
		if result.Header == nil {
			t.Errorf("headers missing")
		} else if result.Header.Get("Content-Type") != contentType {
			t.Errorf("unexpected content type, expected %s, received %s", contentType, result.Header.Get("Content-Type"))
		}
	})
	t.Run("Unauth", func(t *testing.T) {
		r, err := ref.NewHost(tsUnauthHost)
		if err != nil {
			t.Errorf("failed to create ref \"%s\": %v", tsUnauthHost, err)
			return
		}
		result, err := reg.Ping(ctx, r)
		if err == nil {
			t.Errorf("ping did not fail")
			return
		} else if !errors.Is(err, types.ErrHTTPUnauthorized) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrHTTPUnauthorized, err)
		}
		if result.Header == nil {
			t.Errorf("headers missing")
		} else if result.Header.Get("Content-Type") != contentType {
			t.Errorf("unexpected content type, expected %s, received %s", contentType, result.Header.Get("Content-Type"))
		}
	})
	t.Run("NotFound", func(t *testing.T) {
		r, err := ref.NewHost(tsNotFoundHost)
		if err != nil {
			t.Errorf("failed to create ref \"%s\": %v", tsNotFoundHost, err)
			return
		}
		result, err := reg.Ping(ctx, r)
		if err == nil {
			t.Errorf("ping did not fail")
			return
		} else if !errors.Is(err, types.ErrNotFound) {
			t.Errorf("unexpected error, expected %v, received %v", types.ErrNotFound, err)
			return
		}
		if result.Header == nil {
			t.Errorf("headers missing")
		}
	})
}
