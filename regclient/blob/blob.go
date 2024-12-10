//go:build legacy
// +build legacy

// Package blob is a legacy package, this has been moved to the types/blob package
package blob

import (
	topBlob "github.com/regclient/regclient/types/blob"
)

type (
	// Blob specifies a generic blob.
	//
	// Deprecated: replace with [blob.Blob].
	Blob = topBlob.Blob
	// OCIConfig is an interface for an OCI Config.
	//
	// Deprecated: replace with [blob.OCIConfig].
	OCIConfig = topBlob.OCIConfig
	// Common is an interface of common methods for blobs.
	//
	// Deprecated: replace with [blob.Common].
	Common = topBlob.Common
	// Reader is an interface for blob reader methods.
	//
	// Deprecated: replace with [blob.Reader].
	Reader = topBlob.Reader
)

var (
	// NewOCIConfig
	//
	// Deprecated: replace with [blob.NewOCIConfig].
	NewOCIConfig = topBlob.NewOCIConfig
	// NewReader
	//
	// Deprecated: replace with [blob.NewReader].
	NewReader = topBlob.NewReader
)
