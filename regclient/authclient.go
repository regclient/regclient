package regclient

import (
	"context"
	"fmt"
	"net/http"
	"os"

	cdauth "github.com/containerd/containerd/remotes/docker"
)

// AuthClient authorizes registry reqests
type AuthClient interface {
	AuthReq(context.Context, *http.Request) error
	AddResp(context.Context, *http.Response) error
	Set(host, user, pass string)
}

type ACOpt func(*authClient)

type authClient struct {
	client     *http.Client
	hosts      map[string]authHost
	authorizer cdauth.Authorizer
}

type authHost struct {
	user, pass string
}

// ACWithDockerCreds adds configuration from users docker config with registry logins
func ACWithDockerCreds() ACOpt {
	return func(ac *authClient) {
		return
	}
}

/* type credStore struct {
	cred map[string]*cred
}

type cred struct {
	user, pass, token string
} */

// NewAuthClient creates an AuthClient to authorize registry requests
func NewAuthClient(opts ...ACOpt) AuthClient {
	var a authClient
	a.client = &http.Client{}
	a.hosts = map[string]authHost{}
	a.authorizer = cdauth.NewDockerAuthorizer(cdauth.WithAuthClient(a.client), cdauth.WithAuthCreds(a.getAuth))
	for _, opt := range opts {
		opt(&a)
	}
	return &a
}

// AuthReq Add auth headers to a request
func (ac *authClient) AuthReq(ctx context.Context, req *http.Request) error {
	return ac.authorizer.Authorize(ctx, req)

	/* 	host := req.URL.Host
	   	// ah, ok := ac.hosts[host]

	   	if !ok {
	   		// new host
	   		ac.detectAuth(ctx, req)
	   	}

	   	if !ok {
	   		// Anonymous request

	   	} else {
	   		// Request with credentials
	   		_ = ah
	   	}

	   	return nil */
}

func (ac *authClient) AddResp(ctx context.Context, resp *http.Response) error {
	resps := []*http.Response{resp}
	return ac.authorizer.AddResponses(ctx, resps)
}

// AuthSet create/update a saved user/pass auth entry
func (ac *authClient) Set(host, user, pass string) {
	if ach, ok := ac.hosts[host]; ok {
		ach.user = user
		ach.pass = pass
	} else {
		ac.hosts[host] = authHost{user: user, pass: pass}
	}

}

func (ac *authClient) getAuth(host string) (string, string, error) {
	if ach, ok := ac.hosts[host]; ok {
		return ach.user, ach.pass, nil
	}
	// default credentials are stored under a blank hostname
	if ach, ok := ac.hosts[""]; ok {
		return ach.user, ach.pass, nil
	}
	fmt.Fprintf(os.Stderr, "No credentials found for %s\n", host)
	// anonymous request
	return "", "", nil
}

/* func (ac *authClient) detectAuth(ctx context.Context, req http.Request) error {
	return nil
} */

/* // Basic provides basic authentication to a given url
func (c credStore) Basic(u *url.URL) (string, string) {
	host := u.Host
	if ch, ok := c.cred[host]; ok {
		return ch.user, ch.pass
	}
	// TODO: log url in miss
	return "", ""
}

func (c credStore) RefreshToken(u *url.URL, service string) string {
	host := u.Host
	if ch, ok := c.cred[host]; ok {
		return ch.token
	}
	return ""
}

func (c credStore) SetRefreshToken(u *url.URL, service string, token string) {
	host := u.Host
	if ch, ok := c.cred[host]; ok {
		ch.token = token
	}
	return
} */
