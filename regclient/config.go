package regclient

import (
	"encoding/json"
	"fmt"
	"strings"
)

var (
	// ConfigFilename is the default filename to read/write configuration
	ConfigFilename = "config.json"
	// ConfigDir is the default directory within the user's home directory to read/write configuration
	ConfigDir = ".regclient"
	// ConfigEnv is the environment variable to override the config filename
	ConfigEnv = "REGCLIENT_CONFIG"
)

// TLSConf specifies whether TLS is enabled for a host
type TLSConf int

const (
	// TLSUndefined indicates TLS is not passed, defaults to Enabled
	TLSUndefined TLSConf = iota
	// TLSEnabled uses TLS (https) for the connection
	TLSEnabled
	// TLSInsecure uses TLS but does not verify CA
	TLSInsecure
	// TLSDisabled does not use TLS (http)
	TLSDisabled
)

// MarshalJSON converts to a json string using MarshalText
func (t TLSConf) MarshalJSON() ([]byte, error) {
	s, err := t.MarshalText()
	if err != nil {
		return []byte(""), err
	}
	return json.Marshal(string(s))
}

// MarshalText converts TLSConf to a string
func (t TLSConf) MarshalText() ([]byte, error) {
	var s string
	switch t {
	default:
		s = ""
	case TLSEnabled:
		s = "enabled"
	case TLSInsecure:
		s = "insecure"
	case TLSDisabled:
		s = "disabled"
	}
	return []byte(s), nil
}

// UnmarshalJSON converts TLSConf from a json string
func (t *TLSConf) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(s))
}

// UnmarshalText converts TLSConf from a string
func (t *TLSConf) UnmarshalText(b []byte) error {
	switch strings.ToLower(string(b)) {
	default:
		return fmt.Errorf("Unknown TLS value \"%s\"", b)
	case "":
		*t = TLSUndefined
	case "enabled":
		*t = TLSEnabled
	case "insecure":
		*t = TLSInsecure
	case "disabled":
		*t = TLSDisabled
	}
	return nil
}

// Config struct contains contents loaded from / saved to a config file
type Config struct {
	// Filename      string                 `json:"-"`                 // filename that was loaded
	// Version       int                    `json:"version,omitempty"` // version the file in case the config file syntax changes in the future
	Hosts         map[string]*ConfigHost `json:"hosts"`
	IncDockerCred *bool                  `json:"incDockerCred,omitempty"`
	// IncDockerCert *bool                  `json:"incDockerCert,omitempty"`
}

// ConfigHost struct contains host specific settings
type ConfigHost struct {
	Name       string   `json:"-"`
	Scheme     string   `json:"scheme,omitempty"`
	TLS        TLSConf  `json:"tls,omitempty"`
	RegCert    string   `json:"regcert,omitempty"`
	ClientCert string   `json:"clientcert,omitempty"`
	ClientKey  string   `json:"clientkey,omitempty"`
	DNS        []string `json:"dns,omitempty"`
	User       string   `json:"user,omitempty"`
	Pass       string   `json:"pass,omitempty"`
}

// ConfigNew creates an empty configuration
func ConfigNew() *Config {
	c := Config{
		Hosts: map[string]*ConfigHost{},
	}
	return &c
}

// ConfigHostNew creates a default ConfigHost entry
func ConfigHostNew() *ConfigHost {
	h := ConfigHost{
		Scheme: "https",
		TLS:    TLSEnabled,
	}
	return &h
}
