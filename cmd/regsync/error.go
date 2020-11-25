package main

import "errors"

var (
	// ErrCanceled is used when context is canceled before task completes
	ErrCanceled = errors.New("Task was canceled")
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("Invalid input")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("Required input missing")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("Not implemented")
	// ErrNotFound when anything else isn't found
	ErrNotFound = errors.New("Not found")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("Unsupported config version")
)
