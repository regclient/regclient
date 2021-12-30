//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the types/blob package

package blob

import (
	topBlob "github.com/regclient/regclient/types/blob"
)

type Blob = topBlob.Blob
type OCIConfig = topBlob.OCIConfig
type Common = topBlob.Common
type Reader = topBlob.Reader

var (
	NewOCIConfig = topBlob.NewOCIConfig
	NewReader    = topBlob.NewReader
)
