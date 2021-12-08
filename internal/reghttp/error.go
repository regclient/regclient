package reghttp

import "errors"

var (
	// ErrAPINotFound if an api is not available for the host
	ErrAPINotFound = errors.New("API not found")
	// ErrAllRequestsFailed when there are no mirrors left to try
	ErrAllRequestsFailed = errors.New("All requests failed")
	// ErrBackoffLimit maximum backoff attempts reached
	ErrBackoffLimit = errors.New("Backoff limit reached")
	// ErrCanceled if the context was canceled
	ErrCanceled = errors.New("Context was canceled")
	// ErrDigestMismatch if the expected digest wasn't received
	ErrDigestMismatch = errors.New("Digest mismatch")
	// ErrEmptyChallenge indicates an issue with the received challenge in the WWW-Authenticate header
	ErrEmptyChallenge = errors.New("Empty challenge header")
	// ErrHttpStatus if the http status code was unexpected
	ErrHttpStatus = errors.New("Unexpected http status code")
	// ErrInvalidChallenge indicates an issue with the received challenge in the WWW-Authenticate header
	ErrInvalidChallenge = errors.New("Invalid challenge header")
	// ErrNoNewChallenge indicates a challenge update did not result in any change
	ErrNoNewChallenge = errors.New("No new challenge")
	// ErrNotFound indicates no credentials found for basic auth
	ErrNotFound = errors.New("Not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
	// ErrParseFailure indicates the WWW-Authenticate header could not be parsed
	ErrParseFailure = errors.New("Parse failure")
	// ErrRateLimit when requests exceed server rate limit
	ErrRateLimit = errors.New("Rate limit exceeded")
	// ErrRetryNeeded indicates a request needs to be retried
	ErrRetryNeeded = errors.New("Retry needed")
	// ErrUnauthorized request was not authorized
	ErrUnauthorized = errors.New("Unauthorized")
	// ErrUnsupportedAPI happens when an API is not supported on a registry
	ErrUnsupportedAPI = errors.New("Unsupported API")
	// ErrUnsupported indicates the request was unsupported
	ErrUnsupported = errors.New("Unsupported")
)
