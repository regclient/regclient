//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the config package

package regclient

import (
	"github.com/regclient/regclient/config"
)

type ConfigHost = config.Host
type TLSConf = config.TLSConf

var ConfigHostNewName = config.HostNewName

const (
	TLSUndefined = config.TLSUndefined
	TLSEnabled   = config.TLSEnabled
	TLSInsecure  = config.TLSInsecure
	TLSDisabled  = config.TLSDisabled
)
