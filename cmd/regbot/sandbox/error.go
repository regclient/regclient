package sandbox

import "errors"

var (
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidWrappedValue indicates the wrapped value did not expand to a table
	ErrInvalidWrappedValue = errors.New("wrapped value must map to a lua table")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("required input missing")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("not implemented")
	// ErrScriptFailed when the script fails to run
	ErrScriptFailed = errors.New("failure in user script")
)
