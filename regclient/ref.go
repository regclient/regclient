package regclient

import "github.com/docker/distribution/reference"

// Ref reference to a registry/repository
// If the tag or digest is available, it's also included in the reference.
// Reference itself is the unparsed string.
// While this is currently a struct, that may change in the future and access
// to contents should not be assumed/used.
type Ref struct {
	Reference, Registry, Repository, Tag, Digest string
}

// NewRef returns a repository reference including a registry, repository (path), digest, and tag
func NewRef(ref string) (Ref, error) {
	parsed, err := reference.ParseNormalizedNamed(ref)

	var ret Ref
	ret.Reference = ref

	if err != nil {
		return ret, err
	}

	ret.Registry = reference.Domain(parsed)
	ret.Repository = reference.Path(parsed)

	if canonical, ok := parsed.(reference.Canonical); ok {
		ret.Digest = canonical.Digest().String()
	}

	if tagged, ok := parsed.(reference.Tagged); ok {
		ret.Tag = tagged.Tag()
	}

	if ret.Tag == "" && ret.Digest == "" {
		ret.Tag = "latest"
	}

	return ret, nil
}

// CommonName outputs a parsable name from a reference
func (r Ref) CommonName() string {
	cn := ""
	if r.Registry != "" {
		cn = r.Registry + "/"
	}
	if r.Repository == "" {
		return ""
	}
	cn = cn + r.Repository
	if r.Tag != "" {
		cn = cn + ":" + r.Tag
	}
	if r.Digest != "" {
		cn = cn + "@" + r.Digest
	}
	return cn
}
