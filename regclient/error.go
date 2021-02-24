package regclient

import "errors"

var (
	// ErrAPINotFound if an api is not available for the host
	ErrAPINotFound = errors.New("API not found")
	// ErrCanceled if the context was canceled
	ErrCanceled = errors.New("Context was canceled")
	// ErrMissingDigest returned when image reference does not include a digest
	ErrMissingDigest = errors.New("Digest missing from image reference")
	// ErrMissingName returned when name missing for host
	ErrMissingName = errors.New("Name missing")
	// ErrMissingTag returned when image reference does not include a tag
	ErrMissingTag = errors.New("Tag missing from image reference")
	// ErrMissingTagOrDigest returned when image reference does not include a tag or digest
	ErrMissingTagOrDigest = errors.New("Tag or Digest missing from image reference")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("Not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
	// ErrParsingFailed when a string cannot be parsed
	ErrParsingFailed = errors.New("Parsing failed")
	// ErrRateLimit when requests exceed server rate limit
	ErrRateLimit = errors.New("Rate limit exceeded")
	// ErrUnavailable when a requested value is not available
	ErrUnavailable = errors.New("Unavailable")
	// ErrUnauthorized when authentication fails
	ErrUnauthorized = errors.New("Unauthorized")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("Unsupported config version")
	// ErrUnsupportedMediaType returned when media type is unknown or unsupported
	ErrUnsupportedMediaType = errors.New("Unsupported media type")
)
