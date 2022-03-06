package main

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types"
	"gopkg.in/yaml.v2"
)

// delay checking for at least 5 minutes when rate limit is exceeded
var rateLimitRetryMin time.Duration
var defaultMediaTypes = []string{
	types.MediaTypeDocker2Manifest,
	types.MediaTypeDocker2ManifestList,
	types.MediaTypeOCI1Manifest,
	types.MediaTypeOCI1ManifestList,
}

func init() {
	rateLimitRetryMin, _ = time.ParseDuration("5m")
}

// Config is parsed configuration file for regsync
type Config struct {
	Version  int            `yaml:"version" json:"version"`
	Creds    []ConfigCreds  `yaml:"creds" json:"creds"`
	Defaults ConfigDefaults `yaml:"defaults" json:"defaults"`
	Sync     []ConfigSync   `yaml:"sync" json:"sync"`
}

// ConfigCreds allows the registry login to be passed in the config rather than from Docker
type ConfigCreds struct {
	Registry   string            `yaml:"registry" json:"registry"`
	Hostname   string            `yaml:"hostname" json:"hostname"`
	User       string            `yaml:"user" json:"user"`
	Pass       string            `yaml:"pass" json:"pass"`
	Token      string            `yaml:"token" json:"token"`
	TLS        config.TLSConf    `yaml:"tls" json:"tls"`
	Scheme     string            `yaml:"scheme" json:"scheme"` // TODO: eventually delete
	RegCert    string            `yaml:"regcert" json:"regcert"`
	PathPrefix string            `yaml:"pathPrefix" json:"pathPrefix"`
	Mirrors    []string          `yaml:"mirrors" json:"mirrors"`
	Priority   uint              `yaml:"priority" json:"priority"`
	RepoAuth   bool              `yaml:"repoAuth" json:"repoAuth"`
	API        string            `yaml:"api" json:"api"`
	APIOpts    map[string]string `yaml:"apiOpts" json:"apiOpts"`
	BlobChunk  int64             `yaml:"blobChunk" json:"blobChunk"`
	BlobMax    int64             `yaml:"blobMax" json:"blobMax"`
}

func credsToRCHost(c ConfigCreds) config.Host {
	return config.Host{
		Name:       c.Registry,
		Hostname:   c.Hostname,
		User:       c.User,
		Pass:       c.Pass,
		Token:      c.Token,
		TLS:        c.TLS,
		RegCert:    c.RegCert,
		PathPrefix: c.PathPrefix,
		Mirrors:    c.Mirrors,
		Priority:   c.Priority,
		RepoAuth:   c.RepoAuth,
		API:        c.API,
		APIOpts:    c.APIOpts,
		BlobChunk:  c.BlobChunk,
		BlobMax:    c.BlobMax,
	}
}

// ConfigDefaults is uses for general options and defaults for ConfigSync entries
type ConfigDefaults struct {
	Backup         string          `yaml:"backup" json:"backup"`
	Interval       time.Duration   `yaml:"interval" json:"interval"`
	Schedule       string          `yaml:"schedule" json:"schedule"`
	RateLimit      ConfigRateLimit `yaml:"ratelimit" json:"ratelimit"`
	Parallel       int             `yaml:"parallel" json:"parallel"`
	DigestTags     *bool           `yaml:"digestTags" json:"digestTags"`
	ForceRecursive *bool           `yaml:"forceRecursive" json:"forceRecursive"`
	MediaTypes     []string        `yaml:"mediaTypes" json:"mediaTypes"`
	SkipDockerConf bool            `yaml:"skipDockerConfig" json:"skipDockerConfig"`
	Hooks          ConfigHooks     `yaml:"hooks" json:"hooks"`
	UserAgent      string          `yaml:"userAgent" json:"userAgent"`
}

// ConfigRateLimit is for rate limit settings
type ConfigRateLimit struct {
	Min   int           `yaml:"min" json:"min"`
	Retry time.Duration `yaml:"retry" json:"retry"`
}

// ConfigSync defines a source/target repository to sync
type ConfigSync struct {
	Source         string          `yaml:"source" json:"source"`
	Target         string          `yaml:"target" json:"target"`
	Type           string          `yaml:"type" json:"type"`
	Tags           ConfigTags      `yaml:"tags" json:"tags"`
	DigestTags     *bool           `yaml:"digestTags" json:"digestTags"`
	Platform       string          `yaml:"platform" json:"platform"`
	Platforms      []string        `yaml:"platforms" json:"platforms"`
	ForceRecursive *bool           `yaml:"forceRecursive" json:"forceRecursive"`
	Backup         string          `yaml:"backup" json:"backup"`
	Interval       time.Duration   `yaml:"interval" json:"interval"`
	Schedule       string          `yaml:"schedule" json:"schedule"`
	RateLimit      ConfigRateLimit `yaml:"ratelimit" json:"ratelimit"`
	MediaTypes     []string        `yaml:"mediaTypes" json:"mediaTypes"`
	Hooks          ConfigHooks     `yaml:"hooks" json:"hooks"`
}

// ConfigTags is an allow and deny list of tag regex strings
type ConfigTags struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny" json:"deny"`
}

// ConfigHooks for commands that run during the sync
type ConfigHooks struct {
	Pre       *ConfigHook `yaml:"pre" json:"pre"`
	Post      *ConfigHook `yaml:"post" json:"post"`
	Unchanged *ConfigHook `yaml:"unchanged" json:"unchanged"`
}

// ConfigHook identifies the hook type and params
type ConfigHook struct {
	Type   string   `yaml:"type" json:"type"`
	Params []string `yaml:"params" json:"params"`
}

// ConfigNew creates an empty configuration
func ConfigNew() *Config {
	c := Config{
		Creds: []ConfigCreds{},
		Sync:  []ConfigSync{},
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
	if c.Defaults.RateLimit.Retry < rateLimitRetryMin {
		c.Defaults.RateLimit.Retry = rateLimitRetryMin
	}
	// apply defaults to each step
	for i := range c.Sync {
		syncSetDefaults(&c.Sync[i], c.Defaults)
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
		val, err := template.String(c.Creds[i].Registry, nil)
		if err != nil {
			return err
		}
		c.Creds[i].Registry = val
		val, err = template.String(c.Creds[i].User, nil)
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
	for i := range c.Sync {
		val, err := template.String(c.Sync[i].Source, nil)
		if err != nil {
			return err
		}
		c.Sync[i].Source = val
		val, err = template.String(c.Sync[i].Target, nil)
		if err != nil {
			return err
		}
		c.Sync[i].Target = val
	}
	return nil
}

// updates sync entry with defaults
func syncSetDefaults(s *ConfigSync, d ConfigDefaults) {
	if s.Backup == "" && d.Backup != "" {
		s.Backup = d.Backup
	}
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
	if len(s.MediaTypes) == 0 {
		if len(d.MediaTypes) > 0 {
			s.MediaTypes = d.MediaTypes
		} else {
			s.MediaTypes = defaultMediaTypes
		}
	}
	if s.DigestTags == nil {
		b := (d.DigestTags != nil && *d.DigestTags)
		s.DigestTags = &b
	}
	if s.ForceRecursive == nil {
		b := (d.ForceRecursive != nil && *d.ForceRecursive)
		s.ForceRecursive = &b
	}
	if s.Hooks.Pre == nil && d.Hooks.Pre != nil {
		s.Hooks.Pre = d.Hooks.Pre
	}
	if s.Hooks.Post == nil && d.Hooks.Post != nil {
		s.Hooks.Post = d.Hooks.Post
	}
	if s.Hooks.Unchanged == nil && d.Hooks.Unchanged != nil {
		s.Hooks.Unchanged = d.Hooks.Unchanged
	}
}
