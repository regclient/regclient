//go:build !nolegacy
// +build !nolegacy

// Package config is a legacy package, this has been moved to the config package
package config

import (
	topConfig "github.com/regclient/regclient/config"
)

type TLSConf = topConfig.TLSConf
type Host = topConfig.Host

const (
	TLSUndefined       = topConfig.TLSUndefined
	TLSEnabled         = topConfig.TLSEnabled
	TLSInsecure        = topConfig.TLSInsecure
	TLSDisabled        = topConfig.TLSDisabled
	DockerRegistry     = topConfig.DockerRegistry
	DockerRegistryAuth = topConfig.DockerRegistryAuth
	DockerRegistryDNS  = topConfig.DockerRegistryDNS
)

var HostNew = topConfig.HostNew
var HostNewName = topConfig.HostNewName
