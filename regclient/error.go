package regclient

import "errors"

var (
	// ErrMissingTag returned when image reference does not include a tag or digest
	ErrMissingTag = errors.New("Tag or Digest missing from image reference")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("Not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
)
