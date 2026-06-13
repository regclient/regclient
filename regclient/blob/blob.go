// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	//go:fix inline
	Blob = topBlob.Blob
	// OCIConfig is an interface for an OCI Config.
	//
	// Deprecated: replace with [blob.OCIConfig].
	//go:fix inline
	OCIConfig = topBlob.OCIConfig
	// Common is an interface of common methods for blobs.
	//
	// Deprecated: replace with [blob.Common].
	//go:fix inline
	Common = topBlob.Common
	// Reader is an interface for blob reader methods.
	//
	// Deprecated: replace with [blob.Reader].
	//go:fix inline
	Reader = topBlob.Reader
)

var (
	// NewOCIConfig
	//
	// Deprecated: replace with [blob.NewOCIConfig].
	//go:fix inline
	NewOCIConfig = topBlob.NewOCIConfig
	// NewReader
	//
	// Deprecated: replace with [blob.NewReader].
	//go:fix inline
	NewReader = topBlob.NewReader
)
