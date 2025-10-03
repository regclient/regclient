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
	//go:fix inline
	RegClient = *rcTop.RegClient
	// Client is used to access OCI distribution-spec registries.
	//
	// Deprecated: replace with [regclient.RegClient]
	//go:fix inline
	Client = *rcTop.RegClient
	// Opt functions are used by [New] to create a [*RegClient].
	//
	// Deprecated: replace with [regclient.Opt]
	//go:fix inline
	Opt = rcTop.Opt
	// ImageOpts functions are used by image methods.
	//
	// Deprecated: replace with [regclient.ImageOpts]
	//go:fix inline
	ImageOpts = rcTop.ImageOpts
	// RepoList is the response for a registry listing.
	//
	// Deprecated: replace with [repo.RepoList]
	//go:fix inline
	RepoList = *repo.RepoList
	// RepoDockerList is a list of repositories from the _catalog API
	//
	// Deprecated: replace with [repo.RepoRegistryList
	//go:fix inline
	RepoDockerList = repo.RepoRegistryList
	// RepoOpts is a breaking change (struct to func opts)
	//
	// Deprecated: replace with [regclient.RepoOpts]
	//go:fix inline
	RepoOpts = scheme.RepoOpts
	// TagList contains a tag list.
	//
	// Deprecated: replace with [tag.List]
	//go:fix inline
	TagList = *tag.List
	// TagDockerList is returned from registry/2.0 API's
	//
	// Deprecated: replace with [tag.DockerList]
	//go:fix inline
	TagDockerList = tag.DockerList
	// TagOpts is used to set options on tag APIs.
	//
	// Deprecated: replace with [scheme.TagOpts]
	//go:fix inline
	TagOpts = scheme.TagOpts
)

var (
	// NewRegClient creates a new regclient instance.
	//
	// Deprecated: replace with [regclient.New].
	//go:fix inline
	NewRegClient = rcTop.New
	// WithCertDir adds a path of certificates to trust similar to Docker's /etc/docker/certs.d.
	//
	// Deprecated: replace with [regclient.WithCertDir].
	//go:fix inline
	WithCertDir = rcTop.WithCertDir
	// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d.
	//
	// Deprecated: replace with [regclient.WithDockerCerts].
	//go:fix inline
	WithDockerCerts = rcTop.WithDockerCerts
	// WithDockerCreds adds configuration from users docker config with registry logins.
	//
	// Deprecated: replace with [regclient.WithDockerCreds].
	//go:fix inline
	WithDockerCreds = rcTop.WithDockerCreds
	// WithConfigHosts adds a list of config host settings.
	//
	// Deprecated: replace with [regclient.WithConfigHosts].
	//go:fix inline
	WithConfigHosts = rcTop.WithConfigHosts
	// WithConfigHost adds a list of config host settings.
	//
	// Deprecated: replace with [regclient.WithConfigHost].
	//go:fix inline
	WithConfigHost = rcTop.WithConfigHost
	// WithBlobSize overrides default blob sizes.
	//
	// Deprecated: replace with [regclient.WithBlobSize].
	//go:fix inline
	WithBlobSize = rcTop.WithBlobSize
	// WithLog overrides default logrus Logger.
	//
	// Deprecated: replace with [regclient.WithLog].
	//go:fix inline
	WithLog = rcTop.WithLog
	// WithRetryDelay specifies the time permitted for retry delays.
	//
	// Deprecated: replace with [regclient.WithRetryDelay].
	//go:fix inline
	WithRetryDelay = rcTop.WithRetryDelay
	// WithRetryLimit specifies the number of retries for non-fatal errors.
	//
	// Deprecated: replace with [regclient.WithRetryLimit].
	//go:fix inline
	WithRetryLimit = rcTop.WithRetryLimit
	// WithUserAgent specifies the User-Agent http header.
	//
	// Deprecated: replace with [regclient.WithUserAgent].
	//go:fix inline
	WithUserAgent = rcTop.WithUserAgent
	// ImageWithForceRecursive attempts to copy every manifest and blob even if parent manifests already exist in ImageCopy.
	//
	// Deprecated: replace with [regclient.ImageWithForceRecursive].
	//go:fix inline
	ImageWithForceRecursive = rcTop.ImageWithForceRecursive
	// ImageWithDigestTags looks for "sha-<digest>.*" tags in the repo to copy with any manifest in ImageCopy.
	//
	// Deprecated: replace with [regclient.ImageWithDigestTags].
	//go:fix inline
	ImageWithDigestTags = rcTop.ImageWithDigestTags
	// ImageWithPlatform requests specific platforms from a manifest list in ImageCheckBase.
	//
	// Deprecated: replace with [regclient.ImageWithPlatforms].
	//go:fix inline
	ImageWithPlatforms = rcTop.ImageWithPlatforms
	// WithRepoLast passes the last received repository for requesting the next batch of repositories.
	//
	// Deprecated: replace with [scheme.WithRepoLast].
	//go:fix inline
	WithRepoLast = scheme.WithRepoLast
	// WithRepoLimit passes a maximum number of repositories to return to the repository list API.
	//
	// Deprecated: replace with [scheme.WithRepoLimit].
	//go:fix inline
	WithRepoLimit = scheme.WithRepoLimit
	// TagOptLast passes the last received tag for requesting the next batch of tags.
	//
	// Deprecated: replace with [scheme.WithTagLast].
	//go:fix inline
	TagOptLast = scheme.WithTagLast
	// TagOptLimit passes a maximum number of tags to return to the tag list API.
	//
	// Deprecated: replace with [scheme.WithTagLimit].
	//go:fix inline
	TagOptLimit = scheme.WithTagLimit
)
