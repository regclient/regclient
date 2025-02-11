package main

import (
	"errors"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/mediatype"
)

// delay checking for at least 5 minutes when rate limit is exceeded
var rateLimitRetryMin = time.Minute * 5
var defaultMediaTypes = []string{
	mediatype.Docker2Manifest,
	mediatype.Docker2ManifestList,
	mediatype.OCI1Manifest,
	mediatype.OCI1ManifestList,
}

// Config is parsed configuration file for regsync
type Config struct {
	Version  int            `yaml:"version" json:"version"`
	Creds    []config.Host  `yaml:"creds" json:"creds"`
	Defaults ConfigDefaults `yaml:"defaults" json:"defaults"`
	Sync     []ConfigSync   `yaml:"sync" json:"sync"`
}

// ConfigDefaults is uses for general options and defaults for ConfigSync entries
type ConfigDefaults struct {
	Backup          string                 `yaml:"backup" json:"backup"`
	Interval        time.Duration          `yaml:"interval" json:"interval"`
	Schedule        string                 `yaml:"schedule" json:"schedule"`
	RateLimit       ConfigRateLimit        `yaml:"ratelimit" json:"ratelimit"`
	Parallel        int                    `yaml:"parallel" json:"parallel"`
	DigestTags      *bool                  `yaml:"digestTags" json:"digestTags"`
	Referrers       *bool                  `yaml:"referrers" json:"referrers"`
	ReferrerFilters []ConfigReferrerFilter `yaml:"referrerFilters" json:"referrerFilters"`
	ReferrerSrc     string                 `yaml:"referrerSource" json:"referrerSource"`
	ReferrerTgt     string                 `yaml:"referrerTarget" json:"referrerTarget"`
	FastCheck       *bool                  `yaml:"fastCheck" json:"fastCheck"`
	ForceRecursive  *bool                  `yaml:"forceRecursive" json:"forceRecursive"`
	IncludeExternal *bool                  `yaml:"includeExternal" json:"includeExternal"`
	MediaTypes      []string               `yaml:"mediaTypes" json:"mediaTypes"`
	Hooks           ConfigHooks            `yaml:"hooks" json:"hooks"`
	// general options
	BlobLimit      int64         `yaml:"blobLimit" json:"blobLimit"`
	CacheCount     int           `yaml:"cacheCount" json:"cacheCount"`
	CacheTime      time.Duration `yaml:"cacheTime" json:"cacheTime"`
	SkipDockerConf bool          `yaml:"skipDockerConfig" json:"skipDockerConfig"`
	UserAgent      string        `yaml:"userAgent" json:"userAgent"`
}

// ConfigRateLimit is for rate limit settings
type ConfigRateLimit struct {
	Min   int           `yaml:"min" json:"min"`
	Retry time.Duration `yaml:"retry" json:"retry"`
}

// ConfigSync defines a source/target repository to sync
type ConfigSync struct {
	Source          string                 `yaml:"source" json:"source"`
	Target          string                 `yaml:"target" json:"target"`
	Type            string                 `yaml:"type" json:"type"`
	Tags            AllowDeny              `yaml:"tags" json:"tags"`
	Repos           AllowDeny              `yaml:"repos" json:"repos"`
	DigestTags      *bool                  `yaml:"digestTags" json:"digestTags"`
	Referrers       *bool                  `yaml:"referrers" json:"referrers"`
	ReferrerFilters []ConfigReferrerFilter `yaml:"referrerFilters" json:"referrerFilters"`
	ReferrerSrc     string                 `yaml:"referrerSource" json:"referrerSource"`
	ReferrerTgt     string                 `yaml:"referrerTarget" json:"referrerTarget"`
	Platform        string                 `yaml:"platform" json:"platform"`
	Platforms       []string               `yaml:"platforms" json:"platforms"`
	FastCheck       *bool                  `yaml:"fastCheck" json:"fastCheck"`
	ForceRecursive  *bool                  `yaml:"forceRecursive" json:"forceRecursive"`
	IncludeExternal *bool                  `yaml:"includeExternal" json:"includeExternal"`
	Backup          string                 `yaml:"backup" json:"backup"`
	Interval        time.Duration          `yaml:"interval" json:"interval"`
	Schedule        string                 `yaml:"schedule" json:"schedule"`
	RateLimit       ConfigRateLimit        `yaml:"ratelimit" json:"ratelimit"`
	MediaTypes      []string               `yaml:"mediaTypes" json:"mediaTypes"`
	Hooks           ConfigHooks            `yaml:"hooks" json:"hooks"`
}

// AllowDeny is an allow and deny list of regex strings
type AllowDeny struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny" json:"deny"`
}

type ConfigReferrerFilter struct {
	ArtifactType string            `yaml:"artifactType" json:"artifactType"`
	Annotations  map[string]string `yaml:"annotations" json:"annotations"`
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
		Creds: []config.Host{},
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
	if c.Version == 0 {
		c.Version = 1
	}
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
		//#nosec G304 command is run by a user accessing their own files
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

// ConfigWrite outputs the processed config
func ConfigWrite(c *Config, w io.Writer) error {
	return yaml.NewEncoder(w).Encode(c)
}

// expand templates in various parts of the config
func configExpandTemplates(c *Config) error {
	dataSync := struct {
		Sync ConfigSync
	}{}
	for i := range c.Creds {
		val, err := template.String(c.Creds[i].Name, nil)
		if err != nil {
			return err
		}
		c.Creds[i].Name = val
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
		val, err = template.String(c.Creds[i].ClientCert, nil)
		if err != nil {
			return err
		}
		c.Creds[i].ClientCert = val
		val, err = template.String(c.Creds[i].ClientKey, nil)
		if err != nil {
			return err
		}
		c.Creds[i].ClientKey = val
	}
	for i := range c.Sync {
		dataSync.Sync = c.Sync[i]
		val, err := template.String(c.Sync[i].Source, dataSync)
		if err != nil {
			return err
		}
		c.Sync[i].Source = val
		dataSync.Sync.Source = val
		val, err = template.String(c.Sync[i].ReferrerSrc, dataSync)
		if err != nil {
			return err
		}
		c.Sync[i].ReferrerSrc = val
		dataSync.Sync.ReferrerSrc = val
		val, err = template.String(c.Sync[i].Target, dataSync)
		if err != nil {
			return err
		}
		c.Sync[i].Target = val
		val, err = template.String(c.Sync[i].ReferrerTgt, dataSync)
		if err != nil {
			return err
		}
		c.Sync[i].ReferrerTgt = val
		dataSync.Sync.ReferrerTgt = val
		// templates for Backup are expanded in each sync step
	}
	return nil
}

// updates sync entry with defaults
func syncSetDefaults(s *ConfigSync, d ConfigDefaults) {
	if s.Backup == "" && d.Backup != "" {
		s.Backup = d.Backup
	}
	if s.Schedule == "" && s.Interval == 0 {
		if d.Schedule != "" {
			s.Schedule = d.Schedule
		} else if d.Interval != 0 {
			s.Interval = d.Interval
		}
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
	if s.Referrers == nil {
		b := (d.Referrers != nil && *d.Referrers)
		s.Referrers = &b
	}
	if s.ReferrerFilters == nil {
		s.ReferrerFilters = d.ReferrerFilters
	}
	if s.ReferrerSrc == "" && d.ReferrerSrc != "" {
		s.ReferrerSrc = d.ReferrerSrc
	}
	if s.ReferrerTgt == "" && d.ReferrerTgt != "" {
		s.ReferrerTgt = d.ReferrerTgt
	}
	if s.FastCheck == nil {
		b := (d.FastCheck != nil && *d.FastCheck)
		s.FastCheck = &b
	}
	if s.ForceRecursive == nil {
		b := (d.ForceRecursive != nil && *d.ForceRecursive)
		s.ForceRecursive = &b
	}
	if s.IncludeExternal == nil {
		b := (d.IncludeExternal != nil && *d.IncludeExternal)
		s.IncludeExternal = &b
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
