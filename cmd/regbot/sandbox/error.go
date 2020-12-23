package sandbox

import "errors"

var (
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("Invalid input")
	// ErrInvalidWrappedValue indicates the wrapped value did not expand to a table
	ErrInvalidWrappedValue = errors.New("Wrapped value must map to a lua table")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("Required input missing")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
	// ErrScriptFailed when the script fails to run
	ErrScriptFailed = errors.New("Failure in user script")
)
