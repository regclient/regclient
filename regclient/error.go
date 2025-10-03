//go:build legacy
// +build legacy

// Legacy package, this has been moved to the types/error.go package

package regclient

import (
	"github.com/regclient/regclient/types/errs"
)

var (
	// ErrAPINotFound
	//
	// Deprecated: replace with [errs.ErrAPINotFound].
	//go:fix inline
	ErrAPINotFound = errs.ErrAPINotFound
	// ErrCanceled
	//
	// Deprecated: replace with [errs.ErrCanceled].
	//go:fix inline
	ErrCanceled = errs.ErrCanceled
	//lint:ignore ST1003 exported field cannot be changed for legacy reasons
	// ErrHttpStatus
	//
	// Deprecated: replace with [errs.ErrHttpStatus].
	//go:fix inline
	ErrHttpStatus = errs.ErrHTTPStatus
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
