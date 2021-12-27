package types

import "errors"

var (
	// ErrAllRequestsFailed when there are no mirrors left to try
	ErrAllRequestsFailed = errors.New("All requests failed")
	// ErrAPINotFound if an api is not available for the host
	ErrAPINotFound = errors.New("API not found")
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
	// ErrMissingDigest returned when image reference does not include a digest
	ErrMissingDigest = errors.New("Digest missing from image reference")
	// ErrMissingLocation returned when the location header is missing
	ErrMissingLocation = errors.New("Location header missing")
	// ErrMissingName returned when name missing for host
	ErrMissingName = errors.New("Name missing")
	// ErrMissingTag returned when image reference does not include a tag
	ErrMissingTag = errors.New("Tag missing from image reference")
	// ErrMissingTagOrDigest returned when image reference does not include a tag or digest
	ErrMissingTagOrDigest = errors.New("Tag or Digest missing from image reference")
	// ErrMountReturnedLocation when a blob mount fails but a location header is received
	ErrMountReturnedLocation = errors.New("Blob mount returned a location to upload")
	// ErrNoNewChallenge indicates a challenge update did not result in any change
	ErrNoNewChallenge = errors.New("No new challenge")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("Not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
	// ErrParsingFailed when a string cannot be parsed
	ErrParsingFailed = errors.New("Parsing failed")
	// ErrRateLimit when requests exceed server rate limit
	ErrRateLimit = errors.New("Rate limit exceeded")
	// ErrRetryNeeded indicates a request needs to be retried
	ErrRetryNeeded = errors.New("Retry needed")
	// ErrUnavailable when a requested value is not available
	ErrUnavailable = errors.New("Unavailable")
	// ErrUnauthorized when authentication fails
	ErrUnauthorized = errors.New("Unauthorized")
	// ErrUnsupported indicates the request was unsupported
	ErrUnsupported = errors.New("Unsupported")
	// ErrUnsupportedAPI happens when an API is not supported on a registry
	ErrUnsupportedAPI = errors.New("Unsupported API")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("Unsupported config version")
	// ErrUnsupportedMediaType returned when media type is unknown or unsupported
	ErrUnsupportedMediaType = errors.New("Unsupported media type")
)
