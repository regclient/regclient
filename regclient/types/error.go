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
	//go:fix inline
	ErrAllRequestsFailed = errs.ErrAllRequestsFailed
	// ErrAPINotFound
	//
	// Deprecated: replace with [errs.ErrAPINotFound].
	//go:fix inline
	ErrAPINotFound = errs.ErrAPINotFound
	// ErrBackoffLimit
	//
	// Deprecated: replace with [errs.ErrBackoffLimit].
	//go:fix inline
	ErrBackoffLimit = errs.ErrBackoffLimit
	// ErrCanceled
	//
	// Deprecated: replace with [errs.ErrCanceled].
	//go:fix inline
	ErrCanceled = errs.ErrCanceled
	// ErrDigestMismatch
	//
	// Deprecated: replace with [errs.ErrDigestMismatch].
	//go:fix inline
	ErrDigestMismatch = errs.ErrDigestMismatch
	// ErrEmptyChallenge
	//
	// Deprecated: replace with [errs.ErrEmptyChallenge].
	//go:fix inline
	ErrEmptyChallenge = errs.ErrEmptyChallenge
	//lint:ignore ST1003 exported field cannot be changed for legacy reasons
	// ErrHttpStatus
	//
	// Deprecated: replace with [errs.ErrHttpStatus].
	//go:fix inline
	ErrHttpStatus = errs.ErrHTTPStatus
	// ErrInvalidChallenge
	//
	// Deprecated: replace with [errs.ErrInvalidChallenge].
	//go:fix inline
	ErrInvalidChallenge = errs.ErrInvalidChallenge
	// ErrMissingDigest
	//
	// Deprecated: replace with [errs.ErrMissingDigest].
	//go:fix inline
	ErrMissingDigest = errs.ErrMissingDigest
	// ErrMissingLocation
	//
	// Deprecated: replace with [errs.ErrMissingLocation].
	//go:fix inline
	ErrMissingLocation = errs.ErrMissingLocation
	// ErrMissingName
	//
	// Deprecated: replace with [errs.ErrMissingName].
	//go:fix inline
	ErrMissingName = errs.ErrMissingName
	// ErrMissingTag
	//
	// Deprecated: replace with [errs.ErrMissingTag].
	//go:fix inline
	ErrMissingTag = errs.ErrMissingTag
	// ErrMissingTagOrDigest
	//
	// Deprecated: replace with [errs.ErrMissingTagOrDigest].
	//go:fix inline
	ErrMissingTagOrDigest = errs.ErrMissingTagOrDigest
	// ErrMountReturnedLocation
	//
	// Deprecated: replace with [errs.ErrMountReturnedLocation].
	//go:fix inline
	ErrMountReturnedLocation = errs.ErrMountReturnedLocation
	// ErrNoNewChallenge
	//
	// Deprecated: replace with [errs.ErrNoNewChallenge].
	//go:fix inline
	ErrNoNewChallenge = errs.ErrNoNewChallenge
	// ErrNotFound
	//
	// Deprecated: replace with [errs.ErrNotFound].
	//go:fix inline
	ErrNotFound = errs.ErrNotFound
	// ErrNotImplemented
	//
	// Deprecated: replace with [errs.ErrNotImplemented].
	//go:fix inline
	ErrNotImplemented = errs.ErrNotImplemented
	// ErrParsingFailed
	//
	// Deprecated: replace with [errs.ErrParsingFailed].
	//go:fix inline
	ErrParsingFailed = errs.ErrParsingFailed
	// ErrRateLimit
	//
	// Deprecated: replace with [errs.ErrRateLimit].
	//go:fix inline
	ErrRateLimit = errs.ErrHTTPRateLimit
	// ErrRetryNeeded
	//
	// Deprecated: replace with [errs.ErrRetryNeeded].
	//go:fix inline
	ErrRetryNeeded = errs.ErrRetryNeeded
	// ErrUnavailable
	//
	// Deprecated: replace with [errs.ErrUnavailable].
	//go:fix inline
	ErrUnavailable = errs.ErrUnavailable
	// ErrUnauthorized
	//
	// Deprecated: replace with [errs.ErrUnauthorized].
	//go:fix inline
	ErrUnauthorized = errs.ErrHTTPUnauthorized
	// ErrUnsupported
	//
	// Deprecated: replace with [errs.ErrUnsupported].
	//go:fix inline
	ErrUnsupported = errs.ErrUnsupported
	// ErrUnsupportedAPI
	//
	// Deprecated: replace with [errs.ErrUnsupportedAPI].
	//go:fix inline
	ErrUnsupportedAPI = errs.ErrUnsupportedAPI
	// ErrUnsupportedConfigVersion
	//
	// Deprecated: replace with [errs.ErrUnsupportedConfigVersion].
	//go:fix inline
	ErrUnsupportedConfigVersion = errs.ErrUnsupportedConfigVersion
	// ErrUnsupportedMediaType
	//
	// Deprecated: replace with [errs.ErrUnsupportedMediaType].
	//go:fix inline
	ErrUnsupportedMediaType = errs.ErrUnsupportedMediaType
)
