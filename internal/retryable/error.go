//go:build legacy
// +build legacy

package retryable

import "errors"

var (
	// ErrAllRequestsFailed when there are no mirrors left to try
	ErrAllRequestsFailed = errors.New("all requests failed")
	// ErrBackoffLimit maximum backoff attempts reached
	ErrBackoffLimit = errors.New("backoff limit reached")
	// ErrCanceled if the context was canceled
	ErrCanceled = errors.New("context was canceled")
	// ErrDigestMismatch if the expected digest wasn't received
	ErrDigestMismatch = errors.New("digest mismatch")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("not implemented")
	// ErrRetryNeeded indicates a request needs to be retried
	ErrRetryNeeded = errors.New("retry needed")
	// ErrUnauthorized request was not authorized
	ErrUnauthorized = errors.New("unauthorized")
)
