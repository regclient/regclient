//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the config package

package regclient

import (
	"github.com/regclient/regclient/config"
)

type (
	// ConfigHost defines settings for connecting to a registry.
	//
	// Deprecated: replace with [config.Host].
	ConfigHost = config.Host
	// TLSConf specifies whether TLS is enabled and verified for a host.
	//
	// Deprecated: replace with [config.TLSConf].
	TLSConf = config.TLSConf
)

var (
	// ConfigHostNewName
	//
	// Deprecated: replace with [config.ConfigHostNewName].
	ConfigHostNewName = config.HostNewName
)

const (
	// TLSUndefined
	//
	// Deprecated: replace with [config.TLSUndefined].
	TLSUndefined = config.TLSUndefined
	// TLSEnabled
	//
	// Deprecated: replace with [config.TLSEnabled].
	TLSEnabled = config.TLSEnabled
	// TLSInsecure
	//
	// Deprecated: replace with [config.TLSInsecure].
	TLSInsecure = config.TLSInsecure
	// TLSDisabled
	//
	// Deprecated: replace with [config.TLSDisabled].
	TLSDisabled = config.TLSDisabled
)
