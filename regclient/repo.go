package regclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"

	"github.com/regclient/regclient/pkg/retryable"
	"github.com/regclient/regclient/pkg/wraperr"
	"github.com/sirupsen/logrus"
)

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
	host := rc.getHost(hostname)
	repoURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/_catalog",
	}
	query := url.Values{}
	if opts.Last != "" {
		query.Set("last", opts.Last)
	}
	if opts.Limit > 0 {
		query.Set("n", strconv.Itoa(opts.Limit))
	}
	repoURL.RawQuery = query.Encode()

	rty := rc.getRetryable(host)
	resp, err := rty.DoRequest(ctx, "GET", repoURL)
	if err != nil && !errors.Is(err, retryable.ErrStatusCode) {
		rc.log.WithFields(logrus.Fields{
			"err":  err,
			"host": hostname,
		}).Warn("Failed to request repo list")
		return rl, fmt.Errorf("Failed to request repo list for %s: %w", hostname, err)
	}
	defer resp.Close()
	switch resp.HTTPResponse().StatusCode {
	case 200: // success
	case 401:
		return rl, wraperr.New(fmt.Errorf("Unauthorized request for repo list %s", hostname), ErrUnauthorized)
	case 403:
		return rl, wraperr.New(fmt.Errorf("Forbidden request for repo list %s", hostname), ErrUnauthorized)
	case 404:
		return rl, wraperr.New(fmt.Errorf("Repo list not found: %s", hostname), ErrNotFound)
	case 429:
		return rl, wraperr.New(fmt.Errorf("Rate limit exceeded for repo list %s", hostname), ErrRateLimit)
	default:
		return rl, fmt.Errorf("Error getting repo list for %s: http response code %d != 200", hostname, resp.HTTPResponse().StatusCode)
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
