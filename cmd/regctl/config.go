package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/regclient/regclient/regclient"
)

var (
	// ConfigFilename is the default filename to read/write configuration
	ConfigFilename = "config.json"
	// ConfigDir is the default directory within the user's home directory to read/write configuration
	ConfigDir = ".regctl"
	// ConfigEnv is the environment variable to override the config filename
	ConfigEnv = "REGCTL_CONFIG"
)

// Config struct contains contents loaded from / saved to a config file
type Config struct {
	Filename      string                 `json:"-"`                 // filename that was loaded
	Version       int                    `json:"version,omitempty"` // version the file in case the config file syntax changes in the future
	Hosts         map[string]*ConfigHost `json:"hosts"`
	IncDockerCred *bool                  `json:"incDockerCred,omitempty"`
	IncDockerCert *bool                  `json:"incDockerCert,omitempty"`
}

// ConfigHost struct contains host specific settings
type ConfigHost struct {
	Name       string            `json:"-"`
	TLS        regclient.TLSConf `json:"tls,omitempty"`
	RegCert    string            `json:"regcert,omitempty"`
	ClientCert string            `json:"clientcert,omitempty"`
	ClientKey  string            `json:"clientkey,omitempty"`
	Hostname   string            `json:"hostname,omitempty"`
	User       string            `json:"user,omitempty"`
	Pass       string            `json:"pass,omitempty"`
	PathPrefix string            `json:"pathPrefix,omitempty"` // used for mirrors defined within a repository namespace
	Mirrors    []string          `json:"mirrors,omitempty"`    // list of other ConfigHost Names to use as mirrors
	Priority   uint              `json:"priority,omitempty"`   // priority when sorting mirrors, higher priority attempted first
	API        string            `json:"api,omitempty"`        // registry API to use
}

// ConfigHostNew creates a default ConfigHost entry
func ConfigHostNew() *ConfigHost {
	h := ConfigHost{
		TLS: regclient.TLSEnabled,
	}
	return &h
}

// getConfigFilename returns the filename based on environment variables and defaults
func getConfigFilename() string {
	cf := os.Getenv(ConfigEnv)
	if cf == "" {
		return filepath.Join(getHomeDir(), ConfigDir, ConfigFilename)
	}
	return cf
}

func getHomeDir() string {
	h := os.Getenv("HOME")
	if h == "" {
		if u, err := user.Current(); err == nil {
			return u.HomeDir
		}
	}
	return h
}

// ConfigNew creates an empty configuration
func ConfigNew() *Config {
	c := Config{
		Hosts: map[string]*ConfigHost{},
	}
	return &c
}

// ConfigLoadReader loads the config from an io reader
func ConfigLoadReader(r io.Reader) (*Config, error) {
	c := ConfigNew()
	if err := json.NewDecoder(r).Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	// verify loaded version is not higher than supported version
	if c.Version > 1 {
		return c, ErrUnsupportedConfigVersion
	}
	for h := range c.Hosts {
		if c.Hosts[h].Name == "" {
			c.Hosts[h].Name = h
		}
		if c.Hosts[h].Hostname == "" {
			c.Hosts[h].Hostname = h
		}
		if c.Hosts[h].TLS == regclient.TLSUndefined {
			c.Hosts[h].TLS = regclient.TLSEnabled
		}
		if h == regclient.DockerRegistryDNS || h == regclient.DockerRegistry {
			c.Hosts[h].Name = regclient.DockerRegistry
			if c.Hosts[h].Hostname == h {
				c.Hosts[h].Hostname = regclient.DockerRegistryDNS
			}
		}
		if c.Hosts[h].Name != h {
			c.Hosts[c.Hosts[h].Name] = c.Hosts[h]
			delete(c.Hosts, h)
		}
	}
	return c, nil
}

// ConfigLoadFile loads the config from a specified filename
func ConfigLoadFile(filename string) (*Config, error) {
	_, err := os.Stat(filename)
	if err == nil {
		file, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		c, err := ConfigLoadReader(file)
		if err != nil {
			return nil, err
		}
		c.Filename = filename
		return c, nil
	}
	return nil, err
}

// ConfigLoadDefault loads the config from the (default) filename
func ConfigLoadDefault() (*Config, error) {
	filename := getConfigFilename()
	c, err := ConfigLoadFile(filename)
	if err != nil && os.IsNotExist(err) {
		// do not error on file not found
		c = ConfigNew()
		c.Filename = filename
		return c, nil
	}
	return c, err
}

// ConfigSaveWriter writes formatted json to the writer
func (c *Config) ConfigSaveWriter(w io.Writer) error {
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

// ConfigSave saves to previously loaded filename
func (c *Config) ConfigSave() error {
	if c.Filename == "" {
		return ErrNotFound
	}

	// create directory if missing
	dir := filepath.Dir(c.Filename)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// use a temp file
	tmp, err := ioutil.TempFile(dir, filepath.Base(c.Filename))
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	// write as user formatted json
	if err := c.ConfigSaveWriter(tmp); err != nil {
		return err
	}
	tmp.Close()

	// follow symlink if it exists
	filename := c.Filename
	if link, err := os.Readlink(filename); err == nil {
		filename = link
	}

	// default file perms are 0600 owned by current user
	mode := os.FileMode(0600)
	uid := os.Getuid()
	gid := os.Getgid()
	// adjust defaults based on existing file if available
	stat, err := os.Stat(filename)
	if err == nil {
		// adjust mode to existing file
		if stat.Mode().IsRegular() {
			mode = stat.Mode()
		}
		uid, gid, _ = getFileOwner(stat)
	} else if !os.IsNotExist(err) {
		return err
	}

	// update mode and owner of temp file
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	if uid > 0 && gid > 0 {
		_ = os.Chown(tmp.Name(), uid, gid)
	}

	// move temp file to target filename
	return os.Rename(tmp.Name(), filename)
}
