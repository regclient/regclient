package main

import "errors"

var (
	// ErrCanceled is used when context is canceled before task completes
	ErrCanceled = errors.New("task was canceled")
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("invalid input")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("required input missing")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("not implemented")
	// ErrNotFound when anything else isn't found
	ErrNotFound = errors.New("not found")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("unsupported config version")
)
