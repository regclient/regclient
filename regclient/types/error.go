//go:build legacy
// +build legacy

// Legacy package, this has been moved to top level types package

package types

import (
	"github.com/regclient/regclient/types/errs"
)

var (
	// ErrAllRequestsFailed
	//
	// Deprecated: replace with [errs.ErrAllRequestsFailed].
	ErrAllRequestsFailed = errs.ErrAllRequestsFailed
	// ErrAPINotFound
	//
	// Deprecated: replace with [errs.ErrAPINotFound].
	ErrAPINotFound = errs.ErrAPINotFound
	// ErrBackoffLimit
	//
	// Deprecated: replace with [errs.ErrBackoffLimit].
	ErrBackoffLimit = errs.ErrBackoffLimit
	// ErrCanceled
	//
	// Deprecated: replace with [errs.ErrCanceled].
	ErrCanceled = errs.ErrCanceled
	// ErrDigestMismatch
	//
	// Deprecated: replace with [errs.ErrDigestMismatch].
	ErrDigestMismatch = errs.ErrDigestMismatch
	// ErrEmptyChallenge
	//
	// Deprecated: replace with [errs.ErrEmptyChallenge].
	ErrEmptyChallenge = errs.ErrEmptyChallenge
	//lint:ignore ST1003 exported field cannot be changed for legacy reasons
	// ErrHttpStatus
	//
	// Deprecated: replace with [errs.ErrHttpStatus].
	ErrHttpStatus = errs.ErrHTTPStatus
	// ErrInvalidChallenge
	//
	// Deprecated: replace with [errs.ErrInvalidChallenge].
	ErrInvalidChallenge = errs.ErrInvalidChallenge
	// ErrMissingDigest
	//
	// Deprecated: replace with [errs.ErrMissingDigest].
	ErrMissingDigest = errs.ErrMissingDigest
	// ErrMissingLocation
	//
	// Deprecated: replace with [errs.ErrMissingLocation].
	ErrMissingLocation = errs.ErrMissingLocation
	// ErrMissingName
	//
	// Deprecated: replace with [errs.ErrMissingName].
	ErrMissingName = errs.ErrMissingName
	// ErrMissingTag
	//
	// Deprecated: replace with [errs.ErrMissingTag].
	ErrMissingTag = errs.ErrMissingTag
	// ErrMissingTagOrDigest
	//
	// Deprecated: replace with [errs.ErrMissingTagOrDigest].
	ErrMissingTagOrDigest = errs.ErrMissingTagOrDigest
	// ErrMountReturnedLocation
	//
	// Deprecated: replace with [errs.ErrMountReturnedLocation].
	ErrMountReturnedLocation = errs.ErrMountReturnedLocation
	// ErrNoNewChallenge
	//
	// Deprecated: replace with [errs.ErrNoNewChallenge].
	ErrNoNewChallenge = errs.ErrNoNewChallenge
	// ErrNotFound
	//
	// Deprecated: replace with [errs.ErrNotFound].
	ErrNotFound = errs.ErrNotFound
	// ErrNotImplemented
	//
	// Deprecated: replace with [errs.ErrNotImplemented].
	ErrNotImplemented = errs.ErrNotImplemented
	// ErrParsingFailed
	//
	// Deprecated: replace with [errs.ErrParsingFailed].
	ErrParsingFailed = errs.ErrParsingFailed
	// ErrRateLimit
	//
	// Deprecated: replace with [errs.ErrRateLimit].
	ErrRateLimit = errs.ErrHTTPRateLimit
	// ErrRetryNeeded
	//
	// Deprecated: replace with [errs.ErrRetryNeeded].
	ErrRetryNeeded = errs.ErrRetryNeeded
	// ErrUnavailable
	//
	// Deprecated: replace with [errs.ErrUnavailable].
	ErrUnavailable = errs.ErrUnavailable
	// ErrUnauthorized
	//
	// Deprecated: replace with [errs.ErrUnauthorized].
	ErrUnauthorized = errs.ErrHTTPUnauthorized
	// ErrUnsupported
	//
	// Deprecated: replace with [errs.ErrUnsupported].
	ErrUnsupported = errs.ErrUnsupported
	// ErrUnsupportedAPI
	//
	// Deprecated: replace with [errs.ErrUnsupportedAPI].
	ErrUnsupportedAPI = errs.ErrUnsupportedAPI
	// ErrUnsupportedConfigVersion
	//
	// Deprecated: replace with [errs.ErrUnsupportedConfigVersion].
	ErrUnsupportedConfigVersion = errs.ErrUnsupportedConfigVersion
	// ErrUnsupportedMediaType
	//
	// Deprecated: replace with [errs.ErrUnsupportedMediaType].
	ErrUnsupportedMediaType = errs.ErrUnsupportedMediaType
)
