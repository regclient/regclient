package regclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/docker/distribution/reference"
)

var (
	ErrNotImplemented = errors.New("Not implemented")
)

type tlsConf int

const (
	tlsEnabled tlsConf = iota
	tlsInsecure
	tlsDisabled
)

// RegClient provides an interfaces to working with registries
type RegClient interface {
	Auth() AuthClient
	RepoList(ctx context.Context, ref Ref) error
}

// Ref reference to a registry/repository
// If the tag or digest is available, it's also included in the reference.
// Reference itself is the unparsed string.
// While this is currently a struct, that may change in the future and access
// to contents should not be assumed/used.
type Ref struct {
	Reference, Registry, Repository, Tag, Digest string
}

type regClient struct {
	hosts map[string]*regHost
	auth  AuthClient
}

type regHost struct {
	scheme string
	tls    tlsConf
}

// Opt functions are used to configure NewRegClient
type Opt func(*regClient)

// NewRegClient returns a registry client
func NewRegClient(opts ...Opt) RegClient {
	var rc regClient

	rc.auth = NewAuthClient()

	for _, opt := range opts {
		opt(&rc)
	}

	return &rc
}

// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d
func WithDockerCerts() Opt {
	return func(rc *regClient) {
		return
	}
}

// WithDockerCreds adds configuration from users docker config with registry logins
func WithDockerCreds() Opt {
	return func(rc *regClient) {
		return
	}
}

// WithRegClientConf adds configuration from regclient
func WithRegClientConf() Opt {
	return func(rc *regClient) {
		return
	}
}

// NewRef returns a repository reference including a registry, repository (path), digest, and tag
func NewRef(ref string) (Ref, error) {
	parsed, err := reference.ParseNormalizedNamed(ref)

	var ret Ref
	ret.Reference = ref

	if err != nil {
		return ret, err
	}

	ret.Registry = reference.Domain(parsed)
	ret.Repository = reference.Path(parsed)

	if canonical, ok := parsed.(reference.Canonical); ok {
		ret.Digest = canonical.Digest().String()
	}

	if tagged, ok := parsed.(reference.Tagged); ok {
		ret.Tag = tagged.Tag()
	}

	return ret, nil
}

func (rc *regClient) Auth() AuthClient {
	return rc.auth
}

func (rc *regClient) RepoList(ctx context.Context, ref Ref) error {
	repoURL := url.URL{
		Scheme: "https",
		Host:   ref.Registry,
		Path:   "/v2/" + ref.Repository,
	}

	req, err := http.NewRequest("GET", repoURL.String(), nil)
	if err != nil {
		return err
	}
	rty := NewRetryable(RetryWithLimit(3))
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return err
	}
	fmt.Printf("Response Code: %d\n", resp.StatusCode)
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Response Body: \n%s", respBody)
	return nil
}

// Retryable retries a request until it succeeds or reaches a max number of failures
// This is also used to inject authorization into a request
type Retryable interface {
	Req(context.Context, RegClient, *http.Request) (*http.Response, error)
}

type retryable struct {
	transport *http.Transport
	req       *http.Request
	resps     []*http.Response
	limit     int
}

// ROpt is used to pass options to NewRetryable
type ROpt func(*retryable)

// NewRetryable returns a Retryable used to retry http requests
func NewRetryable(opts ...ROpt) Retryable {
	r := retryable{
		transport: http.DefaultTransport.(*http.Transport),
		limit:     5,
	}

	for _, opt := range opts {
		opt(&r)
	}

	return &r
}

// RetryWithTransport adds a user provided transport to NewRetryable
func RetryWithTransport(t *http.Transport) ROpt {
	return func(r *retryable) {
		r.transport = t
		return
	}
}

// RetryWithTLSInsecure allows https with invalid certificate
func RetryWithTLSInsecure() ROpt {
	return func(r *retryable) {
		if r.transport.TLSClientConfig == nil {
			r.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		} else {
			r.transport.TLSClientConfig.InsecureSkipVerify = true
		}
		return
	}
}

// RetryWithLimit allows adjusting the retry limit
func RetryWithLimit(limit int) ROpt {
	return func(r *retryable) {
		r.limit = limit
		return
	}
}

func (r *retryable) Req(ctx context.Context, rc RegClient, req *http.Request) (*http.Response, error) {
	return nil, ErrNotImplemented
}
