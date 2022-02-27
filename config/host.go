// Package config is used for all regclient configuration settings
package config

import (
	"encoding/json"
	"fmt"
	"io"
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

const (
	// DockerRegistry is the name resolved in docker images on Hub
	DockerRegistry = "docker.io"
	// DockerRegistryAuth is the name provided in docker's config for Hub
	DockerRegistryAuth = "https://index.docker.io/v1/"
	// DockerRegistryDNS is the host to connect to for Hub
	DockerRegistryDNS = "registry-1.docker.io"
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
		return fmt.Errorf("unknown TLS value \"%s\"", b)
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

// Host struct contains host specific settings
type Host struct {
	Name       string            `json:"-"`
	Scheme     string            `json:"scheme,omitempty"` // TODO: deprecate, delete
	TLS        TLSConf           `json:"tls,omitempty"`
	RegCert    string            `json:"regcert,omitempty"`
	ClientCert string            `json:"clientcert,omitempty"`
	ClientKey  string            `json:"clientkey,omitempty"`
	DNS        []string          `json:"dns,omitempty"`      // TODO: remove slice, single string, or remove entirely?
	Hostname   string            `json:"hostname,omitempty"` // replaces DNS array with single string
	User       string            `json:"user,omitempty"`
	Pass       string            `json:"pass,omitempty"`
	Token      string            `json:"token,omitempty"`
	PathPrefix string            `json:"pathPrefix,omitempty"` // used for mirrors defined within a repository namespace
	Mirrors    []string          `json:"mirrors,omitempty"`    // list of other Host Names to use as mirrors
	Priority   uint              `json:"priority,omitempty"`   // priority when sorting mirrors, higher priority attempted first
	RepoAuth   bool              `json:"repoAuth,omitempty"`   // tracks a separate auth per repo
	API        string            `json:"api,omitempty"`        // experimental: registry API to use
	APIOpts    map[string]string `json:"apiOpts,omitempty"`    // options for APIs
	BlobChunk  int64             `json:"blobChunk,omitempty"`  // size of each blob chunk
	BlobMax    int64             `json:"blobMax,omitempty"`    // threshold to switch to chunked upload, -1 to disable, 0 for regclient.blobMaxPut
}

// HostNew creates a default Host entry
func HostNew() *Host {
	h := Host{
		TLS:     TLSEnabled,
		APIOpts: map[string]string{},
	}
	return &h
}

// HostNewName creates a default Host with a hostname
func HostNewName(host string) *Host {
	h := Host{
		Name:     host,
		TLS:      TLSEnabled,
		Hostname: host,
		APIOpts:  map[string]string{},
	}
	if host == DockerRegistry || host == DockerRegistryDNS || host == DockerRegistryAuth {
		h.Name = DockerRegistry
		h.Hostname = DockerRegistryDNS
	}
	return &h
}

// Merge adds fields from a new config host entry
func (host *Host) Merge(newHost Host, log *logrus.Logger) error {
	name := newHost.Name
	if name == "" {
		name = host.Name
	}
	if log == nil {
		log = &logrus.Logger{Out: io.Discard}
	}

	// merge the existing and new config host
	if host.Name == "" {
		// only set the name if it's not initialized, this shouldn't normally change
		host.Name = newHost.Name
	}

	if newHost.User != "" {
		if host.User != "" && host.User != newHost.User {
			log.WithFields(logrus.Fields{
				"orig": host.User,
				"new":  newHost.User,
				"host": name,
			}).Warn("Changing login user for registry")
		}
		host.User = newHost.User
	}

	if newHost.Pass != "" {
		if host.Pass != "" && host.Pass != newHost.Pass {
			log.WithFields(logrus.Fields{
				"host": name,
			}).Warn("Changing login password for registry")
		}
		host.Pass = newHost.Pass
	}

	if newHost.Token != "" {
		if host.Token != "" && host.Token != newHost.Token {
			log.WithFields(logrus.Fields{
				"host": name,
			}).Warn("Changing login token for registry")
		}
		host.Token = newHost.Token
	}

	if newHost.TLS != TLSUndefined {
		if host.TLS != TLSUndefined && host.TLS != newHost.TLS {
			tlsOrig, _ := host.TLS.MarshalText()
			tlsNew, _ := newHost.TLS.MarshalText()
			log.WithFields(logrus.Fields{
				"orig": string(tlsOrig),
				"new":  string(tlsNew),
				"host": name,
			}).Warn("Changing TLS settings for registry")
		}
		host.TLS = newHost.TLS
	}

	if newHost.RegCert != "" {
		if host.RegCert != "" && host.RegCert != newHost.RegCert {
			log.WithFields(logrus.Fields{
				"orig": host.RegCert,
				"new":  newHost.RegCert,
				"host": name,
			}).Warn("Changing certificate settings for registry")
		}
		host.RegCert = newHost.RegCert
	}

	if newHost.ClientCert != "" {
		if host.ClientCert != "" && host.ClientCert != newHost.ClientCert {
			log.WithFields(logrus.Fields{
				"orig": host.ClientCert,
				"new":  newHost.ClientCert,
				"host": name,
			}).Warn("Changing client certificate settings for registry")
		}
		host.ClientCert = newHost.ClientCert
	}

	if newHost.ClientKey != "" {
		if host.ClientKey != "" && host.ClientKey != newHost.ClientKey {
			log.WithFields(logrus.Fields{
				"host": name,
			}).Warn("Changing client certificate key settings for registry")
		}
		host.ClientKey = newHost.ClientKey
	}

	if newHost.Hostname != "" {
		if host.Hostname != "" && host.Hostname != newHost.Hostname {
			log.WithFields(logrus.Fields{
				"orig": host.Hostname,
				"new":  newHost.Hostname,
				"host": name,
			}).Warn("Changing hostname settings for registry")
		}
		host.Hostname = newHost.Hostname
	}

	if newHost.PathPrefix != "" {
		newHost.PathPrefix = strings.Trim(newHost.PathPrefix, "/") // leading and trailing / are not needed
		if host.PathPrefix != "" && host.PathPrefix != newHost.PathPrefix {
			log.WithFields(logrus.Fields{
				"orig": host.PathPrefix,
				"new":  newHost.PathPrefix,
				"host": name,
			}).Warn("Changing path prefix settings for registry")
		}
		host.PathPrefix = newHost.PathPrefix
	}

	if len(newHost.Mirrors) > 0 {
		if len(host.Mirrors) > 0 && !stringSliceEq(host.Mirrors, newHost.Mirrors) {
			log.WithFields(logrus.Fields{
				"orig": host.Mirrors,
				"new":  newHost.Mirrors,
				"host": name,
			}).Warn("Changing mirror settings for registry")
		}
		host.Mirrors = newHost.Mirrors
	}

	if newHost.Priority != 0 {
		if host.Priority != 0 && host.Priority != newHost.Priority {
			log.WithFields(logrus.Fields{
				"orig": host.Priority,
				"new":  newHost.Priority,
				"host": name,
			}).Warn("Changing priority settings for registry")
		}
		host.Priority = newHost.Priority
	}

	if newHost.RepoAuth {
		host.RepoAuth = newHost.RepoAuth
	}

	if newHost.API != "" {
		if host.API != "" && host.API != newHost.API {
			log.WithFields(logrus.Fields{
				"orig": host.API,
				"new":  newHost.API,
				"host": name,
			}).Warn("Changing API settings for registry")
		}
		host.API = newHost.API
	}

	if len(newHost.APIOpts) > 0 {
		if len(host.APIOpts) > 0 {
			merged := copyMapString(host.APIOpts)
			for k, v := range newHost.APIOpts {
				if host.APIOpts[k] != "" && host.APIOpts[k] != v {
					log.WithFields(logrus.Fields{
						"orig": host.APIOpts[k],
						"new":  newHost.APIOpts[k],
						"opt":  k,
						"host": name,
					}).Warn("Changing APIOpts setting for registry")
				}
				merged[k] = v
			}
			host.APIOpts = merged
		} else {
			host.APIOpts = newHost.APIOpts
		}
	}

	if newHost.BlobChunk > 0 {
		if host.BlobChunk != 0 && host.BlobChunk != newHost.BlobChunk {
			log.WithFields(logrus.Fields{
				"orig": host.BlobChunk,
				"new":  newHost.BlobChunk,
				"host": name,
			}).Warn("Changing blobChunk settings for registry")
		}
		host.BlobChunk = newHost.BlobChunk
	}

	if newHost.BlobMax != 0 {
		if host.BlobMax != 0 && host.BlobMax != newHost.BlobMax {
			log.WithFields(logrus.Fields{
				"orig": host.BlobMax,
				"new":  newHost.BlobMax,
				"host": name,
			}).Warn("Changing blobMax settings for registry")
		}
		host.BlobMax = newHost.BlobMax
	}

	return nil
}

func copyMapString(src map[string]string) map[string]string {
	copy := map[string]string{}
	for k, v := range src {
		copy[k] = v
	}
	return copy
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
