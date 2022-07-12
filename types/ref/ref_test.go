package ref

import (
	"fmt"
	"testing"
)

func TestRef(t *testing.T) {
	var tests = []struct {
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
			name:       "Port registry digest",
			ref:        "registry:5000/group/image@sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			scheme:     "reg",
			registry:   "registry:5000",
			repository: "group/image",
			tag:        "",
			digest:     "sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
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
			ref:        "ocifile://path/to/file.tgz@sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			scheme:     "ocifile",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     "sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			path:       "path/to/file.tgz",
			wantE:      nil,
		},
		{
			name:  "OCI file with invalid digest",
			ref:   "ocifile://path/to/file.tgz@sha256:ZZ15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115ZZ",
			wantE: fmt.Errorf("invalid path for scheme \"ocifile\": path/to/file.tgz@sha256:ZZ15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115ZZ"),
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
			ref:        "ocidir://path/2/dir@sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     "sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			path:       "path/2/dir",
			wantE:      nil,
		},
		{
			name:  "invalid scheme",
			ref:   "unknown://repo:tag",
			wantE: fmt.Errorf(`unhandled reference scheme "unknown" in "unknown://repo:tag"`),
		},
		{
			name:  "invalid host leading dash",
			ref:   "-docker.io/project/image:tag",
			wantE: fmt.Errorf(`invalid reference "-docker.io/project/image:tag"`),
		},
		{
			name:  "invalid host trailing dash",
			ref:   "docker-.io/project/image:tag",
			wantE: fmt.Errorf(`invalid reference "docker-.io/project/image:tag"`),
		},
		{
			name:  "invalid repo case",
			ref:   "docker.io/Upper/Case/Repo:tag",
			wantE: fmt.Errorf(`invalid reference "docker.io/Upper/Case/Repo:tag", repo must be lowercase`),
		},
		{
			name:  "invalid repo dash leading",
			ref:   "project/-image:tag",
			wantE: fmt.Errorf(`invalid reference "project/-image:tag"`),
		},
		{
			name:  "invalid repo dash trailing",
			ref:   "project/image-:tag",
			wantE: fmt.Errorf(`invalid reference "project/image-:tag"`),
		},
		{
			name:  "invalid repo chars",
			ref:   "project/star*:tag",
			wantE: fmt.Errorf(`invalid reference "project/star*:tag"`),
		},
		{
			name:  "invalid tag chars",
			ref:   "project/image:tag^1",
			wantE: fmt.Errorf(`invalid reference "project/image:tag^1"`),
		},
		{
			name:  "invalid short digest",
			ref:   "project/image@sha256:12345",
			wantE: fmt.Errorf(`invalid reference "project/image@sha256:12345"`),
		},
		{
			name:  "invalid digest characters",
			ref:   "project/image@sha256:gggg40677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			wantE: fmt.Errorf(`invalid reference "project/image@sha256:gggg40677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115"`),
		},
		{
			name:  "invalid ocidir path",
			ref:   "ocidir://invalid*filename:tag",
			wantE: fmt.Errorf(`invalid path for scheme "ocidir": invalid*filename:tag`),
		},
		{
			name:  "invalid ocidir tag",
			ref:   "ocidir://filename:tag=fail",
			wantE: fmt.Errorf(`invalid path for scheme "ocidir": filename:tag=fail`),
		},
		{
			name:  "invalid ocidir digest",
			ref:   "ocidir://filename@sha256:abcd",
			wantE: fmt.Errorf(`invalid path for scheme "ocidir": filename@sha256:abcd`),
		},
		{
			name:  "localhost missing repo",
			ref:   "localhost:5000",
			wantE: fmt.Errorf(`invalid reference "localhost:5000"`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := New(tt.ref)
			if tt.wantE == nil && err != nil {
				t.Errorf("failed creating reference, err: %v", err)
				return
			} else if tt.wantE != nil && (err == nil || (tt.wantE != err && tt.wantE.Error() != err.Error())) {
				t.Errorf("expected error not received, expected %v, received %v", tt.wantE, err)
				return
			} else if tt.wantE != nil {
				return
			}
			if tt.ref != ref.Reference {
				t.Errorf("reference mismatch for %s, received %s", tt.ref, ref.Reference)
			}
			if tt.scheme != ref.Scheme {
				t.Errorf("scheme mismatch for %s, expected %s, received %s", tt.ref, tt.scheme, ref.Scheme)
			}
			if tt.registry != ref.Registry {
				t.Errorf("registry mismatch for %s, expected %s, received %s", tt.ref, tt.registry, ref.Registry)
			}
			if tt.repository != ref.Repository {
				t.Errorf("repository mismatch for %s, expected %s, received %s", tt.ref, tt.repository, ref.Repository)
			}
			if tt.tag != ref.Tag {
				t.Errorf("tag mismatch for %s, expected %s, received %s", tt.ref, tt.tag, ref.Tag)
			}
			if tt.digest != ref.Digest {
				t.Errorf("digest mismatch for %s, expected %s, received %s", tt.ref, tt.digest, ref.Digest)
			}
			if tt.path != ref.Path {
				t.Errorf("path mismatch for %s, expected %s, received %s", tt.ref, tt.path, ref.Path)
			}

		})
	}
}

func TestCommon(t *testing.T) {
	tests := []struct {
		name string
		str  string
	}{
		{
			name: "ref with tag",
			str:  "docker.io/group/image:tag",
		},
		{
			name: "ref with digest",
			str:  "docker.io/group/image@sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
		},
		{
			name: "ocidir with tag",
			str:  "ocidir:///tmp/image:tag",
		},
		{
			name: "ocidir with digest",
			str:  "ocidir://image@sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.str)
			if err != nil {
				t.Errorf("failed to parse %s: %v", tt.str, err)
				return
			}
			cn := r.CommonName()
			if tt.str != cn {
				t.Errorf("common name mismatch, input %s, output %s", tt.str, cn)
			}
		})
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if EqualRegistry(tt.a, tt.b) != tt.expectReg {
				t.Errorf("equal registry was not %v for %s and %s", tt.expectReg, tt.a.CommonName(), tt.b.CommonName())
			}
			if EqualRepository(tt.a, tt.b) != tt.expectRepo {
				t.Errorf("equal repository was not %v for %s and %s", tt.expectRepo, tt.a.CommonName(), tt.b.CommonName())
			}
		})
	}
}

func TestToReg(t *testing.T) {
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.inRef)
			if err != nil {
				t.Errorf("failed parsing input ref: %v", err)
				return
			}
			outRef := r.ToReg()
			if outRef.CommonName() != tt.expect {
				t.Errorf("convert expected %s, received %s", tt.expect, outRef.CommonName())
			}
		})
	}

}
