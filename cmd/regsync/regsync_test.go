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
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"golang.org/x/sync/semaphore"
)

func TestRegsyncOnce(t *testing.T) {
	ctx := context.Background()
	boolTrue := true
	// setup sample source with an in-memory ocidir directory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
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
				"test1/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test1/blobs/sha256/5283ed7b662424a7f9edc47f8e0e266d47f8ce997da51949d454b30eaafb5251", // amd64
				"test1/blobs/sha256/3f4eb4d2ca4fe85d3da97aab1a56422cb4a05334274a2e275cf848db90a41b18",
				"test1/blobs/sha256/d4ebbdee222ac2d37f728e9fb4f265ff4b31b9ef5de7a701d093d970f8141f0f",
				"test1/blobs/sha256/a7f0466d930515f984dc334bf786a569973119a3afaa2d4290f2268c62a19b12", // arm64
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
				"test2/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test2/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
				"test2/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3
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
				"test2/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
			},
			undesired: []string{
				"test1/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test1/blobs/sha256/5283ed7b662424a7f9edc47f8e0e266d47f8ce997da51949d454b30eaafb5251", // amd64
				"test1/blobs/sha256/3f4eb4d2ca4fe85d3da97aab1a56422cb4a05334274a2e275cf848db90a41b18",
				"test1/blobs/sha256/a7f0466d930515f984dc334bf786a569973119a3afaa2d4290f2268c62a19b12", // arm64
				"test1/blobs/sha256/fd90fc9bbcdaace19655b8983e3d4efa946332f8c1f1aac40ac5bead09bdf9c9", // v2 referrer sbom
				"test1/blobs/sha256/b97ab0b611c4224b58c6b701a566b031b7a8fb7b644487668d31797013372cc4", // v2 referrer sig
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
				"test3/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test3/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3
			},
			undesired: []string{
				"test3/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
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
				"test4/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test4/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3
			},
			undesired: []string{
				"test4/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
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
			exists: []string{"ocidir://test5:v1", "ocidir://test5:sha256-4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40.2e024d7fe67394ce.meta"},
			desired: []string{
				"test5/index.json",
				"test5/oci-layout",
				"test5/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test5/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3 + meta digest tag
			},
			undesired: []string{
				"test5/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
			},
			expErr: nil,
		},
		{
			name: "ImageReferrers",
			sync: ConfigSync{
				Source:          "ocidir://testrepo:v2",
				Target:          "ocidir://test-referrer:v2",
				Type:            "image",
				Referrers:       &boolTrue,
				ReferrerFilters: []ConfigReferrerFilter{},
			},
			exists: []string{"ocidir://test-referrer:v2", "ocidir://test-referrer:sha256-adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8"},
			desired: []string{
				"test-referrer/index.json",
				"test-referrer/oci-layout",
				"test-referrer/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
				"test-referrer/blobs/sha256/ef05efc8cfd478ac3140fce1297bd6b72dc5f5f1df31bfce690aa903a2c20310", // v2 amd64
				"test-referrer/blobs/sha256/fd90fc9bbcdaace19655b8983e3d4efa946332f8c1f1aac40ac5bead09bdf9c9", // v2 sbom
				"test-referrer/blobs/sha256/b97ab0b611c4224b58c6b701a566b031b7a8fb7b644487668d31797013372cc4", // v2 sig
			},
			undesired: []string{
				"test-referrer/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test-referrer/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3 + meta digest tag
			},
			expErr: nil,
		},
		{
			name: "ImageReferrers",
			sync: ConfigSync{
				Source:    "ocidir://testrepo:v2",
				Target:    "ocidir://test-referrer2:v2",
				Type:      "image",
				Referrers: &boolTrue,
				ReferrerFilters: []ConfigReferrerFilter{
					{
						ArtifactType: "application/example.sbom",
					},
				},
			},
			exists: []string{"ocidir://test-referrer2:v2", "ocidir://test-referrer2:sha256-adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8"},
			desired: []string{
				"test-referrer2/index.json",
				"test-referrer2/oci-layout",
				"test-referrer2/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
				"test-referrer2/blobs/sha256/ef05efc8cfd478ac3140fce1297bd6b72dc5f5f1df31bfce690aa903a2c20310", // v2 amd64
				"test-referrer2/blobs/sha256/fd90fc9bbcdaace19655b8983e3d4efa946332f8c1f1aac40ac5bead09bdf9c9", // v2 sbom
			},
			undesired: []string{
				"test-referrer2/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test-referrer2/blobs/sha256/b97ab0b611c4224b58c6b701a566b031b7a8fb7b644487668d31797013372cc4", // v2 sig
				"test-referrer2/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3 + meta digest tag
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
				"test2/blobs/sha256/adab55c36c56f4054a64972a431e38e407d0060ce90888a2470d67598042f7c8", // v2
				"test2/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3
			},
			undesired: []string{
				"test1/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40", // v1
				"test1/blobs/sha256/5283ed7b662424a7f9edc47f8e0e266d47f8ce997da51949d454b30eaafb5251", // amd64
				"test1/blobs/sha256/3f4eb4d2ca4fe85d3da97aab1a56422cb4a05334274a2e275cf848db90a41b18",
				"test1/blobs/sha256/a7f0466d930515f984dc334bf786a569973119a3afaa2d4290f2268c62a19b12", // arm64
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
				"test1/blobs/sha256/4a88e72dd0e4245e6ecfbc6faae751eeeff82861f9ef39634bea07d77dbb1f40",   // v1
				"backups/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3
			},
			undesired: []string{
				"test1/blobs/sha256/2e024d7fe67394cef7f0df9c303f288c7420d1fb27f7657398e897b7eb2ee1d8", // v3
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

func TestProcess(t *testing.T) {
	ctx := context.Background()
	// setup sample source with an in-memory ocidir directory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	// setup various globals normally done by loadConf
	rc = regclient.New(regclient.WithFS(fsMem))
	cs := ConfigSync{
		Source: "ocidir://testrepo",
		Target: "ocidir://testdest",
		Type:   "repository",
	}
	syncSetDefaults(&cs, conf.Defaults)

	tests := []struct {
		name         string
		src          string
		tgt          string
		action       string
		expErr       error
		checkTgtEq   bool
		checkTgtDiff bool
	}{
		{
			name:   "empty",
			expErr: types.ErrNotFound,
		},
		{
			name:         "check v1",
			src:          "v1",
			tgt:          "tgt",
			action:       "check",
			checkTgtDiff: true,
		},
		{
			name:       "copy v1",
			src:        "v1",
			tgt:        "tgt",
			action:     "copy",
			checkTgtEq: true,
		},
		{
			name:         "missing only on v2",
			src:          "v2",
			tgt:          "tgt",
			action:       "missing",
			checkTgtDiff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := ref.New(cs.Source)
			if err != nil {
				t.Errorf("failed to create src ref: %v", err)
				return
			}
			tgt, err := ref.New(cs.Target)
			if err != nil {
				t.Errorf("failed to create tgt ref: %v", err)
				return
			}
			src.Tag = tt.src
			tgt.Tag = tt.tgt
			err = cs.processRef(ctx, src, tgt, tt.action)
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
			if tt.checkTgtEq || tt.checkTgtDiff {
				mSrc, err := rc.ManifestHead(ctx, src)
				if err != nil {
					t.Errorf("error fetching src: %v", err)
				}
				mTgt, err := rc.ManifestHead(ctx, tgt)
				if err != nil && tt.checkTgtEq {
					t.Errorf("error fetching tgt: %v", err)
				}
				if tt.checkTgtEq {
					if mTgt == nil || mSrc.GetDescriptor().Digest != mTgt.GetDescriptor().Digest {
						t.Errorf("source and target mismatch")
					}
				}
				if tt.checkTgtDiff {
					if mTgt != nil && mSrc.GetDescriptor().Digest == mTgt.GetDescriptor().Digest {
						t.Errorf("source and target match")
					}
				}
			}
		})
	}

}

func TestConfigRead(t *testing.T) {
	cRead := bytes.NewReader([]byte(`
    version: 1
    creds:
      - registry: registry:5000
        tls: disabled
      - registry: docker.io
    defaults:
      ratelimit:
        min: 100
        retry: 15m
      parallel: 2
      interval: 60m
      backup: "bkup-{{.Ref.Tag}}"
    x-sync-hub: &sync-hub
      target: registry:5000/hub/{{ .Sync.Source }}
    x-sync-gcr: &sync-gcr
      target: registry:5000/gcr/{{ index (split .Sync.Source "gcr.io/") 1 }}
    sync:
      - source: busybox:latest
        target: registry:5000/library/busybox:latest
        type: image
      - <<: *sync-hub
        source: alpine
        type: repository
        tags:
          allow:
          - 3
          - 3.9
          - latest
      - <<: *sync-gcr
        source: gcr.io/example/repo
        type: repository
        tags:
          allow:
          - 3
          - 3.9
          - latest
  `))
	c, err := ConfigLoadReader(cRead)
	if err != nil {
		t.Errorf("Filed to load reader: %v", err)
		return
	}
	if c.Sync[1].Target != "registry:5000/hub/alpine" {
		t.Errorf("template sync-hub mismatch, expected: %s, received: %s", "registry:5000/hub/alpine", c.Sync[1].Target)
	}
	if c.Sync[2].Target != "registry:5000/gcr/example/repo" {
		t.Errorf("template sync-gcr mismatch, expected: %s, received: %s", "registry:5000/gcr/example/repo", c.Sync[2].Target)
	}
	// TODO: test remainder of templates and parsing
}
