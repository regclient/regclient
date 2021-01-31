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
	Scheme     string   `json:"scheme,omitempty"`
	TLS        TLSConf  `json:"tls,omitempty"`
	RegCert    string   `json:"regcert,omitempty"`
	ClientCert string   `json:"clientcert,omitempty"`
	ClientKey  string   `json:"clientkey,omitempty"`
	DNS        []string `json:"dns,omitempty"`
	User       string   `json:"user,omitempty"`
	Pass       string   `json:"pass,omitempty"`
}

// ConfigHostNew creates a default ConfigHost entry
func ConfigHostNew() *ConfigHost {
	h := ConfigHost{
		Scheme: "https",
		TLS:    TLSEnabled,
	}
	return &h
}

// ConfigHostNewName creates a default ConfigHost with a hostname
func ConfigHostNewName(host string) *ConfigHost {
	h := ConfigHost{
		Name:   host,
		Scheme: "https",
		TLS:    TLSEnabled,
		DNS:    []string{host},
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

	if newHost.Scheme != "" {
		if warn && curHost.Scheme != "" && curHost.Scheme != newHost.Scheme {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.Scheme,
				"new":  newHost.Scheme,
				"host": name,
			}).Warn("Changing scheme for registry")
		}
		curHost.Scheme = newHost.Scheme
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

	if len(newHost.DNS) > 0 {
		if warn && len(curHost.DNS) > 0 && !stringSliceEq(curHost.DNS, newHost.DNS) {
			rc.log.WithFields(logrus.Fields{
				"orig": curHost.DNS,
				"new":  newHost.DNS,
				"host": name,
			}).Warn("Changing certificate settings for registry")
		}
		curHost.DNS = newHost.DNS
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
