package manifest

import "errors"

var (
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("Not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
	// ErrUnavailable when a requested value is not available
	ErrUnavailable = errors.New("Unavailable")
	// ErrUnsupportedMediaType returned when media type is unknown or unsupported
	ErrUnsupportedMediaType = errors.New("Unsupported media type")
)
