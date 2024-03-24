package reg

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reqresp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/errs"
)

func TestRepo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	partialLen := 2
	listRegistry := []string{
		"library/alpine",
		"library/busybox",
		"library/debian",
		"library/golang",
	}
	rrss := map[string][]reqresp.ReqResp{
		"empty": {{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Empty List",
				Method: "GET",
				Path:   "/v2/_catalog",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   []byte(`{"repositories":[]}`),
				Headers: http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
				},
			},
		}},
		"disabled": {{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Disabled API",
				Method: "GET",
				Path:   "/v2/_catalog",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusNotImplemented,
			},
		}},
		"unknown-mt": {{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Unknown MT",
				Method: "GET",
				Path:   "/v2/_catalog",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   []byte(`{"version":2}`),
				Headers: http.Header{
					"Content-Type": {"application/unknown"},
				},
			},
		}},
		"parse-error": {{
			ReqEntry: reqresp.ReqEntry{
				Name:   "Parse error",
				Method: "GET",
				Path:   "/v2/_catalog",
			},
			RespEntry: reqresp.RespEntry{
				Status: http.StatusOK,
				Body:   []byte(`{"repositories":["some-project"`),
				Headers: http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
				},
			},
		}},
		"registry": {
			{
				ReqEntry: reqresp.ReqEntry{
					Name:   "First n",
					Method: "GET",
					Path:   "/v2/_catalog",
					Query: map[string][]string{
						"n": {fmt.Sprintf("%d", partialLen)},
					},
				},
				RespEntry: reqresp.RespEntry{
					Status: http.StatusOK,
					Body:   []byte(fmt.Sprintf(`{"repositories":["%s"]}`, strings.Join(listRegistry[:partialLen], `","`))),
					Headers: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
			},
			{
				ReqEntry: reqresp.ReqEntry{
					Name:   "Remainder",
					Method: "GET",
					Path:   "/v2/_catalog",
					Query: map[string][]string{
						"last": {listRegistry[partialLen-1]},
					},
				},
				RespEntry: reqresp.RespEntry{
					Status: http.StatusOK,
					Body:   []byte(fmt.Sprintf(`{"repositories":["%s"]}`, strings.Join(listRegistry[partialLen:], `","`))),
					Headers: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
			},
			{
				ReqEntry: reqresp.ReqEntry{
					Name:   "Full List",
					Method: "GET",
					Path:   "/v2/_catalog",
				},
				RespEntry: reqresp.RespEntry{
					Status: http.StatusOK,
					Body:   []byte(fmt.Sprintf(`{"repositories":["%s"]}`, strings.Join(listRegistry, `","`))),
					Headers: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
			},
		},
	}
	tss := map[string]*httptest.Server{}
	rcHosts := []*config.Host{}
	for name := range rrss {
		rrss[name] = append(rrss[name], reqresp.BaseEntries...)
		// create a server
		tss[name] = httptest.NewServer(reqresp.NewHandler(t, rrss[name]))
		defer tss[name].Close()
		tsURL, _ := url.Parse(tss[name].URL)
		tsHost := tsURL.Host
		rcHosts = append(rcHosts, &config.Host{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		})
	}
	// setup the reg
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
	// empty list
	t.Run("Empty", func(t *testing.T) {
		u, _ := url.Parse(tss["empty"].URL)
		host := u.Host
		rl, err := reg.RepoList(ctx, host)
		if err != nil {
			t.Fatalf("error listing repos: %v", err)
		}
		rlRepos, err := rl.GetRepos()
		if err != nil {
			t.Errorf("error retrieving repos: %v", err)
		} else if len(rlRepos) > 0 {
			t.Errorf("repositories found on empty server: %v", rlRepos)
		}
	})
	// normal list
	t.Run("Registry", func(t *testing.T) {
		u, _ := url.Parse(tss["registry"].URL)
		host := u.Host
		rl, err := reg.RepoList(ctx, host)
		if err != nil {
			t.Fatalf("error listing repos: %v", err)
		}
		rlRepos, err := rl.GetRepos()
		if err != nil {
			t.Errorf("error retrieving repos: %v", err)
		} else if stringSliceCmp(listRegistry, rlRepos) == false {
			t.Errorf("repositories do not match: expected %v, received %v", listRegistry, rlRepos)
		}
	})
	// test with options
	t.Run("Pagenation", func(t *testing.T) {
		u, _ := url.Parse(tss["registry"].URL)
		host := u.Host
		rl, err := reg.RepoList(ctx, host, scheme.WithRepoLimit(partialLen))
		if err != nil {
			t.Fatalf("error listing repos: %v", err)
		}
		rlRepos, err := rl.GetRepos()
		if err != nil {
			t.Errorf("error retrieving repos (limit): %v", err)
		} else if stringSliceCmp(listRegistry[:partialLen], rlRepos) == false {
			t.Errorf("repositories do not match: expected %v, received %v", listRegistry[:partialLen], rlRepos)
		}

		rl, err = reg.RepoList(ctx, host, scheme.WithRepoLast(rlRepos[len(rlRepos)-1]))
		if err != nil {
			t.Fatalf("error listing repos: %v", err)
		}
		rlRepos, err = rl.GetRepos()
		if err != nil {
			t.Errorf("error retrieving repos (last): %v", err)
		} else if stringSliceCmp(listRegistry[partialLen:], rlRepos) == false {
			t.Errorf("repositories do not match: expected %v, received %v", listRegistry[partialLen:], rlRepos)
		}

	})
	// test with http errors
	t.Run("Disabled", func(t *testing.T) {
		u, _ := url.Parse(tss["disabled"].URL)
		host := u.Host
		_, err := reg.RepoList(ctx, host)
		if err == nil {
			t.Errorf("unexpected success listing repos on disabled registry")
		} else if !errors.Is(err, errs.ErrHTTPStatus) {
			t.Errorf("unexpected error: expected %v, received %v", errs.ErrHTTPStatus, err)
		}
	})
	// test with unknown media-type header
	t.Run("Unknown MT", func(t *testing.T) {
		u, _ := url.Parse(tss["unknown-mt"].URL)
		host := u.Host
		_, err := reg.RepoList(ctx, host)
		if err == nil {
			t.Errorf("unexpected success listing repos on unknown-mt registry")
		} else if !errors.Is(err, errs.ErrUnsupportedMediaType) {
			t.Errorf("unexpected error: expected %v, received %v", errs.ErrUnsupportedMediaType, err)
		}
	})
	// test with parsing errors
	t.Run("Parse error", func(t *testing.T) {
		u, _ := url.Parse(tss["parse-error"].URL)
		host := u.Host
		_, err := reg.RepoList(ctx, host)
		if err == nil {
			t.Errorf("unexpected success listing repos on parse-error registry")
		}
		// error is a json error, no custom error type was made for this yet
	})
	t.Run("Normalize host", func(t *testing.T) {
		u, _ := url.Parse(tss["registry"].URL)
		host := u.Host
		rl, err := reg.RepoList(ctx, host+"/path")
		if err != nil {
			t.Fatalf("error listing repos: %v", err)
		}
		rlRepos, err := rl.GetRepos()
		if err != nil {
			t.Errorf("error retrieving repos: %v", err)
		} else if stringSliceCmp(listRegistry, rlRepos) == false {
			t.Errorf("repositories do not match: expected %v, received %v", listRegistry, rlRepos)
		}
	})

}
