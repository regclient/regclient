package types

import "errors"

var (
	// ErrAllRequestsFailed when there are no mirrors left to try
	ErrAllRequestsFailed = errors.New("all requests failed")
	// ErrAPINotFound if an api is not available for the host
	ErrAPINotFound = errors.New("API not found")
	// ErrBackoffLimit maximum backoff attempts reached
	ErrBackoffLimit = errors.New("backoff limit reached")
	// ErrCanceled if the context was canceled
	ErrCanceled = errors.New("context was canceled")
	// ErrDigestMismatch if the expected digest wasn't received
	ErrDigestMismatch = errors.New("digest mismatch")
	// ErrEmptyChallenge indicates an issue with the received challenge in the WWW-Authenticate header
	ErrEmptyChallenge = errors.New("empty challenge header")
	// ErrHTTPStatus if the http status code was unexpected
	ErrHTTPStatus = errors.New("unexpected http status code")
	// ErrInvalidChallenge indicates an issue with the received challenge in the WWW-Authenticate header
	ErrInvalidChallenge = errors.New("invalid challenge header")
	// ErrMissingAnnotation returned when a needed annotation is not found
	ErrMissingAnnotation = errors.New("annotation is missing")
	// ErrMissingDigest returned when image reference does not include a digest
	ErrMissingDigest = errors.New("digest missing from image reference")
	// ErrMissingLocation returned when the location header is missing
	ErrMissingLocation = errors.New("location header missing")
	// ErrMissingName returned when name missing for host
	ErrMissingName = errors.New("name missing")
	// ErrMissingTag returned when image reference does not include a tag
	ErrMissingTag = errors.New("tag missing from image reference")
	// ErrMissingTagOrDigest returned when image reference does not include a tag or digest
	ErrMissingTagOrDigest = errors.New("tag or Digest missing from image reference")
	// ErrMismatch returned when a comparison detects a difference
	ErrMismatch = errors.New("content does not match")
	// ErrMountReturnedLocation when a blob mount fails but a location header is received
	ErrMountReturnedLocation = errors.New("blob mount returned a location to upload")
	// ErrNoNewChallenge indicates a challenge update did not result in any change
	ErrNoNewChallenge = errors.New("no new challenge")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("not implemented")
	// ErrParsingFailed when a string cannot be parsed
	ErrParsingFailed = errors.New("parsing failed")
	// ErrRateLimit when requests exceed server rate limit
	ErrRateLimit = errors.New("rate limit exceeded")
	// ErrRetryNeeded indicates a request needs to be retried
	ErrRetryNeeded = errors.New("retry needed")
	// ErrUnavailable when a requested value is not available
	ErrUnavailable = errors.New("unavailable")
	// ErrUnauthorized when authentication fails
	ErrUnauthorized = errors.New("unauthorized")
	// ErrUnsupported indicates the request was unsupported
	ErrUnsupported = errors.New("unsupported")
	// ErrUnsupportedAPI happens when an API is not supported on a registry
	ErrUnsupportedAPI = errors.New("unsupported API")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("unsupported config version")
	// ErrUnsupportedMediaType returned when media type is unknown or unsupported
	ErrUnsupportedMediaType = errors.New("unsupported media type")
)
