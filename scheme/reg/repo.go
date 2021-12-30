package reg

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/repo"
	"github.com/sirupsen/logrus"
)

func (reg *Reg) RepoList(ctx context.Context, hostname string, opts ...scheme.RepoOpts) (*repo.RepoList, error) {
	config := scheme.RepoConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	query := url.Values{}
	if config.Last != "" {
		query.Set("last", config.Last)
	}
	if config.Limit > 0 {
		query.Set("n", strconv.Itoa(config.Limit))
	}

	headers := http.Header{
		"Accept": []string{"application/json"},
	}
	req := &reghttp.Req{
		Host:      hostname,
		NoMirrors: true,
		APIs: map[string]reghttp.ReqAPI{
			"": {
				Method:   "GET",
				Path:     "_catalog",
				NoPrefix: true,
				Query:    query,
				Headers:  headers,
			},
		},
	}
	resp, err := reg.reghttp.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Failed to list repositories for %s: %w", hostname, err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return nil, fmt.Errorf("Failed to list repositories for %s: %w", hostname, reghttp.HttpError(resp.HTTPResponse().StatusCode))
	}

	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		reg.log.WithFields(logrus.Fields{
			"err":  err,
			"host": hostname,
		}).Warn("Failed to read repo list")
		return nil, fmt.Errorf("Failed to read repo list for %s: %w", hostname, err)
	}
	mt := resp.HTTPResponse().Header.Get("Content-Type")
	rl, err := repo.New(
		repo.WithMT(mt),
		repo.WithRaw(respBody),
		repo.WithHost(hostname),
		repo.WithHeaders(resp.HTTPResponse().Header),
	)
	if err != nil {
		reg.log.WithFields(logrus.Fields{
			"err":  err,
			"body": respBody,
			"host": hostname,
		}).Warn("Failed to unmarshal repo list")
		return nil, fmt.Errorf("Failed to parse repo list for %s: %w", hostname, err)
	}
	return rl, nil
}
