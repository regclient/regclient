package scheme

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/tag"
)

type SchemeAPI interface {
	// BlobDelete removes a blob from the repository
	BlobDelete(ctx context.Context, r ref.Ref, d digest.Digest) error
	// BlobGet retrieves a blob, returning a reader
	BlobGet(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error)
	// BlobHead verifies the existence of a blob, the reader contains the headers but no body to read
	BlobHead(ctx context.Context, r ref.Ref, d digest.Digest) (blob.Reader, error)
	// BlobMount attempts to perform a server side copy of the blob
	BlobMount(ctx context.Context, refSrc ref.Ref, refTgt ref.Ref, d digest.Digest) error
	// BlobPut sends a blob to the repository
	BlobPut(ctx context.Context, r ref.Ref, d digest.Digest, rdr io.Reader, cl int64) (digest.Digest, int64, error)

	// ManifestDelete removes a manifest, including all tags that point to that manifest
	ManifestDelete(ctx context.Context, r ref.Ref) error
	// ManifestGet retrieves a manifest from a repository
	ManifestGet(ctx context.Context, r ref.Ref) (manifest.Manifest, error)
	// ManifestHead gets metadata about the manifest (existence, digest, mediatype, size)
	ManifestHead(ctx context.Context, r ref.Ref) (manifest.Manifest, error)
	// ManifestPut sends a manifest to the repository
	ManifestPut(ctx context.Context, r ref.Ref, m manifest.Manifest) error

	// TagDelete removes a tag from the repository
	TagDelete(ctx context.Context, r ref.Ref) error
	// TagList returns a list of tags from the repository
	TagList(ctx context.Context, r ref.Ref, opts ...TagOpts) (*tag.TagList, error)
}

// RepoConfig is used for options to the repo functions
type RepoConfig struct {
	Limit int
	Last  string
}
type RepoOpts func(*RepoConfig)

func WithRepoLimit(l int) RepoOpts {
	return func(config *RepoConfig) {
		config.Limit = l
	}
}
func WithRepoLast(l string) RepoOpts {
	return func(config *RepoConfig) {
		config.Last = l
	}
}

type TagConfig struct {
	Limit int
	Last  string
}
type TagOpts func(*TagConfig)

func WithTagLimit(limit int) TagOpts {
	return func(t *TagConfig) {
		t.Limit = limit
	}
}
func WithTagLast(last string) TagOpts {
	return func(t *TagConfig) {
		t.Last = last
	}
}
