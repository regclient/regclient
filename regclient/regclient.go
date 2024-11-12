//go:build legacy
// +build legacy

//lint:file-ignore SA1019 Ignore deprecations since this entire package is deprecated

// Package regclient is a legacy package, this has been moved to the top level regclient package
package regclient

import (
	rcTop "github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/reghttp"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/repo"
	"github.com/regclient/regclient/types/tag"
)

const (
	DefaultBlobChunk   = 1024 * 1024
	DefaultBlobMax     = -1
	DefaultRetryLimit  = reghttp.DefaultRetryLimit
	DefaultUserAgent   = rcTop.DefaultUserAgent
	DockerCertDir      = rcTop.DockerCertDir
	DockerRegistry     = config.DockerRegistry
	DockerRegistryAuth = config.DockerRegistryAuth
	DockerRegistryDNS  = config.DockerRegistryDNS
)

var (
	// VCSRef is injected from a build flag, used to version the UserAgent header
	//
	// Deprecated: this should now be set using github.com/regclient/regclient/internal/version.vcsTag.
	VCSRef = "unknown"
)

type (
	// RegClient is used to access OCI distribution-spec registries.
	//
	// Deprecated: replace with [regclient.RegClient]
	RegClient = *rcTop.RegClient
	// Client is used to access OCI distribution-spec registries.
	//
	// Deprecated: replace with [regclient.RegClient]
	Client = *rcTop.RegClient
	// Opt functions are used by [New] to create a [*RegClient].
	//
	// Deprecated: replace with [regclient.Opt]
	Opt = rcTop.Opt
	// ImageOpts functions are used by image methods.
	//
	// Deprecated: replace with [regclient.ImageOpts]
	ImageOpts = rcTop.ImageOpts
	// RepoList is the response for a registry listing.
	//
	// Deprecated: replace with [repo.RepoList]
	RepoList = *repo.RepoList
	// RepoDockerList is a list of repositories from the _catalog API
	//
	// Deprecated: replace with [repo.RepoRegistryList
	RepoDockerList = repo.RepoRegistryList
	// RepoOpts is a breaking change (struct to func opts)
	//
	// Deprecated: replace with [regclient.RepoOpts]
	RepoOpts = scheme.RepoOpts
	// TagList contains a tag list.
	//
	// Deprecated: replace with [tag.List]
	TagList = *tag.List
	// TagDockerList is returned from registry/2.0 API's
	//
	// Deprecated: replace with [tag.DockerList]
	TagDockerList = tag.DockerList
	// TagOpts is used to set options on tag APIs.
	//
	// Deprecated: replace with [scheme.TagOpts]
	TagOpts = scheme.TagOpts
)

var (
	// NewRegClient creates a new regclient instance.
	//
	// Deprecated: replace with [regclient.New].
	NewRegClient = rcTop.New
	// WithCertDir adds a path of certificates to trust similar to Docker's /etc/docker/certs.d.
	//
	// Deprecated: replace with [regclient.WithCertDir].
	WithCertDir = rcTop.WithCertDir
	// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d.
	//
	// Deprecated: replace with [regclient.WithDockerCerts].
	WithDockerCerts = rcTop.WithDockerCerts
	// WithDockerCreds adds configuration from users docker config with registry logins.
	//
	// Deprecated: replace with [regclient.WithDockerCreds].
	WithDockerCreds = rcTop.WithDockerCreds
	// WithConfigHosts adds a list of config host settings.
	//
	// Deprecated: replace with [regclient.WithConfigHosts].
	WithConfigHosts = rcTop.WithConfigHosts
	// WithConfigHost adds a list of config host settings.
	//
	// Deprecated: replace with [regclient.WithConfigHost].
	WithConfigHost = rcTop.WithConfigHost
	// WithBlobSize overrides default blob sizes.
	//
	// Deprecated: replace with [regclient.WithBlobSize].
	WithBlobSize = rcTop.WithBlobSize
	// WithLog overrides default logrus Logger.
	//
	// Deprecated: replace with [regclient.WithLog].
	WithLog = rcTop.WithLog
	// WithRetryDelay specifies the time permitted for retry delays.
	//
	// Deprecated: replace with [regclient.WithRetryDelay].
	WithRetryDelay = rcTop.WithRetryDelay
	// WithRetryLimit specifies the number of retries for non-fatal errors.
	//
	// Deprecated: replace with [regclient.WithRetryLimit].
	WithRetryLimit = rcTop.WithRetryLimit
	// WithUserAgent specifies the User-Agent http header.
	//
	// Deprecated: replace with [regclient.WithUserAgent].
	WithUserAgent = rcTop.WithUserAgent
	// ImageWithForceRecursive attempts to copy every manifest and blob even if parent manifests already exist in ImageCopy.
	//
	// Deprecated: replace with [regclient.ImageWithForceRecursive].
	ImageWithForceRecursive = rcTop.ImageWithForceRecursive
	// ImageWithDigestTags looks for "sha-<digest>.*" tags in the repo to copy with any manifest in ImageCopy.
	//
	// Deprecated: replace with [regclient.ImageWithDigestTags].
	ImageWithDigestTags = rcTop.ImageWithDigestTags
	// ImageWithPlatform requests specific platforms from a manifest list in ImageCheckBase.
	//
	// Deprecated: replace with [regclient.ImageWithPlatforms].
	ImageWithPlatforms = rcTop.ImageWithPlatforms
	// WithRepoLast passes the last received repository for requesting the next batch of repositories.
	//
	// Deprecated: replace with [scheme.WithRepoLast].
	WithRepoLast = scheme.WithRepoLast
	// WithRepoLimit passes a maximum number of repositories to return to the repository list API.
	//
	// Deprecated: replace with [scheme.WithRepoLimit].
	WithRepoLimit = scheme.WithRepoLimit
	// TagOptLast passes the last received tag for requesting the next batch of tags.
	//
	// Deprecated: replace with [scheme.WithTagLast].
	TagOptLast = scheme.WithTagLast
	// TagOptLimit passes a maximum number of tags to return to the tag list API.
	//
	// Deprecated: replace with [scheme.WithTagLimit].
	TagOptLimit = scheme.WithTagLimit
)
