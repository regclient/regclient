package main

import "errors"

var (
	// ErrCredsNotFound returned when creds needed and cannot be found
	ErrCredsNotFound = errors.New("auth creds not found")
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("invalid input")
	// ErrLoopEncountered indicates a loop was encountered when walking the artifact tree
	ErrLoopEncountered = errors.New("loop encountered")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("required input missing")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("not found")
	// ErrNotImplemented returned when method has not been implemented yet
	// TODO: Delete when all methods are implemented
	ErrNotImplemented = errors.New("not implemented")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("unsupported config version")
)
