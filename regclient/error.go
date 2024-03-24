//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the types/error.go package

package regclient

import (
	"github.com/regclient/regclient/types/errs"
)

var (
	// ErrAPINotFound
	//
	// Deprecated: replace with [errs.ErrAPINotFound].
	ErrAPINotFound = errs.ErrAPINotFound
	// ErrCanceled
	//
	// Deprecated: replace with [errs.ErrCanceled].
	ErrCanceled = errs.ErrCanceled
	//lint:ignore ST1003 exported field cannot be changed for legacy reasons
	// ErrHttpStatus
	//
	// Deprecated: replace with [errs.ErrHttpStatus].
	ErrHttpStatus = errs.ErrHTTPStatus
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
	// ErrUnavailable
	//
	// Deprecated: replace with [errs.ErrUnavailable].
	ErrUnavailable = errs.ErrUnavailable
	// ErrUnauthorized
	//
	// Deprecated: replace with [errs.ErrUnauthorized].
	ErrUnauthorized = errs.ErrHTTPUnauthorized
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
