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
	"github.com/regclient/regclient/internal/throttle"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

func TestProcess(t *testing.T) {
	ctx := context.Background()
	boolTrue := true
	// setup sample source with an in-memory ocidir directory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
	if err != nil {
		t.Fatalf("failed to setup memfs copy: %v", err)
	}
	// setup various globals normally done by loadConf
	rc = regclient.New(regclient.WithFS(fsMem))
	throttleC = throttle.New(1)
	var confBytes = `
  version: 1
  defaults:
    parallel: 1
  `
	confRdr := bytes.NewReader([]byte(confBytes))
	conf, err = ConfigLoadReader(confRdr)
	if err != nil {
		t.Fatalf("failed parsing config: %v", err)
	}
	pAMD, err := platform.Parse("linux/amd64")
	if err != nil {
		t.Fatalf("failed to parse amd platform: %v", err)
	}
	pARM, err := platform.Parse("linux/arm64")
	if err != nil {
		t.Fatalf("failed to parse arm platform: %v", err)
	}
	r1, err := ref.New("ocidir://testrepo:v1")
	if err != nil {
		t.Fatalf("failed to parse v1 reference: %v", err)
	}
	r2, err := ref.New("ocidir://testrepo:v2")
	if err != nil {
		t.Fatalf("failed to parse v2 reference: %v", err)
	}
	r3, err := ref.New("ocidir://testrepo:v3")
	if err != nil {
		t.Fatalf("failed to parse v3 reference: %v", err)
	}
	rMirror, err := ref.New("ocidir://testrepo:mirror")
	if err != nil {
		t.Fatalf("failed to parse mirror reference: %v", err)
	}
	rChild, err := ref.New("ocidir://testrepo:child")
	if err != nil {
		t.Fatalf("failed to parse child reference: %v", err)
	}
	rLoop, err := ref.New("ocidir://testrepo:loop")
	if err != nil {
		t.Fatalf("failed to parse loop reference: %v", err)
	}
	m1, err := rc.ManifestGet(ctx, r1)
	if err != nil {
		t.Fatalf("failed to get manifest v1: %v", err)
	}
	d1 := m1.GetDescriptor().Digest
	desc1AMD, err := manifest.GetPlatformDesc(m1, &pAMD)
	if err != nil {
		t.Fatalf("failed to get platform ")
	}
	d1AMD := desc1AMD.Digest
	desc1ARM, err := manifest.GetPlatformDesc(m1, &pARM)
	if err != nil {
		t.Fatalf("failed to get platform ")
	}
	d1ARM := desc1ARM.Digest
	m2, err := rc.ManifestGet(ctx, r2)
	if err != nil {
		t.Fatalf("failed to get manifest v2: %v", err)
	}
	d2 := m2.GetDescriptor().Digest
	desc2AMD, err := manifest.GetPlatformDesc(m2, &pAMD)
	if err != nil {
		t.Fatalf("failed to get platform ")
	}
	d2AMD := desc2AMD.Digest
	desc2SBOM, err := rc.ReferrerList(ctx, r2, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: "application/example.sbom"}))
	if err != nil || len(desc2SBOM.Descriptors) == 0 {
		t.Fatalf("failed to get SBOM for v2: %v", err)
	}
	d2SBOM := desc2SBOM.Descriptors[0].Digest
	desc2Sig, err := rc.ReferrerList(ctx, r2, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: "application/example.signature"}))
	if err != nil || len(desc2Sig.Descriptors) == 0 {
		t.Fatalf("failed to get signature for v2: %v", err)
	}
	d2Sig := desc2Sig.Descriptors[0].Digest
	m3, err := rc.ManifestGet(ctx, r3)
	if err != nil {
		t.Fatalf("failed to get manifest v3: %v", err)
	}
	d3 := m3.GetDescriptor().Digest
	mMirror, err := rc.ManifestGet(ctx, rMirror)
	if err != nil {
		t.Fatalf("failed to get manifest vMirror: %v", err)
	}
	dMirror := mMirror.GetDescriptor().Digest
	mChild, err := rc.ManifestGet(ctx, rChild)
	if err != nil {
		t.Fatalf("failed to get manifest vChild: %v", err)
	}
	dChild := mChild.GetDescriptor().Digest
	mLoop, err := rc.ManifestGet(ctx, rLoop)
	if err != nil {
		t.Fatalf("failed to get manifest vLoop: %v", err)
	}
	dLoop := mLoop.GetDescriptor().Digest

	// run process on each entry
	tt := []struct {
		name      string
		sync      ConfigSync
		action    actionType
		exists    []string
		desired   []string
		undesired []string
		expErr    error
	}{
		{
			name: "Action Missing",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "ocidir://test1:latest",
				Type:   "image",
			},
			action: actionMissing,
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test1/blobs/sha256/" + d1.Hex(),    // v1
				"test1/blobs/sha256/" + d1AMD.Hex(), // amd64
				"test1/blobs/sha256/" + d1ARM.Hex(), // arm64
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
			action: actionCopy,
			exists: []string{"ocidir://test2:v1", "ocidir://test2:v2", "ocidir://test2:v3"},
			desired: []string{
				"test2/index.json",
				"test2/oci-layout",
				"test2/blobs/sha256/" + d1.Hex(), // v1
				"test2/blobs/sha256/" + d2.Hex(), // v2
				"test2/blobs/sha256/" + d3.Hex(), // v3
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
			action: actionCopy,
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/" + d2.Hex(), // v2
			},
			undesired: []string{
				"test1/blobs/sha256/" + d1.Hex(),     // v1
				"test1/blobs/sha256/" + d1AMD.Hex(),  // amd64
				"test1/blobs/sha256/" + d1ARM.Hex(),  // arm64
				"test1/blobs/sha256/" + d2SBOM.Hex(), // v2 referrer sbom
				"test1/blobs/sha256/" + d2Sig.Hex(),  // v2 referrer sig
			},
			expErr: nil,
		},
		{
			name: "Fast Check",
			sync: ConfigSync{
				Source:     "ocidir://testrepo:v2",
				Target:     "ocidir://test1:latest",
				Type:       "image",
				FastCheck:  &boolTrue,
				Referrers:  &boolTrue,
				DigestTags: &boolTrue,
			},
			action: actionCopy,
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/" + d2.Hex(), // v2
			},
			undesired: []string{
				"test1/blobs/sha256/" + d1.Hex(),     // v1
				"test1/blobs/sha256/" + d1AMD.Hex(),  // amd64
				"test1/blobs/sha256/" + d1ARM.Hex(),  // arm64
				"test1/blobs/sha256/" + d2SBOM.Hex(), // v2 referrer sbom
				"test1/blobs/sha256/" + d2Sig.Hex(),  // v2 referrer sig
			},
			expErr: nil,
		},
		{
			name: "Action Check",
			sync: ConfigSync{
				Source:     "ocidir://testrepo:v1",
				Target:     "ocidir://test1:latest",
				Type:       "image",
				Referrers:  &boolTrue,
				DigestTags: &boolTrue,
			},
			action: actionCheck,
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/" + d2.Hex(), // v2
			},
			undesired: []string{
				"test1/blobs/sha256/" + d1.Hex(),     // v1
				"test1/blobs/sha256/" + d1AMD.Hex(),  // amd64
				"test1/blobs/sha256/" + d1ARM.Hex(),  // arm64
				"test1/blobs/sha256/" + d2SBOM.Hex(), // v2 referrer sbom
				"test1/blobs/sha256/" + d2Sig.Hex(),  // v2 referrer sig
			},
			expErr: nil,
		},
		{
			name: "Action Missing Exists",
			sync: ConfigSync{
				Source:     "ocidir://testrepo:v1",
				Target:     "ocidir://test1:latest",
				Type:       "image",
				Referrers:  &boolTrue,
				DigestTags: &boolTrue,
			},
			action: actionMissing,
			exists: []string{"ocidir://test1:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/" + d2.Hex(), // v2
			},
			undesired: []string{
				"test1/blobs/sha256/" + d1.Hex(),     // v1
				"test1/blobs/sha256/" + d1AMD.Hex(),  // amd64
				"test1/blobs/sha256/" + d1ARM.Hex(),  // arm64
				"test1/blobs/sha256/" + d2SBOM.Hex(), // v2 referrer sbom
				"test1/blobs/sha256/" + d2Sig.Hex(),  // v2 referrer sig
			},
			expErr: nil,
		},
		{
			name: "RepoTagFilterAllow",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "ocidir://test3",
				Type:   "repository",
				Tags: AllowDeny{
					Allow: []string{"v1", "v3", "latest"},
				},
			},
			action: actionCopy,
			exists: []string{"ocidir://test3:v1", "ocidir://test3:v3"},
			desired: []string{
				"test3/index.json",
				"test3/oci-layout",
				"test3/blobs/sha256/" + d1.Hex(), // v1
				"test3/blobs/sha256/" + d3.Hex(), // v3
			},
			undesired: []string{
				"test3/blobs/sha256/" + d2.Hex(), // v2
			},
			expErr: nil,
		},
		{
			name: "RepoTagFilterDeny",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "ocidir://test4",
				Type:   "repository",
				Tags: AllowDeny{
					Deny: []string{"v2", "old"},
				},
			},
			action: actionCopy,
			exists: []string{"ocidir://test4:v1", "ocidir://test4:v3"},
			desired: []string{
				"test4/index.json",
				"test4/oci-layout",
				"test4/blobs/sha256/" + d1.Hex(), // v1
				"test4/blobs/sha256/" + d3.Hex(), // v3
			},
			undesired: []string{
				"test4/blobs/sha256/" + d2.Hex(), // v2
			},
			expErr: nil,
		},
		{
			name: "Missing Setup v1",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v2",
				Target: "ocidir://test-missing:v1",
				Type:   "image",
			},
			action: actionCopy,
			exists: []string{"ocidir://test-missing:v1"},
			desired: []string{
				"test-missing/index.json",
				"test-missing/oci-layout",
				"test-missing/blobs/sha256/" + d2.Hex(), // v2
			},
			undesired: []string{
				"test-missing/blobs/sha256/" + d1.Hex(), // v1
				"test-missing/blobs/sha256/" + d3.Hex(), // v3
			},
			expErr: nil,
		},
		{
			name: "Missing Setup v1.1",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v2",
				Target: "ocidir://test-missing:v1.1",
				Type:   "image",
			},
			action: actionCopy,
			exists: []string{"ocidir://test-missing:v1.1"},
			expErr: nil,
		},
		{
			name: "Missing Setup v3",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v3",
				Target: "ocidir://test-missing:v3",
				Type:   "image",
			},
			action: actionCopy,
			exists: []string{"ocidir://test-missing:v1", "ocidir://test-missing:v3"},
			desired: []string{
				"test-missing/index.json",
				"test-missing/oci-layout",
				"test-missing/blobs/sha256/" + d2.Hex(), // v2
				"test-missing/blobs/sha256/" + d3.Hex(), // v3
			},
			undesired: []string{
				"test-missing/blobs/sha256/" + d1.Hex(), // v1
			},
			expErr: nil,
		},
		{
			name: "Missing",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "ocidir://test-missing",
				Type:   "repository",
				Tags: AllowDeny{
					Allow: []string{"v1", "v2", "v3", "latest"},
				},
			},
			action: actionMissing,
			exists: []string{"ocidir://test-missing:v1", "ocidir://test-missing:v2", "ocidir://test-missing:v3"},
			desired: []string{
				"test-missing/index.json",
				"test-missing/oci-layout",
				"test-missing/blobs/sha256/" + d2.Hex(), // v2
				"test-missing/blobs/sha256/" + d3.Hex(), // v3
			},
			undesired: []string{
				"test-missing/blobs/sha256/" + d1.Hex(), // v1
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
			action: actionCopy,
			exists: []string{"ocidir://test5:v1", fmt.Sprintf("ocidir://test5:sha256-%s.%.16s.meta", d1.Hex(), d3.Hex())},
			desired: []string{
				"test5/index.json",
				"test5/oci-layout",
				"test5/blobs/sha256/" + d1.Hex(), // v1
				"test5/blobs/sha256/" + d3.Hex(), // v3 + meta digest tag
			},
			undesired: []string{
				"test5/blobs/sha256/" + d2.Hex(), // v2
			},
			expErr: nil,
		},
		{
			name: "ImageReferrers Fast",
			sync: ConfigSync{
				Source:          "ocidir://testrepo:v2",
				Target:          "ocidir://test-referrer:v2",
				Type:            "image",
				FastCheck:       &boolTrue,
				Referrers:       &boolTrue,
				ReferrerFilters: []ConfigReferrerFilter{},
			},
			action: actionCopy,
			exists: []string{"ocidir://test-referrer:v2", "ocidir://test-referrer:sha256-" + d2.Hex()},
			desired: []string{
				"test-referrer/index.json",
				"test-referrer/oci-layout",
				"test-referrer/blobs/sha256/" + d2.Hex(),     // v2
				"test-referrer/blobs/sha256/" + d2AMD.Hex(),  // v2 amd64
				"test-referrer/blobs/sha256/" + d2SBOM.Hex(), // v2 sbom
				"test-referrer/blobs/sha256/" + d2Sig.Hex(),  // v2 sig
			},
			undesired: []string{
				"test-referrer/blobs/sha256/" + d1.Hex(), // v1
				"test-referrer/blobs/sha256/" + d3.Hex(), // v3 + meta digest tag
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
			action: actionCopy,
			exists: []string{"ocidir://test-referrer2:v2", "ocidir://test-referrer2:sha256-" + d2.Hex()},
			desired: []string{
				"test-referrer2/index.json",
				"test-referrer2/oci-layout",
				"test-referrer2/blobs/sha256/" + d2.Hex(),     // v2
				"test-referrer2/blobs/sha256/" + d2AMD.Hex(),  // v2 amd64
				"test-referrer2/blobs/sha256/" + d2SBOM.Hex(), // v2 sbom
			},
			undesired: []string{
				"test-referrer2/blobs/sha256/" + d1.Hex(),    // v1
				"test-referrer2/blobs/sha256/" + d2Sig.Hex(), // v2 sig
				"test-referrer2/blobs/sha256/" + d3.Hex(),    // v3 + meta digest tag
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
			action: actionCopy,
			exists: []string{"ocidir://test1:latest", "ocidir://test1:old"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test2/blobs/sha256/" + d2.Hex(), // v2
				"test2/blobs/sha256/" + d3.Hex(), // v3
			},
			undesired: []string{
				"test1/blobs/sha256/" + d1.Hex(),    // v1
				"test1/blobs/sha256/" + d1AMD.Hex(), // amd64
				"test1/blobs/sha256/" + d1ARM.Hex(), // arm64
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
			action: actionCopy,
			exists: []string{"ocidir://test1:latest", "ocidir://backups:latest"},
			desired: []string{
				"test1/index.json",
				"test1/oci-layout",
				"test1/blobs/sha256/" + d1.Hex(),   // v1
				"backups/blobs/sha256/" + d3.Hex(), // v3
			},
			undesired: []string{
				"test1/blobs/sha256/" + d3.Hex(), // v3
			},
			expErr: nil,
		},
		{
			name: "Image Self Digest Tag",
			sync: ConfigSync{
				Source:     "ocidir://testrepo:mirror",
				Target:     "ocidir://test-mirror:mirror",
				Type:       "image",
				DigestTags: &boolTrue,
			},
			action: actionCopy,
			exists: []string{"ocidir://test-mirror:mirror", "ocidir://test-mirror:sha256-" + dMirror.Hex()},
			desired: []string{
				"test-mirror/index.json",
				"test-mirror/oci-layout",
				"test-mirror/blobs/sha256/" + dMirror.Hex(), // mirror
			},
			expErr: nil,
		},
		{
			name: "Image Loop",
			sync: ConfigSync{
				Source:    "ocidir://testrepo:loop",
				Target:    "ocidir://test-loop:loop",
				Type:      "image",
				Referrers: &boolTrue,
			},
			action: actionCopy,
			exists: []string{"ocidir://test-loop:loop", "ocidir://test-loop:sha256-" + dChild.Hex()},
			desired: []string{
				"test-loop/index.json",
				"test-loop/oci-layout",
				"test-loop/blobs/sha256/" + dChild.Hex(), // child
				"test-loop/blobs/sha256/" + dLoop.Hex(),  // loop
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
			action:  actionCopy,
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
			action:  actionCopy,
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
			action:  actionCopy,
			desired: []string{},
			expErr:  errs.ErrInvalidReference,
		},
		{
			name: "InvalidTargetImage",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "InvalidTestmissing:v1:garbage",
				Type:   "image",
			},
			action:  actionCopy,
			desired: []string{},
			expErr:  errs.ErrInvalidReference,
		},
		{
			name: "InvalidSourceRepository",
			sync: ConfigSync{
				Source: "InvalidTestmissing:garbage",
				Target: "ocidir://testrepo",
				Type:   "repository",
			},
			action:  actionCopy,
			desired: []string{},
			expErr:  errs.ErrInvalidReference,
		},
		{
			name: "InvalidTargetRepository",
			sync: ConfigSync{
				Source: "ocidir://testrepo",
				Target: "InvalidTestmissing:garbage",
				Type:   "repository",
			},
			action:  actionCopy,
			desired: []string{},
			expErr:  errs.ErrInvalidReference,
		},
		{
			name: "InvalidType",
			sync: ConfigSync{
				Source: "ocidir://testrepo:v1",
				Target: "ocidir://test1:v1",
				Type:   "invalid",
			},
			action:  actionCopy,
			desired: []string{},
			expErr:  ErrInvalidInput,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// run each test
			rootOpts := rootCmd{}
			syncSetDefaults(&tc.sync, conf.Defaults)
			err = rootOpts.process(ctx, tc.sync, tc.action)
			// validate err
			if tc.expErr != nil {
				if err == nil {
					t.Errorf("process did not fail")
				} else if !errors.Is(err, tc.expErr) && err.Error() != tc.expErr.Error() {
					t.Errorf("unexpected error on process: %v, expected %v", err, tc.expErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error on process: %v", err)
			}
			// validate tags and files exist/don't exist
			for _, exist := range tc.exists {
				r, err := ref.New(exist)
				if err != nil {
					t.Fatalf("cannot parse ref %s: %v", exist, err)
				}
				_, err = rc.ManifestHead(ctx, r)
				if err != nil {
					t.Errorf("ref does not exist: %s", exist)
				}
			}
			for _, file := range tc.desired {
				_, err = rwfs.Stat(fsMem, file)
				if err != nil {
					t.Errorf("missing file in sync: %s", file)
				}
			}
			for _, file := range tc.undesired {
				_, err = rwfs.Stat(fsMem, file)
				if err == nil {
					t.Errorf("undesired file after sync: %s", file)
				}
			}
		})
	}
}

func TestProcessRef(t *testing.T) {
	ctx := context.Background()
	// setup sample source with an in-memory ocidir directory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
	if err != nil {
		t.Fatalf("failed to setup memfs copy: %v", err)
	}
	// setup various globals normally done by loadConf
	rc = regclient.New(regclient.WithFS(fsMem))
	cs := ConfigSync{
		Source: "ocidir://testrepo",
		Target: "ocidir://testdest",
		Type:   "repository",
	}
	syncSetDefaults(&cs, conf.Defaults)

	tt := []struct {
		name         string
		src          string
		tgt          string
		action       actionType
		expErr       error
		checkTgtEq   bool
		checkTgtDiff bool
	}{
		{
			name:   "empty",
			expErr: errs.ErrNotFound,
		},
		{
			name:         "check v1",
			src:          "v1",
			tgt:          "tgt",
			action:       actionCheck,
			checkTgtDiff: true,
		},
		{
			name:       "copy v1",
			src:        "v1",
			tgt:        "tgt",
			action:     actionCopy,
			checkTgtEq: true,
		},
		{
			name:         "missing only on v2",
			src:          "v2",
			tgt:          "tgt",
			action:       actionMissing,
			checkTgtDiff: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			rootOpts := rootCmd{}
			src, err := ref.New(cs.Source)
			if err != nil {
				t.Fatalf("failed to create src ref: %v", err)
			}
			tgt, err := ref.New(cs.Target)
			if err != nil {
				t.Fatalf("failed to create tgt ref: %v", err)
			}
			src.Tag = tc.src
			tgt.Tag = tc.tgt
			err = rootOpts.processRef(ctx, cs, src, tgt, tc.action)
			// validate err
			if tc.expErr != nil {
				if err == nil {
					t.Errorf("process did not fail")
				} else if !errors.Is(err, tc.expErr) && err.Error() != tc.expErr.Error() {
					t.Errorf("unexpected error on process: %v, expected %v", err, tc.expErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error on process: %v", err)
			}
			if tc.checkTgtEq || tc.checkTgtDiff {
				mSrc, err := rc.ManifestHead(ctx, src)
				if err != nil {
					t.Fatalf("error fetching src: %v", err)
				}
				mTgt, err := rc.ManifestHead(ctx, tgt)
				if err != nil && tc.checkTgtEq {
					t.Fatalf("error fetching tgt: %v", err)
				}
				if tc.checkTgtEq {
					if mTgt == nil || mSrc.GetDescriptor().Digest != mTgt.GetDescriptor().Digest {
						t.Errorf("source and target mismatch")
					}
				}
				if tc.checkTgtDiff {
					if mTgt != nil && mSrc.GetDescriptor().Digest == mTgt.GetDescriptor().Digest {
						t.Errorf("source and target match")
					}
				}
			}
		})
	}
}

func TestConfigRead(t *testing.T) {
	// CAUTION: the below yaml is space indented and will not parse with tabs
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
      cacheCount: 500
      cacheTime: "5m"
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
		t.Fatalf("Filed to load reader: %v", err)
	}
	if c.Sync[1].Target != "registry:5000/hub/alpine" {
		t.Errorf("template sync-hub mismatch, expected: %s, received: %s", "registry:5000/hub/alpine", c.Sync[1].Target)
	}
	if c.Sync[2].Target != "registry:5000/gcr/example/repo" {
		t.Errorf("template sync-gcr mismatch, expected: %s, received: %s", "registry:5000/gcr/example/repo", c.Sync[2].Target)
	}
	// TODO: test remainder of templates and parsing
}
