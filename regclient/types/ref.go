//go:build legacy
// +build legacy

// Legacy package, this has been moved to the types/ref package

package types

import (
	"github.com/regclient/regclient/types/ref"
)

// Ref is used for a reference to an image or repository.
//
// Deprecated: replace with [ref.Ref].
//
//go:fix inline
type Ref = ref.Ref

// NewRef create a new [Ref].
//
// Deprecated: replace with [ref.New].
//
//go:fix inline
var NewRef = ref.New
