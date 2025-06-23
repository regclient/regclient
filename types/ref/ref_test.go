package ref

import (
	"errors"
	"strings"
	"testing"

	"github.com/regclient/regclient/types/errs"
)

var testDigest = "sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115"

func TestNew(t *testing.T) {
	t.Parallel()
	var tt = []struct {
		name       string
		ref        string
		scheme     string
		registry   string
		repository string
		tag        string
		digest     string
		path       string
		wantE      error
	}{
		{
			name:       "Docker library",
			ref:        "alpine",
			scheme:     "reg",
			registry:   "docker.io",
			repository: "library/alpine",
			tag:        "latest",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Docker add library",
			ref:        "docker.io/alpine",
			scheme:     "reg",
			registry:   "docker.io",
			repository: "library/alpine",
			tag:        "latest",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Docker project",
			ref:        "regclient/regctl:edge",
			scheme:     "reg",
			registry:   "docker.io",
			repository: "regclient/regctl",
			tag:        "edge",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Docker legacy",
			ref:        "index.docker.io/library/alpine",
			scheme:     "reg",
			registry:   "docker.io",
			repository: "library/alpine",
			tag:        "latest",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Docker DNS",
			ref:        "registry-1.docker.io/library/alpine",
			scheme:     "reg",
			registry:   "docker.io",
			repository: "library/alpine",
			tag:        "latest",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Private registry",
			ref:        "example.com/group/image:v42",
			scheme:     "reg",
			registry:   "example.com",
			repository: "group/image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Uppercase registry",
			ref:        "EXAMPLE/group-image:V42",
			scheme:     "reg",
			registry:   "EXAMPLE",
			repository: "group-image",
			tag:        "V42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Uppercase registry before dash",
			ref:        "Example-1/image:1.0-BETA",
			scheme:     "reg",
			registry:   "Example-1",
			repository: "image",
			tag:        "1.0-BETA",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Uppercase registry after dash",
			ref:        "example-A/group/image:v42",
			scheme:     "reg",
			registry:   "example-A",
			repository: "group/image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "explicit short name",
			ref:        "example./g/roup/image:a",
			scheme:     "reg",
			registry:   "example.",
			repository: "g/roup/image",
			tag:        "a",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "explicit short name with port",
			ref:        "example.:5000/g/roup/image:a",
			scheme:     "reg",
			registry:   "example.:5000",
			repository: "g/roup/image",
			tag:        "a",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "explicit fqdn with port",
			ref:        "example.com:5000/g/roup/image:a",
			scheme:     "reg",
			registry:   "example.com:5000",
			repository: "g/roup/image",
			tag:        "a",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "single character names",
			ref:        "e.xample.co/g/roup/image:a",
			scheme:     "reg",
			registry:   "e.xample.co",
			repository: "g/roup/image",
			tag:        "a",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "separators in repo",
			ref:        "e.xample.co/g-r.o__u_p/im----age:a",
			scheme:     "reg",
			registry:   "e.xample.co",
			repository: "g-r.o__u_p/im----age",
			tag:        "a",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "separators in tag",
			ref:        "e.xample.co/g/roup/image:__a--b..5__",
			scheme:     "reg",
			registry:   "e.xample.co",
			repository: "g/roup/image",
			tag:        "__a--b..5__",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Localhost registry",
			ref:        "localhost/group/image:v42",
			scheme:     "reg",
			registry:   "localhost",
			repository: "group/image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Localhost registry with port",
			ref:        "localhost:5000/group/image:v42",
			scheme:     "reg",
			registry:   "localhost:5000",
			repository: "group/image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "ip address registry",
			ref:        "127.0.0.1:5000/image:v42",
			scheme:     "reg",
			registry:   "127.0.0.1:5000",
			repository: "image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "ipv6 address registry",
			ref:        "[fddd:1222:1222:1222::ac29:5a6a]:5000/image:v42",
			scheme:     "reg",
			registry:   "[fddd:1222:1222:1222::ac29:5a6a]:5000",
			repository: "image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "ipv6 address registry without port",
			ref:        "[fddd:1222:1222:1222::ac29:5a6a]/image:v42",
			scheme:     "reg",
			registry:   "[fddd:1222:1222:1222::ac29:5a6a]",
			repository: "image",
			tag:        "v42",
			digest:     "",
			path:       "",
			wantE:      nil,
		},
		{
			name:       "Port registry digest",
			ref:        "registry:5000/group/image@" + testDigest,
			scheme:     "reg",
			registry:   "registry:5000",
			repository: "group/image",
			tag:        "",
			digest:     testDigest,
			path:       "",
			wantE:      nil,
		},
		{
			name:       "OCI file",
			ref:        "ocifile://path/to/file.tgz",
			scheme:     "ocifile",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     "",
			path:       "path/to/file.tgz",
			wantE:      nil,
		},
		{
			name:       "OCI file with tag",
			ref:        "ocifile://path/to/file.tgz:v1.2.3",
			scheme:     "ocifile",
			registry:   "",
			repository: "",
			tag:        "v1.2.3",
			digest:     "",
			path:       "path/to/file.tgz",
			wantE:      nil,
		},
		{
			name:       "OCI file with digest",
			ref:        "ocifile://path/to/file.tgz@" + testDigest,
			scheme:     "ocifile",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     testDigest,
			path:       "path/to/file.tgz",
			wantE:      nil,
		},
		{
			name:  "OCI file with invalid digest",
			ref:   "ocifile://path/to/file.tgz@sha256:ZZ15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115ZZ",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:       "OCI dir",
			ref:        "ocidir://path/to/dir",
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     "",
			path:       "path/to/dir",
			wantE:      nil,
		},
		{
			name:       "OCI dir with tag",
			ref:        "ocidir://path/to/DIR:v1.2.3",
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "v1.2.3",
			digest:     "",
			path:       "path/to/DIR",
			wantE:      nil,
		},
		{
			name:       "OCI dir with digest",
			ref:        "ocidir://path/2/dir@" + testDigest,
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     testDigest,
			path:       "path/2/dir",
			wantE:      nil,
		},
		{
			name:       "OCI dir with tilde",
			ref:        "ocidir://path/2/~dir~/+rules_oci+/examples:latest",
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "latest",
			digest:     "",
			path:       "path/2/~dir~/+rules_oci+/examples",
			wantE:      nil,
		},
		{
			name:  "invalid scheme",
			ref:   "unknown://repo:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid host leading dash",
			ref:   "-docker.io/project/image:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid host trailing dash",
			ref:   "docker-.io/project/image:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid repo case",
			ref:   "docker.io/Upper/Case/Repo:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid repo dash leading",
			ref:   "project/-image:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid repo dash trailing",
			ref:   "project/image-:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid repo triple underscore",
			ref:   "project/image___x:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid repo chars",
			ref:   "project/star*:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid tag chars",
			ref:   "project/image:tag^1",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid tag length",
			ref:   "project/image:" + strings.Repeat("x", 129),
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid short digest",
			ref:   "project/image@sha256:12345",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid digest characters",
			ref:   "project/image@sha256:gggg40677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid ocidir path",
			ref:   "ocidir://invalid*filename:tag",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid ocidir tag",
			ref:   "ocidir://filename:tag=fail",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "invalid ocidir digest",
			ref:   "ocidir://filename@sha256:abcd",
			wantE: errs.ErrInvalidReference,
		},
		{
			name:  "localhost missing repo",
			ref:   "localhost:5000",
			wantE: errs.ErrInvalidReference,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r, err := New(tc.ref)
			if tc.wantE != nil {
				if err == nil {
					t.Errorf("error not received, expected %v", tc.wantE)
				} else if !errors.Is(err, tc.wantE) && err.Error() != tc.wantE.Error() {
					t.Errorf("expected error not received, expected %v, received %v", tc.wantE, err)
				}
				return
			} else if err != nil {
				t.Fatalf("failed creating reference, err: %v", err)
			}
			if tc.ref != r.Reference {
				t.Errorf("reference mismatch for %s, received %s", tc.ref, r.Reference)
			}
			if tc.scheme != r.Scheme {
				t.Errorf("scheme mismatch for %s, expected %s, received %s", tc.ref, tc.scheme, r.Scheme)
			}
			if tc.registry != r.Registry {
				t.Errorf("registry mismatch for %s, expected %s, received %s", tc.ref, tc.registry, r.Registry)
			}
			if tc.repository != r.Repository {
				t.Errorf("repository mismatch for %s, expected %s, received %s", tc.ref, tc.repository, r.Repository)
			}
			if tc.tag != r.Tag {
				t.Errorf("tag mismatch for %s, expected %s, received %s", tc.ref, tc.tag, r.Tag)
			}
			if tc.digest != r.Digest {
				t.Errorf("digest mismatch for %s, expected %s, received %s", tc.ref, tc.digest, r.Digest)
			}
			if tc.path != r.Path {
				t.Errorf("path mismatch for %s, expected %s, received %s", tc.ref, tc.path, r.Path)
			}
		})
	}
}

func TestNewHost(t *testing.T) {
	t.Parallel()
	var tt = []struct {
		name     string
		host     string
		scheme   string
		registry string
		path     string
		wantE    error
	}{
		{
			name:  "empty string",
			host:  "",
			wantE: errs.ErrParsingFailed,
		},
		{
			name:     "Docker Hub",
			host:     "docker.io",
			scheme:   "reg",
			registry: "docker.io",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "example.com",
			host:     "example.com",
			scheme:   "reg",
			registry: "example.com",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "Uppercase registry",
			host:     "EXAMPLE",
			scheme:   "reg",
			registry: "EXAMPLE",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "explicit short name",
			host:     "example.",
			scheme:   "reg",
			registry: "example.",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "short name with port",
			host:     "example:5000",
			scheme:   "reg",
			registry: "example:5000",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "fqdn with port",
			host:     "example.com:5000",
			scheme:   "reg",
			registry: "example.com:5000",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "Localhost",
			host:     "localhost",
			scheme:   "reg",
			registry: "localhost",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "Localhost with port",
			host:     "localhost:5000",
			scheme:   "reg",
			registry: "localhost:5000",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "ip address registry",
			host:     "127.0.0.1:5000",
			scheme:   "reg",
			registry: "127.0.0.1:5000",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "ipv6 address registry",
			host:     "[fddd:1222:1222:1222::ac29:5a6a]:5000",
			scheme:   "reg",
			registry: "[fddd:1222:1222:1222::ac29:5a6a]:5000",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "ipv6 address registry without port",
			host:     "[fddd:1222:1222:1222::ac29:5a6a]",
			scheme:   "reg",
			registry: "[fddd:1222:1222:1222::ac29:5a6a]",
			path:     "",
			wantE:    nil,
		},
		{
			name:     "OCI file",
			host:     "ocifile://path",
			scheme:   "ocifile",
			registry: "",
			path:     "path",
			wantE:    nil,
		},
		{
			name:     "OCI file with tag",
			host:     "ocifile://path/to/file.tgz:v1.2.3",
			scheme:   "ocifile",
			registry: "",
			path:     "path/to/file.tgz",
			wantE:    nil,
		},
		{
			name:     "OCI file with digest",
			host:     "ocifile://path/to/file.tgz@" + testDigest,
			scheme:   "ocifile",
			registry: "",
			path:     "path/to/file.tgz",
			wantE:    nil,
		},
		{
			name:  "OCI file with invalid digest",
			host:  "ocifile://path/to/file.tgz@sha256:ZZ15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115ZZ",
			wantE: errs.ErrParsingFailed,
		},
		{
			name:     "OCI dir",
			host:     "ocidir://path/to/dir",
			scheme:   "ocidir",
			registry: "",
			path:     "path/to/dir",
			wantE:    nil,
		},
		{
			name:     "OCI dir with tag",
			host:     "ocidir://path/to/DIR:v1.2.3",
			scheme:   "ocidir",
			registry: "",
			path:     "path/to/DIR",
			wantE:    nil,
		},
		{
			name:     "OCI dir with digest",
			host:     "ocidir://path/2/dir@" + testDigest,
			scheme:   "ocidir",
			registry: "",
			path:     "path/2/dir",
			wantE:    nil,
		},
		{
			name:  "invalid scheme",
			host:  "unknown://repo:tag",
			wantE: errs.ErrParsingFailed,
		},
		{
			name:  "invalid host leading dash",
			host:  "-docker.io",
			wantE: errs.ErrParsingFailed,
		},
		{
			name:  "invalid host trailing dash",
			host:  "docker-.io",
			wantE: errs.ErrParsingFailed,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewHost(tc.host)
			if tc.wantE != nil {
				if err == nil {
					t.Errorf("error not received, expected %v", tc.wantE)
				} else if !errors.Is(err, tc.wantE) && err.Error() != tc.wantE.Error() {
					t.Errorf("expected error not received, expected %v, received %v", tc.wantE, err)
				}
				return
			} else if err != nil {
				t.Fatalf("failed creating reference, err: %v", err)
			}
			if r.Scheme == "reg" && r.IsSet() {
				t.Errorf("isSet unexpected for %s, expected %t, received %t", tc.host, false, r.IsSet())
			}
			if tc.scheme != r.Scheme {
				t.Errorf("scheme mismatch for %s, expected %s, received %s", tc.host, tc.scheme, r.Scheme)
			}
			if tc.registry != r.Registry {
				t.Errorf("registry mismatch for %s, expected %s, received %s", tc.host, tc.registry, r.Registry)
			}
			if tc.path != r.Path {
				t.Errorf("path mismatch for %s, expected %s, received %s", tc.host, tc.path, r.Path)
			}
		})
	}
}

func TestCommon(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name string
		str  string
	}{
		{
			name: "ref with tag",
			str:  "docker.io/group/image:tag",
		},
		{
			name: "ref with digest",
			str:  "docker.io/group/image@" + testDigest,
		},
		{
			name: "ocidir with tag",
			str:  "ocidir:///tmp/image:tag",
		},
		{
			name: "ocidir with digest",
			str:  "ocidir://image@" + testDigest,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r, err := New(tc.str)
			if err != nil {
				t.Fatalf("failed to parse %s: %v", tc.str, err)
			}
			cn := r.CommonName()
			if tc.str != cn {
				t.Errorf("common name mismatch, input %s, output %s", tc.str, cn)
			}
		})
	}
}

func TestEqual(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name       string
		a, b       Ref
		expectReg  bool
		expectRepo bool
	}{
		{
			name: "ref eq reg/repo",
			a: Ref{
				Scheme:     "reg",
				Registry:   "host:5000",
				Repository: "repo",
				Tag:        "a",
			},
			b: Ref{
				Scheme:     "reg",
				Registry:   "host:5000",
				Repository: "repo",
				Tag:        "b",
			},
			expectReg:  true,
			expectRepo: true,
		},
		{
			name: "ref eq reg",
			a: Ref{
				Scheme:     "reg",
				Registry:   "host:5000",
				Repository: "repo-a",
				Tag:        "a",
			},
			b: Ref{
				Scheme:     "reg",
				Registry:   "host:5000",
				Repository: "repo-b",
				Tag:        "b",
			},
			expectReg:  true,
			expectRepo: false,
		},
		{
			name: "ref ne reg",
			a: Ref{
				Scheme:     "reg",
				Registry:   "host:5000",
				Repository: "repo-a",
				Tag:        "a",
			},
			b: Ref{
				Scheme:     "reg",
				Registry:   "host:5001",
				Repository: "repo-b",
				Tag:        "b",
			},
			expectReg:  false,
			expectRepo: false,
		},
		{
			name: "ocidir eq file",
			a: Ref{
				Scheme: "ocidir",
				Path:   "path/to/file",
				Tag:    "a",
			},
			b: Ref{
				Scheme: "ocidir",
				Path:   "path/to/file",
				Tag:    "b",
			},
			expectReg:  true,
			expectRepo: true,
		},
		{
			name: "ocidir ne file",
			a: Ref{
				Scheme: "ocidir",
				Path:   "path/to/file-a",
				Tag:    "a",
			},
			b: Ref{
				Scheme: "ocidir",
				Path:   "path/to/file-b",
				Tag:    "b",
			},
			expectReg:  false,
			expectRepo: false,
		},
		{
			name: "ne scheme",
			a: Ref{
				Scheme:     "reg",
				Registry:   "host:5000",
				Repository: "repo-a",
				Path:       "path/to/file-b",
				Tag:        "a",
			},
			b: Ref{
				Scheme:     "ocidir",
				Registry:   "host:5000",
				Repository: "repo-a",
				Path:       "path/to/file-b",
				Tag:        "b",
			},
			expectReg:  false,
			expectRepo: false,
		},
		{
			name: "unknown scheme",
			a: Ref{
				Scheme:     "unknown",
				Registry:   "host:5000",
				Repository: "repo-a",
				Tag:        "a",
			},
			b: Ref{
				Scheme:     "unknown",
				Registry:   "host:5000",
				Repository: "repo-a",
				Tag:        "a",
			},
			expectReg:  false,
			expectRepo: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if EqualRegistry(tc.a, tc.b) != tc.expectReg {
				t.Errorf("equal registry was not %v for %s and %s", tc.expectReg, tc.a.CommonName(), tc.b.CommonName())
			}
			if EqualRepository(tc.a, tc.b) != tc.expectRepo {
				t.Errorf("equal repository was not %v for %s and %s", tc.expectRepo, tc.a.CommonName(), tc.b.CommonName())
			}
		})
	}
}

func TestIsSet(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name      string
		ref       Ref
		isSet     bool
		isSetRepo bool
		isZero    bool
	}{
		{
			name:   "zero",
			isZero: true,
		},
		{
			name: "no scheme",
			ref: Ref{
				Tag: "latest",
			},
		},
		{
			name: "unknown scheme",
			ref: Ref{
				Scheme: "unknown",
				Tag:    "latest",
			},
		},
		{
			name: "no repo",
			ref: Ref{
				Scheme:   "reg",
				Registry: "docker.io",
				Tag:      "latest",
			},
		},
		{
			name: "reg no tag",
			ref: Ref{
				Scheme:     "reg",
				Registry:   "docker.io",
				Repository: "library/alpine",
			},
			isSetRepo: true,
		},
		{
			name: "no path",
			ref: Ref{
				Scheme: "ocidir",
				Tag:    "latest",
			},
		},
		{
			name: "ocidir no tag",
			ref: Ref{
				Scheme: "ocidir",
				Path:   ".",
			},
			isSetRepo: true,
			isSet:     true,
		},
		{
			name: "reg with digest",
			ref: Ref{
				Scheme:     "reg",
				Registry:   "docker.io",
				Repository: "library/alpine",
				Digest:     testDigest,
			},
			isSetRepo: true,
			isSet:     true,
		},
		{
			name: "reg with tag",
			ref: Ref{
				Scheme:     "reg",
				Registry:   "docker.io",
				Repository: "library/alpine",
				Tag:        "latest",
			},
			isSetRepo: true,
			isSet:     true,
		},
		{
			name: "ocidir",
			ref: Ref{
				Scheme: "ocidir",
				Path:   ".",
				Tag:    "latest",
			},
			isSetRepo: true,
			isSet:     true,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ref.IsSetRepo() != tc.isSetRepo {
				t.Errorf("hasRepo is not %t", tc.isSetRepo)
			}
			if tc.ref.IsSet() != tc.isSet {
				t.Errorf("isSet is not %t", tc.isSet)
			}
			if tc.ref.IsZero() != tc.isZero {
				t.Errorf("isZero is not %t", tc.isSet)
			}
		})
	}
}

func TestSetAndAdd(t *testing.T) {
	t.Parallel()
	rStr := "example.com/repo:v1"
	rDigStr := "example.com/repo@" + testDigest
	rTagStr := "example.com/repo:v2"
	rAddStr := "example.com/repo:v2@" + testDigest
	r, err := New(rStr)
	if err != nil {
		t.Fatalf("unexpected parse failure: %v", err)
	}
	r = r.SetDigest(testDigest)
	if r.Tag != "" {
		t.Errorf("SetDigest tag mismatch, expected empty string, received %s", r.Tag)
	}
	if r.Digest != testDigest {
		t.Errorf("SetDigest digest mismatch, expected %s, received %s", testDigest, r.Digest)
	}
	if r.Reference != rDigStr {
		t.Errorf("SetDigest reference mismatch, expected %s, received %s", rDigStr, r.Reference)
	}
	r = r.SetTag("v2")
	if r.Tag != "v2" {
		t.Errorf("SetTag tag mismatch, expected v2, received %s", r.Tag)
	}
	if r.Digest != "" {
		t.Errorf("SetTag digest mismatch, expected empty string, received %s", r.Digest)
	}
	if r.Reference != rTagStr {
		t.Errorf("SetTag reference mismatch, expected %s, received %s", rTagStr, r.Reference)
	}
	r = r.AddDigest(testDigest)
	if r.Tag != "v2" {
		t.Errorf("AddDigest tag mismatch, expected v2, received %s", r.Tag)
	}
	if r.Digest != testDigest {
		t.Errorf("AddDigest digest mismatch, expected %s, received %s", testDigest, r.Digest)
	}
	if r.Reference != rAddStr {
		t.Errorf("AddDigest reference mismatch, expected %s, received %s", rAddStr, r.Reference)
	}

}

func TestToReg(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name   string
		inRef  string
		expect string
	}{
		{
			name:   "simple path",
			inRef:  "ocidir://test",
			expect: "localhost/test",
		},
		{
			name:   "relative path",
			inRef:  "ocidir://../test",
			expect: "localhost/test",
		},
		{
			name:   "upper case",
			inRef:  "ocidir://Test",
			expect: "localhost/test",
		},
		{
			name:   "other characters",
			inRef:  "ocidir://test_-_hello world",
			expect: "localhost/test-hello-world",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r, err := New(tc.inRef)
			if err != nil {
				t.Fatalf("failed parsing input ref: %v", err)
			}
			outRef := r.ToReg()
			if outRef.CommonName() != tc.expect {
				t.Errorf("convert expected %s, received %s", tc.expect, outRef.CommonName())
			}
		})
	}
}
