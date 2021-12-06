package regclient

import (
	"github.com/regclient/regclient/regclient/config"
)

// Backwards compatibility types
// TODO: eventually delete
type ConfigHost = config.Host
type TLSConf = config.TLSConf

// TODO: eventually delete
var ConfigHostNewName = config.HostNewName

// TODO: eventually delete
const (
	TLSUndefined = config.TLSUndefined
	TLSEnabled   = config.TLSEnabled
	TLSInsecure  = config.TLSInsecure
	TLSDisabled  = config.TLSDisabled
)
