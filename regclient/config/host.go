//go:build legacy
// +build legacy

// Package config is a legacy package, this has been moved to the config package
package config

import (
	topConfig "github.com/regclient/regclient/config"
)

type (
	// TLSConf defines the TLS enumerated values.
	//
	// Deprecated: replace with [config.TLSConf].
	TLSConf = topConfig.TLSConf
	// Host defines a registry configuration.
	//
	// Deprecated: replace with [config.Host].
	Host = topConfig.Host
)

const (
	// TLSUndefined
	//
	// Deprecated: replace with [config.TLSUndefined].
	TLSUndefined = topConfig.TLSUndefined
	// TLSEnabled
	//
	// Deprecated: replace with [config.TLSEnabled].
	TLSEnabled = topConfig.TLSEnabled
	// TLSInsecure
	//
	// Deprecated: replace with [config.TLSInsecure].
	TLSInsecure = topConfig.TLSInsecure
	// TLSDisabled
	//
	// Deprecated: replace with [config.TLSDisabled].
	TLSDisabled = topConfig.TLSDisabled
	// DockerRegistry
	//
	// Deprecated: replace with [config.DockerRegistry].
	DockerRegistry = topConfig.DockerRegistry
	// DockerRegistryAuth
	//
	// Deprecated: replace with [config.DockerRegistryAuth].
	DockerRegistryAuth = topConfig.DockerRegistryAuth
	// DockerRegistryDNS
	//
	// Deprecated: replace with [config.DockerRegistryDNS].
	DockerRegistryDNS = topConfig.DockerRegistryDNS
)

var (
	// HostNew
	//
	// Deprecated: replace with [config.HostNew].
	HostNew = topConfig.HostNew
	// HostNewName
	//
	// Deprecated: replace with [config.HostNewName].
	HostNewName = topConfig.HostNewName
)
