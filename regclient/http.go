package regclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

type httpReq struct {
	host      string
	noMirrors bool
	apis      map[string]httpReqAPI // allow different types of registries (registry/2.0, OCI, default to empty string)
}

type httpReqAPI struct {
	method     string
	noPrefix   bool
	repository string
	path       string
	query      url.Values
	bodyLen    int64
	bodyBytes  []byte
	bodyFunc   func() (io.ReadCloser, error)
	headers    http.Header
	digest     digest.Digest
}

type httpResp interface {
	retryable.Response
}

// httpDo wraps the http calls from regClient to handle mirrors, potentially
// different API's for different registry types, and then uses retryable to run
// requests
func (rc *regClient) httpDo(ctx context.Context, req httpReq) (httpResp, error) {
	errBody := []byte{}
	upstreamHost := rc.hostGet(req.host)
	hosts := make([]*ConfigHost, 0, 1+len(upstreamHost.Mirrors))
	if !req.noMirrors {
		for _, m := range upstreamHost.Mirrors {
			hosts = append(hosts, rc.hostGet(m))
		}
	}
	hosts = append(hosts, upstreamHost)

	// sort the hosts by highest priority, and with upstream last
	sort.Slice(hosts, sortHostsCmp(rc, hosts, upstreamHost.Name))

	// run a separate retryable per host, allows separate auth, separate API, etc
	err := ErrNotFound
	var resp retryable.Response
	for _, h := range hosts {
		// verify context isn't canceled
		select {
		case <-ctx.Done():
			return nil, ErrCanceled
		default:
		}

		// lookup the api for the host
		api, ok := req.apis[h.API]
		if !ok {
			api, ok = req.apis[""]
		}
		if !ok {
			err = fmt.Errorf("Failed looking up api \"%s\" for host \"%s\": %w", h.API, h.Name, ErrAPINotFound)
			continue
		}

		// build the url
		u := url.URL{
			Host:   h.Hostname,
			Scheme: "https",
		}
		path := strings.Builder{}
		path.WriteString("/v2")
		if h.PathPrefix != "" && !api.noPrefix {
			path.WriteString("/" + h.PathPrefix)
		}
		if api.repository != "" {
			path.WriteString("/" + api.repository)
		}
		path.WriteString("/" + api.path)
		u.Path = path.String()
		if h.TLS == TLSDisabled {
			u.Scheme = "http"
		}
		if api.query != nil {
			u.RawQuery = api.query.Encode()
		}

		opts := []retryable.OptsReq{}
		// add headers
		if api.headers != nil {
			opts = append(opts, retryable.WithHeaders(api.headers))
		}
		// add body
		if api.bodyLen > 0 {
			opts = append(opts, retryable.WithContentLen(api.bodyLen))
		}
		if api.bodyFunc != nil {
			opts = append(opts, retryable.WithBodyFunc(api.bodyFunc))
		} else if len(api.bodyBytes) > 0 {
			opts = append(opts, retryable.WithBodyBytes(api.bodyBytes))
		}
		if api.digest != "" {
			opts = append(opts, retryable.WithDigest(api.digest))
		}
		if api.repository != "" {
			push := false
			if api.method != "HEAD" && api.method != "GET" {
				push = true
			}
			opts = append(opts, retryable.WithScope(api.repository, push))
		}
		// call retryable
		rty := rc.getRetryable(h)
		resp, err = rty.DoRequest(ctx, api.method, []url.URL{u}, opts...)

		// return on success
		if err == nil {
			return resp, nil
		}
		// on failures, log, cache the body, and close the response
		rc.log.WithFields(logrus.Fields{
			"error": err,
			"host":  h.Name,
		}).Debug("HTTP request failed")
		if resp != nil {
			errBody, _ = ioutil.ReadAll(resp)
		}
		if resp != nil {
			resp.Close()
		}
	}
	// out of hosts, return final error
	if err != nil && resp != nil && resp.HTTPResponse() != nil {
		err = httpError(resp.HTTPResponse().StatusCode)
	}
	if err != nil && len(errBody) > 0 {
		err = fmt.Errorf("%w: %s", err, errBody)
	}
	return resp, err
}

// httpError returns an error based on the status code
func httpError(statusCode int) error {
	switch statusCode {
	case 401:
		return fmt.Errorf("%w [http %d]", ErrUnauthorized, statusCode)
	case 403:
		return fmt.Errorf("%w [http %d]", ErrUnauthorized, statusCode)
	case 404:
		return fmt.Errorf("%w [http %d]", ErrNotFound, statusCode)
	case 429:
		return fmt.Errorf("%w [http %d]", ErrRateLimit, statusCode)
	default:
		return fmt.Errorf("%w [http %d]", ErrHttpStatus, statusCode)
	}
}

// sortHostCmp to sort host list of mirrors
func sortHostsCmp(rc *regClient, hosts []*ConfigHost, upstream string) func(i, j int) bool {
	// build map of host name to retryable DownUntil times
	backoffUntil := map[string]time.Time{}
	for _, h := range hosts {
		backoffUntil[h.Name] = rc.getRetryable(h).BackoffUntil()
	}
	// sort by DownUntil first, then priority decending, then upstream name last
	return func(i, j int) bool {
		if time.Now().Before(backoffUntil[hosts[i].Name]) || time.Now().Before(backoffUntil[hosts[j].Name]) {
			return backoffUntil[hosts[i].Name].Before(backoffUntil[hosts[j].Name])
		}
		if hosts[i].Priority != hosts[j].Priority {
			return hosts[i].Priority > hosts[j].Priority
		}
		return hosts[i].Name != upstream
	}
}
