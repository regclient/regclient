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

// Legacy package, this has been moved to the config package

package regclient

import (
	"github.com/regclient/regclient/config"
)

type (
	// ConfigHost defines settings for connecting to a registry.
	//
	// Deprecated: replace with [config.Host].
	//go:fix inline
	ConfigHost = config.Host
	// TLSConf specifies whether TLS is enabled and verified for a host.
	//
	// Deprecated: replace with [config.TLSConf].
	//go:fix inline
	TLSConf = config.TLSConf
)

// ConfigHostNewName
//
// Deprecated: replace with [config.ConfigHostNewName].
//
//go:fix inline
var ConfigHostNewName = config.HostNewName

const (
	// TLSUndefined
	//
	// Deprecated: replace with [config.TLSUndefined].
	//go:fix inline
	TLSUndefined = config.TLSUndefined
	// TLSEnabled
	//
	// Deprecated: replace with [config.TLSEnabled].
	//go:fix inline
	TLSEnabled = config.TLSEnabled
	// TLSInsecure
	//
	// Deprecated: replace with [config.TLSInsecure].
	//go:fix inline
	TLSInsecure = config.TLSInsecure
	// TLSDisabled
	//
	// Deprecated: replace with [config.TLSDisabled].
	//go:fix inline
	TLSDisabled = config.TLSDisabled
)
