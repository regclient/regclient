package regclient

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
)

const (
	configFilename = "config.json"
	configDir      = ".regcli"
)

// Config struct contains contents loaded from / saved to a config file
type Config struct {
	Filename string                 `json:"-"`                 // filename that was loaded
	Version  int                    `json:"version,omitempty"` // version the file in case the config file syntax changes in the future
	Hosts    map[string]*ConfigHost `json:"hosts"`
}

// ConfigHost struct contains host specific settings
type ConfigHost struct {
	Name    string   `json:"-"`
	Scheme  string   `json:"scheme,omitempty"`
	TLS     tlsConf  `json:"tls,omitempty"`
	TLSCert string   `json:"tlscert,omitempty"`
	TLSKey  string   `json:"tlskey,omitempty"`
	DNS     []string `json:"dns,omitempty"`
	User    string   `json:"user,omitempty"`
	Pass    string   `json:"pass,omitempty"`
}

// getConfigFilename returns the filename based on environment variables and defaults
func getConfigFilename() string {
	cf := os.Getenv("REGCLI_CONFIG")
	if cf == "" {
		return filepath.Join(getHomeDir(), configDir, configFilename)
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

// ConfigHostNew creates a default ConfigHost entry
func ConfigHostNew() *ConfigHost {
	h := ConfigHost{
		Scheme: "https",
		TLS:    tlsEnabled,
	}
	return &h
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
		c.Hosts[h].Name = h
		if c.Hosts[h].DNS == nil {
			c.Hosts[h].DNS = []string{h}
		}
		if c.Hosts[h].Scheme == "" {
			c.Hosts[h].Scheme = "https"
		}
		if c.Hosts[h].TLS == tlsUndefined {
			c.Hosts[h].TLS = tlsEnabled
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
		// why doesn't Go make uid/gid easier to retrieve?
		if sysstat, ok := stat.Sys().(*syscall.Stat_t); ok {
			uid = int(sysstat.Uid)
			gid = int(sysstat.Gid)
		}
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
