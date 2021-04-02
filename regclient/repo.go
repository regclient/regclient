package regclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

// RepoClient provides registry client requests to repositories
type RepoClient interface {
	RepoList(ctx context.Context, hostname string) (RepoList, error)
	RepoListWithOpts(ctx context.Context, hostname string, opts RepoOpts) (RepoList, error)
}

// RepoList interface is used for listing tags
type RepoList interface {
	GetOrig() interface{}
	MarshalJSON() ([]byte, error)
	RawBody() ([]byte, error)
	RawHeaders() (http.Header, error)
	GetRepos() ([]string, error)
}

type repoCommon struct {
	host      string
	mt        string
	orig      interface{}
	rawHeader http.Header
	rawBody   []byte
}

type repoDockerList struct {
	repoCommon
	RepoDockerList
}

// RepoDockerList is a list of repositories from the _catalog API
type RepoDockerList struct {
	Repositories []string `json:"repositories"`
}

// RepoOpts is used for options to the repo functions
type RepoOpts struct {
	Limit int
	Last  string
}

func (rc *regClient) RepoList(ctx context.Context, hostname string) (RepoList, error) {
	return rc.RepoListWithOpts(ctx, hostname, RepoOpts{})
}

func (rc *regClient) RepoListWithOpts(ctx context.Context, hostname string, opts RepoOpts) (RepoList, error) {
	var rl RepoList
	rlc := repoCommon{
		host: hostname,
	}

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

	rlc.rawHeader = resp.HTTPResponse().Header
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err":  err,
			"host": hostname,
		}).Warn("Failed to read repo list")
		return rl, fmt.Errorf("Failed to read repo list for %s: %w", hostname, err)
	}
	rlc.rawBody = respBody
	rlc.mt = resp.HTTPResponse().Header.Get("Content-Type")
	mt := strings.Split(rlc.mt, ";")[0] // "application/json; charset=utf-8" -> "application/json"
	switch mt {
	case "application/json", "text/plain":
		var rdl RepoDockerList
		err = json.Unmarshal(respBody, &rdl)
		rlc.orig = rdl
		rl = repoDockerList{
			repoCommon:     rlc,
			RepoDockerList: rdl,
		}
	default:
		return rl, fmt.Errorf("%w: media type: %s, hostname: %s", ErrUnsupportedMediaType, mt, hostname)
	}
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

func (r repoCommon) GetOrig() interface{} {
	return r.orig
}

func (r repoCommon) MarshalJSON() ([]byte, error) {
	if len(r.rawBody) > 0 {
		return r.rawBody, nil
	}

	if r.orig != nil {
		return json.Marshal((r.orig))
	}
	return []byte{}, fmt.Errorf("Json marshalling failed: %w", ErrNotFound)
}

func (r repoCommon) RawBody() ([]byte, error) {
	return r.rawBody, nil
}

func (r repoCommon) RawHeaders() (http.Header, error) {
	return r.rawHeader, nil
}

// GetRepos returns the repositories
func (rl RepoDockerList) GetRepos() ([]string, error) {
	return rl.Repositories, nil
}

// MarshalPretty is used for printPretty template formatting
func (rl RepoDockerList) MarshalPretty() ([]byte, error) {
	sort.Slice(rl.Repositories, func(i, j int) bool {
		if strings.Compare(rl.Repositories[i], rl.Repositories[j]) < 0 {
			return true
		}
		return false
	})
	buf := &bytes.Buffer{}
	for _, tag := range rl.Repositories {
		fmt.Fprintf(buf, "%s\n", tag)
	}
	return buf.Bytes(), nil
}
