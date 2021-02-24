package regclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

// RepoClient provides registry client requests to repositories
type RepoClient interface {
	RepoList(ctx context.Context, hostname string) (RepositoryList, error)
	RepoListWithOpts(ctx context.Context, hostname string, opts RepoOpts) (RepositoryList, error)
}

// RepositoryList comes from github.com/opencontainers/distribution-spec,
// switch to their implementation when it becomes stable
type RepositoryList struct {
	Repositories []string `json:"repositories"`
}

// RepoOpts is used for options to the repo functions
type RepoOpts struct {
	Limit int
	Last  string
}

func (rc *regClient) RepoList(ctx context.Context, hostname string) (RepositoryList, error) {
	return rc.RepoListWithOpts(ctx, hostname, RepoOpts{})
}

func (rc *regClient) RepoListWithOpts(ctx context.Context, hostname string, opts RepoOpts) (RepositoryList, error) {
	rl := RepositoryList{}

	query := url.Values{}
	if opts.Last != "" {
		query.Set("last", opts.Last)
	}
	if opts.Limit > 0 {
		query.Set("n", strconv.Itoa(opts.Limit))
	}

	headers := http.Header{
		"Accept": []string{"application/json"},
	}
	req := httpReq{
		host:      hostname,
		noMirrors: true,
		apis: map[string]httpReqAPI{
			"": {
				method:   "GET",
				path:     "_catalog",
				noPrefix: true,
				query:    query,
				headers:  headers,
			},
		},
	}
	resp, err := rc.httpDo(ctx, req)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		return rl, fmt.Errorf("Failed to list repositories for %s: %w", hostname, err)
	}
	defer resp.Close()
	if resp.HTTPResponse().StatusCode != 200 {
		return rl, fmt.Errorf("Failed to list repositories for %s: %w", hostname, httpError(resp.HTTPResponse().StatusCode))
	}

	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":  err,
			"host": hostname,
		}).Warn("Failed to read repo list")
		return rl, fmt.Errorf("Failed to read repo list for %s: %w", hostname, err)
	}
	err = json.Unmarshal(respBody, &rl)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":  err,
			"body": respBody,
			"host": hostname,
		}).Warn("Failed to unmarshal repo list")
		return rl, fmt.Errorf("Failed to parse repo list for %s: %w", hostname, err)
	}

	return rl, nil
}
