//go:build legacy
// +build legacy

// Legacy package, this has been moved to top level types package

package types

import (
	topTypes "github.com/regclient/regclient/types"
)

// RateLimit is returned from some http requests
//
// Deprecated: replace with [types.RateLimit].
//
//go:fix inline
type RateLimit = topTypes.RateLimit
