package regclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
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

// ConfigHost struct contains host specific settings
type ConfigHost struct {
	Name       string   `json:"-"`
	Scheme     string   `json:"scheme,omitempty"` // TODO: deprecate, delete
	TLS        TLSConf  `json:"tls,omitempty"`
	RegCert    string   `json:"regcert,omitempty"`
	ClientCert string   `json:"clientcert,omitempty"`
	ClientKey  string   `json:"clientkey,omitempty"`
	DNS        []string `json:"dns,omitempty"`      // TODO: remove slice, single string, or remove entirely?
	Hostname   string   `json:"hostname,omitempty"` // replaces DNS array with single string
	User       string   `json:"user,omitempty"`
	Pass       string   `json:"pass,omitempty"`
	Token      string   `json:"token,omitempty"`
	PathPrefix string   `json:"pathPrefix,omitempty"` // used for mirrors defined within a repository namespace
	Mirrors    []string `json:"mirrors,omitempty"`    // list of other ConfigHost Names to use as mirrors
	Priority   uint     `json:"priority,omitempty"`   // priority when sorting mirrors, higher priority attempted first
	API        string   `json:"api,omitempty"`        // registry API to use
}

// ConfigHostNew creates a default ConfigHost entry
func ConfigHostNew() *ConfigHost {
	h := ConfigHost{
		TLS: TLSEnabled,
	}
	return &h
}

// ConfigHostNewName creates a default ConfigHost with a hostname
func ConfigHostNewName(host string) *ConfigHost {
	h := ConfigHost{
		Name:     host,
		TLS:      TLSEnabled,
		Hostname: host,
	}
	if host == DockerRegistry || host == DockerRegistryDNS || host == DockerRegistryAuth {
		h.Name = DockerRegistry
		h.Hostname = DockerRegistryDNS
	}
	return &h
}

func (rc *regClient) mergeConfigHost(curHost, newHost ConfigHost, warn bool) ConfigHost {
	name := newHost.Name

	// merge the existing and new config host
	if newHost.User != "" {
		if warn && curHost.User != "" && curHost.User != newHost.User {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.User,
				"new":  newHost.User,
				"host": name,
			}).Warn("Changing login user for registry")
		}
		curHost.User = newHost.User
	}

	if newHost.Pass != "" {
		if warn && curHost.Pass != "" && curHost.Pass != newHost.Pass {
			rc.log.WithFields(logrus.Fields{
				"host": name,
			}).Warn("Changing login password for registry")
		}
		curHost.Pass = newHost.Pass
	}

	if newHost.Token != "" {
		if warn && curHost.Token != "" && curHost.Token != newHost.Token {
			rc.log.WithFields(logrus.Fields{
				"host": name,
			}).Warn("Changing login token for registry")
		}
		curHost.Token = newHost.Token
	}

	if newHost.TLS != TLSUndefined {
		if warn && curHost.TLS != TLSUndefined && curHost.TLS != newHost.TLS {
			tlsOrig, _ := curHost.TLS.MarshalText()
			tlsNew, _ := newHost.TLS.MarshalText()
			rc.log.WithFields(logrus.Fields{
				"orig": string(tlsOrig),
				"new":  string(tlsNew),
				"host": name,
			}).Warn("Changing TLS settings for registry")
		}
		curHost.TLS = newHost.TLS
	}

	if newHost.RegCert != "" {
		if warn && curHost.RegCert != "" && curHost.RegCert != newHost.RegCert {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.RegCert,
				"new":  newHost.RegCert,
				"host": name,
			}).Warn("Changing certificate settings for registry")
		}
		curHost.RegCert = newHost.RegCert
	}

	if newHost.ClientCert != "" {
		if warn && curHost.ClientCert != "" && curHost.ClientCert != newHost.ClientCert {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.ClientCert,
				"new":  newHost.ClientCert,
				"host": name,
			}).Warn("Changing client certificate settings for registry")
		}
		curHost.ClientCert = newHost.ClientCert
	}

	if newHost.ClientKey != "" {
		if warn && curHost.ClientKey != "" && curHost.ClientKey != newHost.ClientKey {
			rc.log.WithFields(logrus.Fields{
				"host": name,
			}).Warn("Changing client certificate key settings for registry")
		}
		curHost.ClientKey = newHost.ClientKey
	}

	if newHost.Hostname != "" {
		if warn && curHost.Hostname != "" && curHost.Hostname != newHost.Hostname {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.Hostname,
				"new":  newHost.Hostname,
				"host": name,
			}).Warn("Changing hostname settings for registry")
		}
		curHost.Hostname = newHost.Hostname
	}

	if newHost.PathPrefix != "" {
		newHost.PathPrefix = strings.Trim(newHost.PathPrefix, "/") // leading and trailing / are not needed
		if warn && curHost.PathPrefix != "" && curHost.PathPrefix != newHost.PathPrefix {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.PathPrefix,
				"new":  newHost.PathPrefix,
				"host": name,
			}).Warn("Changing path prefix settings for registry")
		}
		curHost.PathPrefix = newHost.PathPrefix
	}

	if len(newHost.Mirrors) > 0 {
		if warn && len(curHost.Mirrors) > 0 && !stringSliceEq(curHost.Mirrors, newHost.Mirrors) {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.Mirrors,
				"new":  newHost.Mirrors,
				"host": name,
			}).Warn("Changing mirror settings for registry")
		}
		curHost.Mirrors = newHost.Mirrors
	}

	if newHost.Priority != 0 {
		if warn && curHost.Priority != 0 && curHost.Priority != newHost.Priority {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.Priority,
				"new":  newHost.Priority,
				"host": name,
			}).Warn("Changing priority settings for registry")
		}
		curHost.Priority = newHost.Priority
	}

	if newHost.API != "" {
		if warn && curHost.API != "" && curHost.API != newHost.API {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.API,
				"new":  newHost.API,
				"host": name,
			}).Warn("Changing API settings for registry")
		}
		curHost.API = newHost.API
	}

	return curHost
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
