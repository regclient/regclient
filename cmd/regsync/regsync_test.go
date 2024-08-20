package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/internal/throttle"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

func TestProcess(t *testing.T) {
	ctx := context.Background()
	boolT := true
	var err error
	tempDir := t.TempDir()
	err = copyfs.Copy(tempDir+"/testrepo", "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to copy testrepo to tempdir: %v", err)
	}
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../testdata",
		},
	})
	ts := httptest.NewServer(regHandler)
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	t.Cleanup(func() {
		ts.Close()
		_ = regHandler.Close()
	})
	rcHosts := []config.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     "registry.example.org",
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
	}
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	// replace regclient with one configured for test hosts
	rc = regclient.New(
		regclient.WithConfigHost(rcHosts...),
		regclient.WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)
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
	r1, err := ref.New(tsHost + "/testrepo:v1")
	if err != nil {
		t.Fatalf("failed to parse v1 reference: %v", err)
	}
	r2, err := ref.New(tsHost + "/testrepo:v2")
	if err != nil {
		t.Fatalf("failed to parse v2 reference: %v", err)
	}
	r3, err := ref.New(tsHost + "/testrepo:v3")
	if err != nil {
		t.Fatalf("failed to parse v3 reference: %v", err)
	}
	rMirror, err := ref.New(tsHost + "/testrepo:mirror")
	if err != nil {
		t.Fatalf("failed to parse mirror reference: %v", err)
	}
	rChild, err := ref.New(tsHost + "/testrepo:child")
	if err != nil {
		t.Fatalf("failed to parse child reference: %v", err)
	}
	rLoop, err := ref.New(tsHost + "/testrepo:loop")
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
		name    string
		sync    ConfigSync
		action  actionType
		expect  map[string]digest.Digest
		exists  []string
		missing []string
		expErr  error
	}{
		{
			name: "Action Missing",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v1",
				Target: tsHost + "/test1:latest",
				Type:   "image",
			},
			action: actionMissing,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest": d1,
			},
			exists: []string{
				tsHost + "/test1@" + d1AMD.String(),
				tsHost + "/test1@" + d1ARM.String(),
			},
			missing: []string{
				tsHost + "/test1@" + d2.String(),
				tsHost + "/test1@" + d2SBOM.String(),
				tsHost + "/test1@" + d2Sig.String(),
			},
			expErr: nil,
		},
		{
			name: "RepoCopy",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsHost + "/test2",
				Type:   "repository",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test2:v1": d1,
				tsHost + "/test2:v2": d2,
				tsHost + "/test2:v3": d3,
			},
			expErr: nil,
		},
		{
			name: "Overwrite",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v2",
				Target: tsHost + "/test1:latest",
				Type:   "image",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest": d2,
			},
			exists: []string{},
			missing: []string{
				tsHost + "/test1@" + d2SBOM.String(),
				tsHost + "/test1@" + d2Sig.String(),
			},
			expErr: nil,
		},
		{
			name: "Fast Check",
			sync: ConfigSync{
				Source:     tsHost + "/testrepo:v2",
				Target:     tsHost + "/test1:latest",
				Type:       "image",
				FastCheck:  &boolT,
				Referrers:  &boolT,
				DigestTags: &boolT,
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest": d2,
			},
			exists: []string{},
			missing: []string{
				tsHost + "/test1@" + d2SBOM.String(),
				tsHost + "/test1@" + d2Sig.String(),
			},
			expErr: nil,
		},
		{
			name: "Action Check",
			sync: ConfigSync{
				Source:     tsHost + "/testrepo:v1",
				Target:     tsHost + "/test1:latest",
				Type:       "image",
				Referrers:  &boolT,
				DigestTags: &boolT,
			},
			action: actionCheck,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest": d2,
			},
			exists: []string{},
			expErr: nil,
		},
		{
			name: "Action Missing Exists",
			sync: ConfigSync{
				Source:     tsHost + "/testrepo:v1",
				Target:     tsHost + "/test1:latest",
				Type:       "image",
				Referrers:  &boolT,
				DigestTags: &boolT,
			},
			action: actionMissing,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest": d2,
			},
			exists: []string{},
			missing: []string{
				tsHost + "/test1@" + d2SBOM.String(),
				tsHost + "/test1@" + d2Sig.String(),
			},
			expErr: nil,
		},
		{
			name: "RepoTagFilterAllow",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsHost + "/test3",
				Type:   "repository",
				Tags: AllowDeny{
					Allow: []string{"v1", "v3", "latest"},
				},
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test3:v1": d1,
				tsHost + "/test3:v3": d3,
			},
			exists: []string{},
			missing: []string{
				tsHost + "/test3:v2",
				tsHost + "/test3@" + d2.String(),
			},
			expErr: nil,
		},
		{
			name: "RepoTagFilterDeny",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsHost + "/test4",
				Type:   "repository",
				Tags: AllowDeny{
					Deny: []string{"v2", "old"},
				},
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test4:v1": d1,
				tsHost + "/test4:v3": d3,
			},
			exists: []string{},
			missing: []string{
				tsHost + "/test4:v2",
				tsHost + "/test4@" + d2.String(),
			},
			expErr: nil,
		},
		{
			name: "Missing Setup v1",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v2",
				Target: tsHost + "/test-missing:v1",
				Type:   "image",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-missing:v1": d2,
			},
			missing: []string{
				tsHost + "/test-missing@" + d1.String(),
				tsHost + "/test-missing@" + d3.String(),
			},
			expErr: nil,
		},
		{
			name: "Missing Setup v1.1",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v2",
				Target: tsHost + "/test-missing:v1.1",
				Type:   "image",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-missing:v1":   d2,
				tsHost + "/test-missing:v1.1": d2,
			},
			missing: []string{
				tsHost + "/test-missing@" + d1.String(),
				tsHost + "/test-missing@" + d3.String(),
			},
			expErr: nil,
		},
		{
			name: "Missing Setup v3",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v3",
				Target: tsHost + "/test-missing:v3",
				Type:   "image",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-missing:v1":   d2,
				tsHost + "/test-missing:v1.1": d2,
				tsHost + "/test-missing:v3":   d3,
			},
			missing: []string{
				tsHost + "/test-missing@" + d1.String(),
			},
			expErr: nil,
		},
		{
			name: "Missing",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsHost + "/test-missing",
				Type:   "repository",
				Tags: AllowDeny{
					Allow: []string{"v1", "v2", "v3", "latest"},
				},
			},
			action: actionMissing,
			expect: map[string]digest.Digest{
				tsHost + "/test-missing:v1":   d2,
				tsHost + "/test-missing:v1.1": d2,
				tsHost + "/test-missing:v2":   d2,
				tsHost + "/test-missing:v3":   d3,
			},
			missing: []string{
				tsHost + "/test-missing@" + d1.String(),
			},
			expErr: nil,
		},
		{
			name: "ImageDigestTags",
			sync: ConfigSync{
				Source:     "ocidir://" + tempDir + "/testrepo:v1",
				Target:     "ocidir://" + tempDir + "/test5:v1",
				Type:       "image",
				DigestTags: &boolT,
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				"ocidir://" + tempDir + "/test5:v1":                                                d1,
				fmt.Sprintf("ocidir://%s/test5:sha256-%s.%.16s.meta", tempDir, d1.Hex(), d3.Hex()): digest.Digest(d3.String()),
			},
			expErr: nil,
		},
		{
			name: "ImageReferrers Fast",
			sync: ConfigSync{
				Source:          tsHost + "/testrepo:v2",
				Target:          tsHost + "/test-referrer:v2",
				Type:            "image",
				FastCheck:       &boolT,
				Referrers:       &boolT,
				ReferrerFilters: []ConfigReferrerFilter{},
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-referrer:v2": d2,
			},
			exists: []string{
				tsHost + "/test-referrer@" + d2AMD.String(),
				tsHost + "/test-referrer@" + d2SBOM.String(),
				tsHost + "/test-referrer@" + d2Sig.String(),
			},
			missing: []string{
				tsHost + "/test-referrer@" + d1.String(),
				tsHost + "/test-referrer@" + d3.String(),
			},
			expErr: nil,
		},
		{
			name: "ImageReferrers",
			sync: ConfigSync{
				Source:    tsHost + "/testrepo:v2",
				Target:    tsHost + "/test-referrer2:v2",
				Type:      "image",
				Referrers: &boolT,
				ReferrerFilters: []ConfigReferrerFilter{
					{
						ArtifactType: "application/example.sbom",
					},
				},
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-referrer2:v2": d2,
			},
			exists: []string{
				tsHost + "/test-referrer2@" + d2AMD.String(),
				tsHost + "/test-referrer2@" + d2SBOM.String(),
			},
			missing: []string{
				tsHost + "/test-referrer2@" + d2Sig.String(),
				tsHost + "/test-referrer2@" + d1.String(),
				tsHost + "/test-referrer2@" + d3.String(),
			},
			expErr: nil,
		},
		{
			name: "Backup",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v3",
				Target: tsHost + "/test1:latest",
				Type:   "image",
				Backup: "old",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest": d3,
				tsHost + "/test1:old":    d2,
			},
			expErr: nil,
		},
		{
			name: "BackupFormat",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v1",
				Target: tsHost + "/test1:latest",
				Type:   "image",
				Backup: tsHost + "/backups:{{.Ref.Tag}}",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test1:latest":   d1,
				tsHost + "/backups:latest": d3,
			},
			expErr: nil,
		},
		{
			name: "Image Self Digest Tag",
			sync: ConfigSync{
				Source:     "ocidir://" + tempDir + "/testrepo:mirror",
				Target:     "ocidir://" + tempDir + "/test-mirror:mirror",
				Type:       "image",
				DigestTags: &boolT,
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				"ocidir://" + tempDir + "/test-mirror:mirror":                  dMirror,
				"ocidir://" + tempDir + "/test-mirror:sha256-" + dMirror.Hex(): dMirror,
			},
			expErr: nil,
		},
		{
			name: "Image Loop",
			sync: ConfigSync{
				Source:    tsHost + "/testrepo:loop",
				Target:    tsHost + "/test-loop:loop",
				Type:      "image",
				Referrers: &boolT,
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-loop:loop": dLoop,
			},
			exists: []string{
				tsHost + "/test-loop@" + dChild.String(),
			},
			expErr: nil,
		},
		{
			name: "MissingImage",
			sync: ConfigSync{
				Source: tsHost + "/testmissing:v1",
				Target: tsHost + "/testmissing:v1.1",
				Type:   "image",
			},
			action: actionCopy,
			expErr: errs.ErrNotFound,
		},
		{
			name: "MissingRepository",
			sync: ConfigSync{
				Source: "ocidir://" + tempDir + "/testmissing",
				Target: tsHost + "/testmissing",
				Type:   "repository",
			},
			action: actionCopy,
			expErr: fs.ErrNotExist,
		},
		{
			name: "InvalidSourceImage",
			sync: ConfigSync{
				Source: "InvalidTestmissing:v1:garbage",
				Target: tsHost + "/testrepo:v1",
				Type:   "image",
			},
			action: actionCopy,
			expErr: errs.ErrInvalidReference,
		},
		{
			name: "InvalidTargetImage",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v1",
				Target: "InvalidTestmissing:v1:garbage",
				Type:   "image",
			},
			action: actionCopy,
			expErr: errs.ErrInvalidReference,
		},
		{
			name: "InvalidSourceRepository",
			sync: ConfigSync{
				Source: "InvalidTestmissing:garbage",
				Target: tsHost + "/testrepo",
				Type:   "repository",
			},
			action: actionCopy,
			expErr: errs.ErrInvalidReference,
		},
		{
			name: "InvalidTargetRepository",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: "InvalidTestmissing:garbage",
				Type:   "repository",
			},
			action: actionCopy,
			expErr: errs.ErrInvalidReference,
		},
		{
			name: "InvalidType",
			sync: ConfigSync{
				Source: tsHost + "/testrepo:v1",
				Target: tsHost + "/test1:v1",
				Type:   "invalid",
			},
			action: actionCopy,
			expErr: ErrInvalidInput,
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
			// validate expected digests, refs that exist, and don't exist
			for tag, d := range tc.expect {
				r, err := ref.New(tag)
				if err != nil {
					t.Fatalf("cannot parse ref %s: %v", tag, err)
				}
				m, err := rc.ManifestHead(ctx, r)
				if err != nil {
					t.Errorf("ref does not exist: %s", tag)
				} else if m.GetDescriptor().Digest != d {
					t.Errorf("digest mismatch for %s, expected %s, received %s", tag, d.String(), m.GetDescriptor().Digest.String())
				}
			}
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
			for _, missing := range tc.missing {
				r, err := ref.New(missing)
				if err != nil {
					t.Fatalf("cannot parse ref %s: %v", missing, err)
				}
				_, err = rc.ManifestHead(ctx, r)
				if err == nil {
					t.Errorf("ref exists that should be missing: %s", missing)
				}
			}
		})
	}
}

func TestProcessRef(t *testing.T) {
	ctx := context.Background()
	// setup tempDir
	tempDir := t.TempDir()
	err := copyfs.Copy(tempDir+"/testrepo", "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to copyfs to tempdir: %v", err)
	}
	// setup various globals normally done by loadConf
	rc = regclient.New()
	cs := ConfigSync{
		Source: "ocidir://" + tempDir + "/testrepo",
		Target: "ocidir://" + tempDir + "/testdest",
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
			src = src.SetTag(tc.src)
			tgt = tgt.SetTag(tc.tgt)
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
