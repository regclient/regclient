package regclient

import (
	"context"
	"net/http"
)

// AuthClient authorizes registry reqests
type AuthClient interface {
	AuthReq(context.Context, http.Request) error
	Set(host, user, pass string)
}

type authClient struct {
	client *http.Client
	hosts  map[string]*authHost
}

type authHost struct {
	user, pass string
}

// NewAuthClient creates an AuthClient to authorize registry requests
func NewAuthClient() AuthClient {
	return &authClient{}
}

// AuthReq Add auth headers to a request
func (ac *authClient) AuthReq(ctx context.Context, req http.Request) error {
	host := req.URL.Host
	ah, ok := ac.hosts[host]

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

	return nil
}

// AuthSet create/update a saved user/pass auth entry
func (ac *authClient) Set(host, user, pass string) {
	if ac.hosts[host] != nil {
		ac.hosts[host].user = user
		ac.hosts[host].pass = pass
	} else {
		ac.hosts[host] = &authHost{user: user, pass: pass}
	}
}

func (ac *authClient) detectAuth(ctx context.Context, req http.Request) error {
	return nil
}
