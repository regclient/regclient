package regsync

import "time"

type ActionType int

const (
	ActionCheck ActionType = iota
	ActionCopy
	ActionMissing
)

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
	Tags            TagAllowDeny           `yaml:"tags" json:"tags"`
	TagSets         []TagAllowDeny         `yaml:"tagSets" json:"tagSets"`
	Repos           RepoAllowDeny          `yaml:"repos" json:"repos"`
	DigestTags      *bool                  `yaml:"digestTags" json:"digestTags"`
	Referrers       *bool                  `yaml:"referrers" json:"referrers"`
	ReferrerFilters []ConfigReferrerFilter `yaml:"referrerFilters" json:"referrerFilters"`
	ReferrerSrc     string                 `yaml:"referrerSource" json:"referrerSource"`
	ReferrerTgt     string                 `yaml:"referrerTarget" json:"referrerTarget"`
	ReferrerSlow    *bool                  `yaml:"referrerSlow" json:"referrerSlow"`
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

// RepoAllowDeny is an allow and deny list of regex strings for repository names
type RepoAllowDeny struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny" json:"deny"`
}

// TagAllowDeny is an allow and deny list of regex strings for tags, with optional semver version range support
type TagAllowDeny struct {
	Allow       []string `yaml:"allow" json:"allow"`
	Deny        []string `yaml:"deny" json:"deny"`
	SemverRange []string `yaml:"semverRange,omitempty" json:"semverRange,omitempty"` // array of semver constraints, e.g., [">=1.0.0 <2.0.0", ">=4.0.0"]
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
