//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the types/ref package

package types

import (
	"github.com/regclient/regclient/types/ref"
)

// Ref is used for a reference to an image or repository.
//
// Deprecated: replace with [ref.Ref].
type Ref = ref.Ref

// NewRef create a new [Ref].
//
// Deprecated: replace with [ref.New].
var NewRef = ref.New
