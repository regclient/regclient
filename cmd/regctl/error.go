package main

import "errors"

var (
	// ErrCredsNotFound returned when creds needed and cannot be found
	ErrCredsNotFound = errors.New("Auth creds not found")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("Required input missing")
	// ErrNotImplemented returned when method has not been implemented yet
	// TODO: Delete when all methods are implemented
	ErrNotImplemented = errors.New("Not implemented")
	// ErrNotFound when anything else isn't found
	ErrNotFound = errors.New("Not found")
)
