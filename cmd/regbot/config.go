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

// delay checking for at least 5 minutes when rate limit is exceeded
var rateLimitRetryMin time.Duration

func init() {
	rateLimitRetryMin, _ = time.ParseDuration("5m")
}

// Config is parsed configuration file for regsync
type Config struct {
	Version  int            `json:"version"`
	Creds    []ConfigCreds  `json:"creds"`
	Defaults ConfigDefaults `json:"defaults"`
	Scripts  []ConfigScript `json:"scripts"`
}

// ConfigCreds allows the registry login to be passed in the config rather than from Docker
type ConfigCreds struct {
	Registry string            `json:"registry"`
	User     string            `json:"user"`
	Pass     string            `json:"pass"`
	TLS      regclient.TLSConf `json:"tls"`
	Scheme   string            `json:"scheme"`
	RegCert  string            `json:"regcert"`
}

// ConfigDefaults is uses for general options and defaults for ConfigScript entries
type ConfigDefaults struct {
	Interval       time.Duration   `json:"interval"`
	Schedule       string          `json:"schedule"`
	RateLimit      ConfigRateLimit `json:"ratelimit"`
	Parallel       int             `json:"parallel"`
	SkipDockerConf bool            `json:"skipDockerConfig`
}

// ConfigRateLimit is for rate limit settings
type ConfigRateLimit struct {
	Min   int           `json:"min"`
	Retry time.Duration `json:"retry"`
}

// ConfigScript defines a source/target repository to sync
type ConfigScript struct {
	Name      string          `json:"name"`
	Script    string          `json:"script"`
	Interval  time.Duration   `json:"interval"`
	Schedule  string          `json:"schedule"`
	RateLimit ConfigRateLimit `json:"ratelimit"`
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
	if c.Defaults.RateLimit.Retry < rateLimitRetryMin {
		c.Defaults.RateLimit.Retry = rateLimitRetryMin
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
	if s.RateLimit.Min == 0 && d.RateLimit.Min != 0 {
		s.RateLimit.Min = d.RateLimit.Min
	}
	if s.RateLimit.Retry == 0 {
		s.RateLimit.Retry = d.RateLimit.Retry
	} else if s.RateLimit.Retry < rateLimitRetryMin {
		s.RateLimit.Retry = rateLimitRetryMin
	}
}
