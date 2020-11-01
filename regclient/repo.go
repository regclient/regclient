package regclient

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/url"
	"strconv"

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
	if err != nil {
		return rl, err
	}
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Failed to read repo list")
		return rl, err
	}
	err = json.Unmarshal(respBody, &rl)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":  err,
			"body": respBody,
		}).Warn("Failed to unmarshal repo list")
		return rl, err
	}

	return rl, nil
}
