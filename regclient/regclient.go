//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the top level regclient package

package regclient

import (
	rcTop "github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/repo"
	"github.com/regclient/regclient/types/tag"
)

const (
	DefaultBlobChunk   = reg.DefaultBlobChunk
	DefaultBlobMax     = reg.DefaultBlobMax
	DefaultRetryLimit  = reghttp.DefaultRetryLimit
	DefaultUserAgent   = rcTop.DefaultUserAgent
	DockerCertDir      = rcTop.DockerCertDir
	DockerRegistry     = config.DockerRegistry
	DockerRegistryAuth = config.DockerRegistryAuth
	DockerRegistryDNS  = config.DockerRegistryDNS
)

var (
	// VCSRef is injected from a build flag, used to version the UserAgent header
	VCSRef = "unknown"
)

type RegClient = *rcTop.RegClient
type Client = *rcTop.RegClient
type Opt = rcTop.Opt
type ImageOpts = rcTop.ImageOpts
type RepoList = *repo.RepoList
type RepoDockerList = repo.RepoRegistryList
type RepoOpts = scheme.RepoOpts // RepoOpts is a breaking change (struct to func opts)
type TagList = *tag.TagList
type TagDockerList = tag.TagDockerList
type TagOpts = scheme.TagOpts

var (
	NewRegClient            = rcTop.New
	WithCertDir             = rcTop.WithCertDir
	WithDockerCerts         = rcTop.WithDockerCerts
	WithDockerCreds         = rcTop.WithDockerCreds
	WithConfigHosts         = rcTop.WithConfigHosts
	WithConfigHost          = rcTop.WithConfigHost
	WithBlobSize            = rcTop.WithBlobSize
	WithLog                 = rcTop.WithLog
	WithRetryDelay          = rcTop.WithRetryDelay
	WithRetryLimit          = rcTop.WithRetryLimit
	WithUserAgent           = rcTop.WithUserAgent
	ImageWithForceRecursive = rcTop.ImageWithForceRecursive
	ImageWithDigestTags     = rcTop.ImageWithDigestTags
	ImageWithPlatforms      = rcTop.ImageWithPlatforms
	WithRepoLast            = scheme.WithRepoLast
	WithRepoLimit           = scheme.WithRepoLimit
	TagOptLast              = scheme.WithTagLast
	TagOptLimit             = scheme.WithTagLimit
)
