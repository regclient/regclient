package regclient

import "errors"

var (
	// ErrMissingTag returned when image reference does not include a tag or digest
	ErrMissingTag = errors.New("Tag or Digest missing from image reference")
	// ErrNotImplemented returned when method has not been implemented yet
	// TODO: Delete when all methods are implemented
	ErrNotImplemented = errors.New("Not implemented")
)
