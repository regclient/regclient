package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
	"golang.org/x/sync/semaphore"
)

func TestRegsync(t *testing.T) {
	ctx := context.Background()
	boolTrue := true
	// setup sample source with an in-memory ocidir directory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	// setup various globals normally done by loadConf
	rc = regclient.New(regclient.WithFS(fsMem))
	sem = semaphore.NewWeighted(1)
	var confBytes = `
  version: 1
  defaults:
    parallel: 1
  `
	confRdr := bytes.NewReader([]byte(confBytes))
	conf, err = ConfigLoadReader(confRdr)
	if err != nil {
		t.Errorf("failed parsing config: %v", err)
		return
	}

	// run process on each entry
	tests := []struct {
		name      string
		sync      ConfigSync
		exists    []string
		desired   []string
		undesired []string
		expErr    error
	}{
		{
			name: "ImageCopy",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "ocidir://test1:latest",
				Type:   "image",
			},
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test1/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test1/blobs/sha256/aa962da1b4176a25590e0daad1117723ad155486bffea9f3f1360d312b9aa832", // amd64
				"test1/blobs/sha256/6d4b840cb62c293cb95f29d19fe1ef098354ca1cbc8be9222e7a8e31cb5e1caf",
				"test1/blobs/sha256/9fc9590240f5264b6470ceca1aa90197f9223973890bebbb082011fc985e4412",
				"test1/blobs/sha256/2ea09753fab80a36c32fc7a959537a38a7bcbf09eddba48671c51c29b2c943ac", // arm64
			},
			expErr: nil,
		},
		{
			name: "RepoCopy",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "ocidir://test2",
				Type:   "repository",
			},
			exists: []string{"ocidir://test2:v1", "ocidir://test2:v2", "ocidir://test2:v3"},
			desired: []string{
				"test2/index.json",
				"test2/oci-layout",
				"test2/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test2/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1", // v2
				"test2/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3
			},
			expErr: nil,
		},
		{
			name: "Overwrite",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v2",
				Target: "ocidir://test1:latest",
				Type:   "image",
			},
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1", // v2
			},
			undesired: []string{
				"test1/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test1/blobs/sha256/aa962da1b4176a25590e0daad1117723ad155486bffea9f3f1360d312b9aa832", // amd64
				"test1/blobs/sha256/6d4b840cb62c293cb95f29d19fe1ef098354ca1cbc8be9222e7a8e31cb5e1caf",
				"test1/blobs/sha256/2ea09753fab80a36c32fc7a959537a38a7bcbf09eddba48671c51c29b2c943ac", // arm64
			},
			expErr: nil,
		},
		{
			name: "RepoTagFilterAllow",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "ocidir://test3",
				Type:   "repository",
				Tags: ConfigTags{
					Allow: []string{"v1", "v3", "latest"},
				},
			},
			exists: []string{"ocidir://test3:v1", "ocidir://test3:v3"},
			desired: []string{
				"test3/index.json",
				"test3/oci-layout",
				"test3/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test3/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3
			},
			undesired: []string{
				"test3/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1", // v2
			},
			expErr: nil,
		},
		{
			name: "RepoTagFilterDeny",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "ocidir://test4",
				Type:   "repository",
				Tags: ConfigTags{
					Deny: []string{"v2", "old"},
				},
			},
			exists: []string{"ocidir://test4:v1", "ocidir://test4:v3"},
			desired: []string{
				"test4/index.json",
				"test4/oci-layout",
				"test4/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test4/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3
			},
			undesired: []string{
				"test4/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1", // v2
			},
			expErr: nil,
		},
		{
			name: "ImageDigestTags",
			sync: ConfigSync{
				Source:     "ocidir://testrepo:v1",
				Target:     "ocidir://test5:v1",
				Type:       "image",
				DigestTags: &boolTrue,
			},
			exists: []string{"ocidir://test5:v1", "ocidir://test5:sha256-94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694.meta"},
			desired: []string{
				"test5/index.json",
				"test5/oci-layout",
				"test5/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test5/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3 + meta digest tag
			},
			undesired: []string{
				"test5/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1", // v2
			},
			expErr: nil,
		},
		{
			name: "Backup",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v3",
				Target: "ocidir://test1:latest",
				Type:   "image",
				Backup: "old",
			},
			exists: []string{"ocidir://test1:latest", "ocidir://test1:old"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1", // v2
				"test2/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3
			},
			undesired: []string{
				"test1/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				"test1/blobs/sha256/aa962da1b4176a25590e0daad1117723ad155486bffea9f3f1360d312b9aa832", // amd64
				"test1/blobs/sha256/6d4b840cb62c293cb95f29d19fe1ef098354ca1cbc8be9222e7a8e31cb5e1caf",
				"test1/blobs/sha256/2ea09753fab80a36c32fc7a959537a38a7bcbf09eddba48671c51c29b2c943ac", // arm64
			},
			expErr: nil,
		},
		{
			name: "BackupFormat",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "ocidir://test1:latest",
				Type:   "image",
				Backup: "ocidir://backups:{{.Ref.Tag}}",
			},
			exists: []string{"ocidir://test1:latest", "ocidir://backups:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test1/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
				// "test1/blobs/sha256/3fadbd1aeb4e8c0fe8328c4007012b7a6fdbc7c578ad4880b3480706a3432be1",   // v2 - with old tag
				"backups/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3
			},
			undesired: []string{
				"test1/blobs/sha256/a4bdb3dbc74b4fce1d2064f346ddb767cd36e4f959e570c01970036912c2c0fb", // v3
			},
			expErr: nil,
		},
		{
			name: "MissingImage",
			sync: ConfigSync{
				Source: "ocidir://testmissing:v1",
				Target: "ocidir://testmissing:v1.1",
				Type:   "image",
			},
			desired: []string{},
			expErr:  fs.ErrNotExist,
		},
		{
			name: "MissingRepository",
			sync: ConfigSync{
				Source: "ocidir://testmissing:v1",
				Target: "ocidir://testmissing:v1.1",
				Type:   "repository",
			},
			desired: []string{},
			expErr:  fs.ErrNotExist,
		},
		{
			name: "InvalidSourceImage",
			sync: ConfigSync{
				Source: "InvalidTestmissing:v1:garbage",
				Target: "ocidir://testrepo:v1",
				Type:   "image",
			},
			desired: []string{},
			expErr:  fmt.Errorf(`invalid reference "InvalidTestmissing:v1:garbage"`),
		},
		{
			name: "InvalidTargetImage",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "InvalidTestmissing:v1:garbage",
				Type:   "image",
			},
			desired: []string{},
			expErr:  fmt.Errorf(`invalid reference "InvalidTestmissing:v1:garbage"`),
		},
		{
			name: "InvalidSourceRepository",
			sync: ConfigSync{
				Source: "InvalidTestmissing:v1:garbage",
				Target: "ocidir://testrepo:v1",
				Type:   "repository",
			},
			desired: []string{},
			expErr:  fmt.Errorf(`invalid reference "InvalidTestmissing:v1:garbage"`),
		},
		{
			name: "InvalidTargetRepository",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "InvalidTestmissing:v1:garbage",
				Type:   "repository",
			},
			desired: []string{},
			expErr:  fmt.Errorf(`invalid reference "InvalidTestmissing:v1:garbage"`),
		},
		{
			name: "InvalidType",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "ocidir://test1:v1",
				Type:   "invalid",
			},
			desired: []string{},
			expErr:  ErrInvalidInput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// run each test
			syncSetDefaults(&tt.sync, conf.Defaults)
			err = tt.sync.process(ctx, "once")
			// validate err
			if tt.expErr != nil {
				if err == nil {
					t.Errorf("process did not fail")
				} else if !errors.Is(err, tt.expErr) && err.Error() != tt.expErr.Error() {
					t.Errorf("unexpected error on process: %v, expected %v", err, tt.expErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error on process: %v", err)
				return
			}
			// validate tags and files exist/don't exist
			for _, exist := range tt.exists {
				r, err := ref.New(exist)
				if err != nil {
					t.Errorf("cannot parse ref %s: %v", exist, err)
					continue
				}
				_, err = rc.ManifestHead(ctx, r)
				if err != nil {
					t.Errorf("ref does not exist: %s", exist)
				}
			}
			for _, file := range tt.desired {
				_, err = rwfs.Stat(fsMem, file)
				if err != nil {
					t.Errorf("missing file in sync: %s", file)
				}
			}
			for _, file := range tt.undesired {
				_, err = rwfs.Stat(fsMem, file)
				if err == nil {
					t.Errorf("undesired file after sync: %s", file)
				}
			}
		})
	}

}
