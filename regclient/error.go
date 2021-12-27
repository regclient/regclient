package regclient

import (
	"github.com/regclient/regclient/regclient/types"
)

// Errors have been moved to types/error.go
// this file is retained for backwards compatibility

var (
	ErrAPINotFound              = types.ErrAPINotFound
	ErrCanceled                 = types.ErrCanceled
	ErrHttpStatus               = types.ErrHttpStatus
	ErrMissingDigest            = types.ErrMissingDigest
	ErrMissingLocation          = types.ErrMissingLocation
	ErrMissingName              = types.ErrMissingName
	ErrMissingTag               = types.ErrMissingTag
	ErrMissingTagOrDigest       = types.ErrMissingTagOrDigest
	ErrMountReturnedLocation    = types.ErrMountReturnedLocation
	ErrNotFound                 = types.ErrNotFound
	ErrNotImplemented           = types.ErrNotImplemented
	ErrParsingFailed            = types.ErrParsingFailed
	ErrRateLimit                = types.ErrRateLimit
	ErrUnavailable              = types.ErrUnavailable
	ErrUnauthorized             = types.ErrUnauthorized
	ErrUnsupportedAPI           = types.ErrUnsupportedAPI
	ErrUnsupportedConfigVersion = types.ErrUnsupportedConfigVersion
	ErrUnsupportedMediaType     = types.ErrUnsupportedMediaType
)
