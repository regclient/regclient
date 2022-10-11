//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to top level types package

package types

import (
	topTypes "github.com/regclient/regclient/types"
)

var (
	ErrAllRequestsFailed = topTypes.ErrAllRequestsFailed
	ErrAPINotFound       = topTypes.ErrAPINotFound
	ErrBackoffLimit      = topTypes.ErrBackoffLimit
	ErrCanceled          = topTypes.ErrCanceled
	ErrDigestMismatch    = topTypes.ErrDigestMismatch
	ErrEmptyChallenge    = topTypes.ErrEmptyChallenge
	//lint:ignore ST1003 exported field cannot be changed for legacy reasons
	ErrHttpStatus               = topTypes.ErrHTTPStatus
	ErrInvalidChallenge         = topTypes.ErrInvalidChallenge
	ErrMissingDigest            = topTypes.ErrMissingDigest
	ErrMissingLocation          = topTypes.ErrMissingLocation
	ErrMissingName              = topTypes.ErrMissingName
	ErrMissingTag               = topTypes.ErrMissingTag
	ErrMissingTagOrDigest       = topTypes.ErrMissingTagOrDigest
	ErrMountReturnedLocation    = topTypes.ErrMountReturnedLocation
	ErrNoNewChallenge           = topTypes.ErrNoNewChallenge
	ErrNotFound                 = topTypes.ErrNotFound
	ErrNotImplemented           = topTypes.ErrNotImplemented
	ErrParsingFailed            = topTypes.ErrParsingFailed
	ErrRateLimit                = topTypes.ErrHTTPRateLimit
	ErrRetryNeeded              = topTypes.ErrRetryNeeded
	ErrUnavailable              = topTypes.ErrUnavailable
	ErrUnauthorized             = topTypes.ErrHTTPUnauthorized
	ErrUnsupported              = topTypes.ErrUnsupported
	ErrUnsupportedAPI           = topTypes.ErrUnsupportedAPI
	ErrUnsupportedConfigVersion = topTypes.ErrUnsupportedConfigVersion
	ErrUnsupportedMediaType     = topTypes.ErrUnsupportedMediaType
)
