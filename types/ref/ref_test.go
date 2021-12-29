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
			name:       "Local registry",
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
			ref:        "ocidir://path/to/dir:v1.2.3",
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "v1.2.3",
			digest:     "",
			path:       "path/to/dir",
			wantE:      nil,
		},
		{
			name:       "OCI dir with digest",
			ref:        "ocidir://path/to/dir@sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			scheme:     "ocidir",
			registry:   "",
			repository: "",
			tag:        "",
			digest:     "sha256:15f840677a5e245d9ea199eb9b026b1539208a5183621dced7b469f6aa678115",
			path:       "path/to/dir",
			wantE:      nil,
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
