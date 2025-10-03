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

var (
	// ConfigHostNewName
	//
	// Deprecated: replace with [config.ConfigHostNewName].
	//go:fix inline
	ConfigHostNewName = config.HostNewName
)

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
