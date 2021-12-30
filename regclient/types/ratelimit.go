//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to top level types package

package types

import (
	topTypes "github.com/regclient/regclient/types"
)

// RateLimit is returned from some http requests
type RateLimit = topTypes.RateLimit
