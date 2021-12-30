//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the types/ref package

package types

import (
	"github.com/regclient/regclient/types/ref"
)

type Ref = ref.Ref

var NewRef = ref.New
