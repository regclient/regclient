package main

import "errors"

var (
	// ErrCredsNotFound returned when creds needed and cannot be found
	ErrCredsNotFound = errors.New("Auth creds not found")
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("Invalid input")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("Required input missing")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("Not found")
	// ErrNotImplemented returned when method has not been implemented yet
	// TODO: Delete when all methods are implemented
	ErrNotImplemented = errors.New("Not implemented")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("Unsupported config version")
)
