package main

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
	"gopkg.in/yaml.v2"
)

// Config is parsed configuration file for regsync
type Config struct {
	Version  int            `yaml:"version" json:"version"`
	Creds    []ConfigCreds  `yaml:"creds" json:"creds"`
	Defaults ConfigDefaults `yaml:"defaults" json:"defaults"`
	Scripts  []ConfigScript `yaml:"scripts" json:"scripts"`
}

// ConfigCreds allows the registry login to be passed in the config rather than from Docker
type ConfigCreds struct {
	Registry   string            `yaml:"registry" json:"registry"`
	Hostname   string            `yaml:"hostname" json:"hostname"`
	User       string            `yaml:"user" json:"user"`
	Pass       string            `yaml:"pass" json:"pass"`
	TLS        regclient.TLSConf `yaml:"tls" json:"tls"`
	Scheme     string            `yaml:"scheme" json:"scheme"` // TODO: delete
	RegCert    string            `yaml:"regcert" json:"regcert"`
	PathPrefix string            `yaml:"pathPrefix" json:"pathPrefix"`
	Mirrors    []string          `yaml:"mirrors" json:"mirrors"`
	Priority   uint              `yaml:"priority" json:"priority"`
	API        string            `yaml:"api" json:"api"`
}

// ConfigDefaults is uses for general options and defaults for ConfigScript entries
type ConfigDefaults struct {
	Interval       time.Duration `yaml:"interval" json:"interval"`
	Schedule       string        `yaml:"schedule" json:"schedule"`
	Parallel       int           `yaml:"parallel" json:"parallel"`
	SkipDockerConf bool          `yaml:"skipDockerConfig" json:"skipDockerConfig"`
	Timeout        time.Duration `yaml:"timeout" json:"timeout"`
}

// ConfigScript defines a source/target repository to sync
type ConfigScript struct {
	Name     string        `yaml:"name" json:"name"`
	Script   string        `yaml:"script" json:"script"`
	Interval time.Duration `yaml:"interval" json:"interval"`
	Schedule string        `yaml:"schedule" json:"schedule"`
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`
}

// ConfigNew creates an empty configuration
func ConfigNew() *Config {
	c := Config{
		Creds:   []ConfigCreds{},
		Scripts: []ConfigScript{},
	}
	return &c
}

// ConfigLoadReader reads the config from an io.Reader
func ConfigLoadReader(r io.Reader) (*Config, error) {
	c := ConfigNew()
	if err := yaml.NewDecoder(r).Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	// verify loaded version is not higher than supported version
	if c.Version > 1 {
		return c, ErrUnsupportedConfigVersion
	}
	// apply top level defaults
	if c.Defaults.Parallel <= 0 {
		c.Defaults.Parallel = 1
	}
	// apply defaults to each step
	for i := range c.Scripts {
		scriptSetDefaults(&c.Scripts[i], c.Defaults)
	}
	err := configExpandTemplates(c)
	if err != nil {
		return nil, err
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
		return c, nil
	}
	return nil, err
}

// expand templates in various parts of the config
func configExpandTemplates(c *Config) error {
	for i := range c.Creds {
		val, err := template.String(c.Creds[i].User, nil)
		if err != nil {
			return err
		}
		c.Creds[i].User = val
		val, err = template.String(c.Creds[i].Pass, nil)
		if err != nil {
			return err
		}
		c.Creds[i].Pass = val
		val, err = template.String(c.Creds[i].RegCert, nil)
		if err != nil {
			return err
		}
		c.Creds[i].RegCert = val
	}
	// for i := range c.Scripts {
	// 	val, err := template.String(c.Scripts[i].Script, nil)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	c.Scripts[i].Script = val
	// }
	return nil
}

// updates script entry with defaults
func scriptSetDefaults(s *ConfigScript, d ConfigDefaults) {
	if s.Schedule == "" && d.Schedule != "" {
		s.Schedule = d.Schedule
	}
	if s.Interval == 0 && s.Schedule == "" && d.Interval != 0 {
		s.Interval = d.Interval
	}
	if s.Timeout == 0 && d.Timeout != 0 {
		s.Timeout = d.Timeout
	}
}
