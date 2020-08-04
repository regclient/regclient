package regclient

import (
	"context"
	"fmt"
	"net/http"
)

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
/* func RetryWithTLSInsecure() ROpt {
	return func(r *retryable) {
		if r.transport.TLSClientConfig == nil {
			r.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		} else {
			r.transport.TLSClientConfig.InsecureSkipVerify = true
		}
		return
	}
} */

// RetryWithLimit allows adjusting the retry limit
func RetryWithLimit(limit int) ROpt {
	return func(r *retryable) {
		r.limit = limit
		return
	}
}

func (r *retryable) Req(ctx context.Context, rc RegClient, req *http.Request) (*http.Response, error) {
	// define return values outside of the loop scope
	var resp *http.Response
	var err error
	resps := []*http.Response{}

	client := &http.Client{
		Transport: r.transport,
	}

	for i := 0; i < r.limit; i++ {
		if i > 1 {
			fmt.Printf("Retryable request attempt %d\n", i)
		}

		// add auth
		err = rc.Auth().AuthReq(ctx, req)
		if err != nil {
			return resp, err
		}

		// send request
		resp, err = client.Do(req)
		if err != nil {
			return resp, err
		}
		resps = append(resps, resp)

		switch resp.StatusCode {
		case http.StatusUnauthorized:
			// fmt.Printf("Unauthorized, adding auth\n")
			// update auth based on response
			err = rc.Auth().AddResp(ctx, resps)
			if err != nil {
				return resp, err
			}

		case http.StatusRequestTimeout, http.StatusTooManyRequests:
			// allow retry

		default:
			// on all other success and failure cases do not retry
			return resp, err
		}
		// reset body if GetBody has been set
		if req.GetBody != nil {
			req.Body, _ = req.GetBody()
		}
	}
	return resp, err
}
