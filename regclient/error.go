package regclient

import "errors"

var (
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("Unsupported config version")
	// ErrMissingDigest returned when image reference does not include a digest
	ErrMissingDigest = errors.New("Digest missing from image reference")
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
	// ErrUnsupportedMediaType returned when media type is unknown or unsupported
	ErrUnsupportedMediaType = errors.New("Unsupported media type")
)
