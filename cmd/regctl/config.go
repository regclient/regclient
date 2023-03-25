package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/conffile"
	"github.com/regclient/regclient/pkg/template"
	"github.com/spf13/cobra"
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
	Filename      string                  `json:"-"`                 // filename that was loaded
	Version       int                     `json:"version,omitempty"` // version the file in case the config file syntax changes in the future
	Hosts         map[string]*config.Host `json:"hosts"`
	BlobLimit     int64                   `json:"blobLimit,omitempty"`
	IncDockerCert *bool                   `json:"incDockerCert,omitempty"`
	IncDockerCred *bool                   `json:"incDockerCred,omitempty"`
}

var configOpts struct {
	blobLimit  int64
	dockerCert bool
	dockerCred bool
	format     string
}
var configCmd = &cobra.Command{
	Use:   "config <cmd>",
	Short: "read/set configuration options",
}
var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "show the config",
	Long:  `Displays the configuration. Passwords are not included in the output.`,
	Args:  cobra.ExactArgs(0),
	RunE:  runConfigGet,
}
var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "set a configuration option",
	Long:  `Modifies an option used in future executions.`,
	Args:  cobra.ExactArgs(0),
	RunE:  runConfigSet,
}

func init() {
	configGetCmd.Flags().StringVar(&configOpts.format, "format", "{{ printPretty . }}", "format the output with Go template syntax")

	configSetCmd.Flags().Int64Var(&configOpts.blobLimit, "blob-limit", 0, "limit for blob chunks, this is stored in memory")
	configSetCmd.Flags().BoolVar(&configOpts.dockerCert, "docker-cert", false, "load certificates from docker")
	configSetCmd.Flags().BoolVar(&configOpts.dockerCred, "docker-cred", false, "load credentials from docker")

	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}
	for i := range c.Hosts {
		c.Hosts[i].Pass = ""
		c.Hosts[i].Token = ""
	}

	return template.Writer(cmd.OutOrStdout(), configOpts.format, c)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	c, err := ConfigLoadDefault()
	if err != nil {
		return err
	}

	if flagChanged(cmd, "blob-limit") {
		c.BlobLimit = configOpts.blobLimit
	}
	if flagChanged(cmd, "docker-cert") {
		if !configOpts.dockerCert {
			c.IncDockerCert = &configOpts.dockerCert
		} else {
			c.IncDockerCert = nil
		}
	}
	if flagChanged(cmd, "docker-cred") {
		if !configOpts.dockerCred {
			c.IncDockerCred = &configOpts.dockerCred
		} else {
			c.IncDockerCred = nil
		}
	}

	err = c.ConfigSave()
	if err != nil {
		return err
	}
	return nil
}

// ConfigNew creates an empty configuration
func ConfigNew() *Config {
	c := Config{
		Hosts: map[string]*config.Host{},
	}
	return &c
}

// ConfigLoadConfFile loads the config from an io reader
func ConfigLoadConfFile(cf *conffile.File) (*Config, error) {
	r, err := cf.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	c := ConfigNew()
	if err := json.NewDecoder(r).Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	c.Filename = cf.Name()
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
		if c.Hosts[h].TLS == config.TLSUndefined {
			c.Hosts[h].TLS = config.TLSEnabled
		}
		if h == config.DockerRegistryDNS || h == config.DockerRegistry || h == config.DockerRegistryAuth {
			// Docker Hub
			c.Hosts[h].Name = config.DockerRegistry
			if c.Hosts[h].Hostname == h {
				c.Hosts[h].Hostname = config.DockerRegistryDNS
			}
			if c.Hosts[h].CredHost == h {
				c.Hosts[h].CredHost = config.DockerRegistryAuth
			}
		}
		// ensure key matches Name
		if c.Hosts[h].Name != h {
			c.Hosts[c.Hosts[h].Name] = c.Hosts[h]
			delete(c.Hosts, h)
		}
	}
	return c, nil
}

// ConfigLoadFile loads the config from a specified filename
func ConfigLoadFile(filename string) (*Config, error) {
	cf := conffile.New(conffile.WithFullname(filename))
	if cf == nil {
		return nil, fmt.Errorf("failed to define config file")
	}
	return ConfigLoadConfFile(cf)
}

// ConfigLoadDefault loads the config from the (default) filename
func ConfigLoadDefault() (*Config, error) {
	cf := conffile.New(conffile.WithDirName(ConfigDir, ConfigFilename), conffile.WithEnvFile(ConfigEnv))
	if cf == nil {
		return nil, fmt.Errorf("failed to define config file")
	}
	c, err := ConfigLoadConfFile(cf)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		// do not error on file not found
		c := ConfigNew()
		c.Filename = cf.Name()
		return c, nil
	}
	return c, err
}

// ConfigSave saves to previously loaded filename
func (c *Config) ConfigSave() error {
	cf := conffile.New(conffile.WithFullname(c.Filename))
	if cf == nil {
		return ErrNotFound
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	outRdr := bytes.NewReader(out)
	return cf.Write(outRdr)
}
