package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/internal/pqueue"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

func TestProcess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boolT := true
	var err error
	tempDir := t.TempDir()
	err = copyfs.Copy(tempDir+"/testrepo", "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to copy testrepo to tempdir: %v", err)
	}
	err = copyfs.Copy(tempDir+"/external", "../../testdata/external")
	if err != nil {
		t.Fatalf("failed to copy external to tempdir: %v", err)
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
	regROHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../testdata",
			ReadOnly:  &boolT,
		},
	})
	tsRO := httptest.NewServer(regROHandler)
	tsROURL, _ := url.Parse(tsRO.URL)
	tsROHost := tsROURL.Host
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
			Name:     tsROHost,
			Hostname: tsROHost,
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
	rc := regclient.New(
		regclient.WithConfigHost(rcHosts...),
		regclient.WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)
	pq := pqueue.New(pqueue.Opts[throttle]{Max: 1})
	confBytes := `
version: 1
defaults:
  parallel: 1
`
	confRdr := bytes.NewReader([]byte(confBytes))
	conf, err := ConfigLoadReader(confRdr)
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
	ra1, err := ref.New(tsHost + "/testrepo:a1")
	if err != nil {
		t.Fatalf("failed to parse a1 reference: %v", err)
	}
	ra2, err := ref.New(tsHost + "/testrepo:a2")
	if err != nil {
		t.Fatalf("failed to parse a2 reference: %v", err)
	}
	ra3, err := ref.New(tsHost + "/testrepo:a3")
	if err != nil {
		t.Fatalf("failed to parse a3 reference: %v", err)
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
	rExt, err := ref.New(tsHost + "/external")
	if err != nil {
		t.Fatalf("failed to parse external reference: %v", err)
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
	desc2Ext, err := rc.ReferrerList(ctx, r2, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: "application/example.sbom"}), scheme.WithReferrerSource(rExt))
	if err != nil || len(desc2Ext.Descriptors) < 2 {
		t.Fatalf("failed to get external artifacts for v2: %v", err)
	}
	d2Ext1 := desc2Ext.Descriptors[0].Digest
	d2Ext2 := desc2Ext.Descriptors[1].Digest
	m3, err := rc.ManifestGet(ctx, r3)
	if err != nil {
		t.Fatalf("failed to get manifest v3: %v", err)
	}
	d3 := m3.GetDescriptor().Digest
	ma1, err := rc.ManifestGet(ctx, ra1)
	if err != nil {
		t.Fatalf("failed to get manifest a1: %v", err)
	}
	da1 := ma1.GetDescriptor().Digest
	ma2, err := rc.ManifestGet(ctx, ra2)
	if err != nil {
		t.Fatalf("failed to get manifest a2: %v", err)
	}
	da2 := ma2.GetDescriptor().Digest
	ma3, err := rc.ManifestGet(ctx, ra3)
	if err != nil {
		t.Fatalf("failed to get manifest a3: %v", err)
	}
	da3 := ma3.GetDescriptor().Digest
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
		name       string
		sync       ConfigSync
		action     actionType
		abortOnErr bool
		expect     map[string]digest.Digest
		exists     []string
		missing    []string
		expErr     error
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
			name: "ReadOnly Error Abort",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsROHost + "/test-readonly",
				Type:   "repository",
			},
			action:     actionCopy,
			abortOnErr: true,
			expErr:     errs.ErrHTTPStatus,
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
				Tags: TagAllowDeny{
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
				Tags: TagAllowDeny{
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
			name: "RepoSemver",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsHost + "/testsemver",
				Type:   "repository",
				Tags: TagAllowDeny{
					SemverRange: []string{">=v2"},
				},
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/testsemver:v2": d2,
				tsHost + "/testsemver:v3": d3,
			},
			missing: []string{
				tsHost + "/testsemver:v1",
				tsHost + "/testsemver:a1",
				tsHost + "/testsemver:a2",
				tsHost + "/testsemver:a3",
				tsHost + "/testsemver:b1",
				tsHost + "/testsemver:b2",
				tsHost + "/testsemver:b3",
				tsHost + "/testsemver:loop",
			},
			expErr: nil,
		},
		{
			name: "RepoTagSet",
			sync: ConfigSync{
				Source: tsHost + "/testrepo",
				Target: tsHost + "/testset",
				Type:   "repository",
				TagSets: []TagAllowDeny{
					{Allow: []string{"a.*", "loop"}},
					// {SemverRange: []string{">=v2"}},
				},
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				// tsHost + "/testset:v2":   d2,
				// tsHost + "/testset:v3":   d3,
				tsHost + "/testset:a1":   da1,
				tsHost + "/testset:a2":   da2,
				tsHost + "/testset:a3":   da3,
				tsHost + "/testset:loop": dLoop,
			},
			missing: []string{
				tsHost + "/testset:v1",
				tsHost + "/testset:b1",
				tsHost + "/testset:b2",
				tsHost + "/testset:b3",
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
				Tags: TagAllowDeny{
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
			name: "ImageReferrersExtSrc",
			sync: ConfigSync{
				Source:    tsHost + "/testrepo:v2",
				Target:    tsHost + "/test-referrer-ext1:v2",
				Type:      "image",
				Referrers: &boolT,
				ReferrerFilters: []ConfigReferrerFilter{
					{
						ArtifactType: "application/example.sbom",
					},
				},
				ReferrerSrc: tsHost + "/external",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-referrer-ext1:v2": d2,
			},
			exists: []string{
				tsHost + "/test-referrer-ext1@" + d2AMD.String(),
				tsHost + "/test-referrer-ext1@" + d2Ext1.String(),
				tsHost + "/test-referrer-ext1@" + d2Ext2.String(),
			},
			missing: []string{
				tsHost + "/test-referrer-ext1@" + d2SBOM.String(),
				tsHost + "/test-referrer-ext1@" + d2Sig.String(),
				tsHost + "/test-referrer-ext1@" + d1.String(),
				tsHost + "/test-referrer-ext1@" + d3.String(),
			},
			expErr: nil,
		},
		{
			name: "ImageReferrersExtBoth",
			sync: ConfigSync{
				Source:    tsHost + "/testrepo:v2",
				Target:    tsHost + "/test-referrer-ext2:v2",
				Type:      "image",
				Referrers: &boolT,
				ReferrerFilters: []ConfigReferrerFilter{
					{
						ArtifactType: "application/example.sbom",
					},
				},
				ReferrerSrc: tsHost + "/external",
				ReferrerTgt: tsHost + "/test-referrer-ext2-tgt",
			},
			action: actionCopy,
			expect: map[string]digest.Digest{
				tsHost + "/test-referrer-ext2:v2": d2,
			},
			exists: []string{
				tsHost + "/test-referrer-ext2@" + d2AMD.String(),
				tsHost + "/test-referrer-ext2-tgt@" + d2Ext1.String(),
				tsHost + "/test-referrer-ext2-tgt@" + d2Ext2.String(),
			},
			missing: []string{
				tsHost + "/test-referrer-ext2@" + d2SBOM.String(),
				tsHost + "/test-referrer-ext2@" + d2Sig.String(),
				tsHost + "/test-referrer-ext2@" + d1.String(),
				tsHost + "/test-referrer-ext2@" + d3.String(),
				tsHost + "/test-referrer-ext2@" + d2Ext1.String(),
				tsHost + "/test-referrer-ext2@" + d2Ext2.String(),
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
			rootOpts := rootOpts{
				conf:       conf,
				rc:         rc,
				throttle:   pq,
				log:        slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
				abortOnErr: tc.abortOnErr,
			}
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
	t.Parallel()
	ctx := context.Background()
	// setup tempDir
	tempDir := t.TempDir()
	err := copyfs.Copy(tempDir+"/testrepo", "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to copyfs to tempdir: %v", err)
	}
	// setup various globals normally done by loadConf
	rc := regclient.New()
	cs := ConfigSync{
		Source: "ocidir://" + tempDir + "/testrepo",
		Target: "ocidir://" + tempDir + "/testdest",
		Type:   "repository",
	}
	syncSetDefaults(&cs, ConfigDefaults{})

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
			rootOpts := rootOpts{
				rc: rc,
				conf: &Config{
					Sync: []ConfigSync{cs},
				},
				log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
			}
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

// TestFilterListVersionScheme tests the integration of semver filtering with tag filtering.
// This focuses on real-world scenarios including:
// - Tag patterns with suffixes (alpine, scratch, debian, etc.)
// - Mixed version formats (v1, v1.5, v1.5.0)
// - Multiple ranges and edge cases specific to container image tags
// Note: Basic semver constraint tests are in internal/semver/semver_test.go
func TestFilterListVersionScheme(t *testing.T) {
	tests := []struct {
		name        string
		ad          TagAllowDeny
		input       []string
		expected    []string
		expectError bool
	}{
		{
			name: "semver with multiple ranges and deny",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <2.0.0", ">=4.0.0"},
				Deny:        []string{".*-rc.*"},
			},
			input:    []string{"1.0.0", "1.5.0-rc1", "2.0.0", "4.0.0", "4.1.0-rc2", "5.0.0"},
			expected: []string{"1.0.0", "4.0.0", "5.0.0"},
		},
		{
			name: "semver filters non-semver tags",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0"},
			},
			input:    []string{"latest", "dev", "1.0.0", "1.5.0", "main"},
			expected: []string{"1.0.0", "1.5.0"},
		},
		{
			name: "semver with allow/deny combination",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <3.0.0"},
				Deny:        []string{".*-rc.*"},
			},
			input:    []string{"1.0.0", "1.5.0-rc1", "2.0.0", "2.1.0-rc2", "3.0.0"},
			expected: []string{"1.0.0", "2.0.0"},
		},
		{
			name: "no version range (backward compatibility)",
			ad: TagAllowDeny{
				Allow: []string{"v[0-9]+\\.[0-9]+\\.[0-9]+"},
			},
			input:    []string{"v1.0.0", "v1.5.0", "latest", "dev"},
			expected: []string{"v1.0.0", "v1.5.0"},
		},
		{
			name: "empty result when no matches",
			ad: TagAllowDeny{
				SemverRange: []string{">=5.0.0"},
			},
			input:    []string{"1.0.0", "2.0.0", "3.0.0"},
			expected: []string{},
		},
		{
			name: "multiple ranges skip middle versions",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <1.20.0", ">=1.22.0"},
			},
			input:    []string{"1.0.0", "1.19.0", "1.20.0", "1.21.0", "1.22.0", "1.23.0"},
			expected: []string{"1.0.0", "1.19.0", "1.22.0", "1.23.0"},
		},
		{
			name: "version range with allow filter",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0"},
				Allow:       []string{"v.*"},
			},
			input:    []string{"1.0.0", "v1.5.0", "v2.0.0", "3.0.0"},
			expected: []string{"v1.5.0", "v2.0.0"}, // sequential: semver first (all 4), then allow filters by v.*
		},
		{
			name: "empty version range array",
			ad: TagAllowDeny{
				SemverRange: []string{},
			},
			input:    []string{"1.0.0", "2.0.0"},
			expected: []string{"1.0.0", "2.0.0"},
		},
		{
			name: "semver with suffix alpine",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <2.0.0"},
			},
			input:    []string{"v1.2.3-alpine3.21", "v1.5.0-alpine3.20", "v2.0.0-alpine3.21", "v0.9.0-alpine3.19"},
			expected: []string{"v1.2.3-alpine3.21", "v1.5.0-alpine3.20", "v2.0.0-alpine3.21"},
		},
		{
			name: "semver with suffix scratch",
			ad: TagAllowDeny{
				SemverRange: []string{">=5.0.0"},
			},
			input:    []string{"v5-scratch", "v4-scratch", "v6-scratch", "v5.1-scratch"},
			expected: []string{"v6-scratch", "v5.1-scratch"},
		},
		{
			name: "semver with suffix mixed",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <3.0.0"},
			},
			input:    []string{"v1.0.0", "v1.5.0-alpine", "v2.0.0-scratch", "v2.5.1-debian", "v3.0.0-alpine"},
			expected: []string{"v1.0.0", "v1.5.0-alpine", "v2.0.0-scratch", "v2.5.1-debian", "v3.0.0-alpine"},
		},
		{
			name: "semver major version only",
			ad: TagAllowDeny{
				SemverRange: []string{">=2 <5"},
			},
			input:    []string{"v1", "v2", "v3", "v4", "v5", "v6"},
			expected: []string{"v2", "v3", "v4"},
		},
		{
			name: "semver major.minor only",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.5 <2.0"},
			},
			input:    []string{"v1.4", "v1.5", "v1.6", "v1.9", "v2.0", "v2.1"},
			expected: []string{"v1.5", "v1.6", "v1.9"},
		},
		{
			name: "semver mixed version formats",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <3.0.0"},
			},
			input:    []string{"v1", "v1.5", "v1.5.0", "v2", "v2.0", "v2.0.0", "v3", "v3.0.0"},
			expected: []string{"v1", "v1.5", "v1.5.0", "v2", "v2.0", "v2.0.0"},
		},
		{
			name: "semver with deny on suffixes",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0"},
				Deny:        []string{".*-alpine.*"},
			},
			input:    []string{"v1.0.0", "v1.2.3-alpine3.21", "v2.0.0-scratch", "v2.5.0-alpine3.20"},
			expected: []string{"v1.0.0", "v2.0.0-scratch"},
		},
		{
			name: "semver + allow adds non-semver tags",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0 <2.0.0"},
				Allow:       []string{"latest", "edge"},
			},
			input:    []string{"0.9.0", "1.0.0", "1.5.0", "2.0.0", "latest", "edge", "dev"},
			expected: []string{}, // sequential: semver filters out non-semver tags, so allow has nothing to match
		},
		{
			name: "semver + allow + deny combines all filters",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0"},
				Allow:       []string{"latest", "stable"},
				Deny:        []string{".*-rc.*", "latest"},
			},
			input:    []string{"1.0.0", "1.5.0-rc1", "2.0.0", "latest", "stable", "dev"},
			expected: []string{}, // sequential: semver filters out non-semver "latest", "stable", "dev"
		},
		{
			name: "allow without semver still works (backward compatible)",
			ad: TagAllowDeny{
				Allow: []string{"v[0-9]+\\.[0-9]+"},
			},
			input:    []string{"v1.0", "v1.5", "v2.0", "latest", "edge"},
			expected: []string{"v1.0", "v1.5", "v2.0"},
		},
		{
			name: "semver + allow with overlapping matches",
			ad: TagAllowDeny{
				SemverRange: []string{">=1.0.0"},
				Allow:       []string{"v[0-9]+\\.[0-9]+\\.[0-9]+"},
			},
			input:    []string{"v1.0.0", "v1.5.0", "v2.0.0", "latest"},
			expected: []string{"v1.0.0", "v1.5.0", "v2.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filterTagList(tt.ad, tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d results, got %d\nexpected: %v\ngot: %v",
					len(tt.expected), len(result), tt.expected, result)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("result[%d]: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestConfigRead(t *testing.T) {
	t.Parallel()
	bFalse := false
	tt := []struct {
		name   string
		file   string
		expect Config
		expErr error
	}{
		{
			name: "config1",
			file: "config1.yml",
			expect: Config{
				Version: 1,
				Creds: []config.Host{
					{
						Name: "registry:5000",
						TLS:  config.TLSDisabled,
					},
					{
						Name: "docker.io",
					},
				},
				Defaults: ConfigDefaults{
					RateLimit: ConfigRateLimit{
						Min:   100,
						Retry: 15 * time.Minute,
					},
					Parallel:   2,
					Interval:   60 * time.Minute,
					Backup:     "bkup-{{.Ref.Tag}}",
					CacheCount: 500,
					CacheTime:  5 * time.Minute,
				},
				Sync: []ConfigSync{
					{
						Source:   "busybox:latest",
						Target:   "registry:5000/library/busybox:latest",
						Type:     "image",
						Schedule: "15 3 * * *",
						Backup:   "bkup-{{.Ref.Tag}}",
						RateLimit: ConfigRateLimit{
							Min:   100,
							Retry: 15 * time.Minute,
						},
						MediaTypes:      defaultMediaTypes,
						DigestTags:      &bFalse,
						Referrers:       &bFalse,
						FastCheck:       &bFalse,
						ForceRecursive:  &bFalse,
						IncludeExternal: &bFalse,
					},
					{
						Source: "alpine",
						Target: "registry:5000/hub/alpine",
						Type:   "repository",
						Tags: TagAllowDeny{
							Allow: []string{"3", "3.9", "latest"},
						},
						Interval: 60 * time.Minute,
						Backup:   "bkup-{{.Ref.Tag}}",
						RateLimit: ConfigRateLimit{
							Min:   100,
							Retry: 15 * time.Minute,
						},
						MediaTypes:      defaultMediaTypes,
						DigestTags:      &bFalse,
						Referrers:       &bFalse,
						FastCheck:       &bFalse,
						ForceRecursive:  &bFalse,
						IncludeExternal: &bFalse,
					},
					{
						Source: "gcr.io/example/repo",
						Target: "registry:5000/gcr/example/repo",
						Type:   "repository",
						Tags: TagAllowDeny{
							Allow: []string{"3", "3.9", "latest"},
						},
						Interval: 60 * time.Minute,
						Backup:   "bkup-{{.Ref.Tag}}",
						RateLimit: ConfigRateLimit{
							Min:   100,
							Retry: 15 * time.Minute,
						},
						MediaTypes:      defaultMediaTypes,
						DigestTags:      &bFalse,
						Referrers:       &bFalse,
						FastCheck:       &bFalse,
						ForceRecursive:  &bFalse,
						IncludeExternal: &bFalse,
					},
				},
			},
		},
		{
			name: "config2",
			file: "config2.yml",
			expect: Config{
				Version: 1,
				Creds: []config.Host{
					{
						Name: "registry:5000",
						TLS:  config.TLSDisabled,
					},
				},
				Defaults: ConfigDefaults{
					Schedule: "15 3 * * *",
					RateLimit: ConfigRateLimit{
						Retry: rateLimitRetryMin,
					},
				},
				Sync: []ConfigSync{
					{
						Source:   "busybox:latest",
						Target:   "registry:5000/library/busybox:latest",
						Type:     "image",
						Interval: 12 * time.Hour,
						RateLimit: ConfigRateLimit{
							Retry: rateLimitRetryMin,
						},
						MediaTypes:      defaultMediaTypes,
						DigestTags:      &bFalse,
						Referrers:       &bFalse,
						FastCheck:       &bFalse,
						ForceRecursive:  &bFalse,
						IncludeExternal: &bFalse,
					},
					{
						Source:   "alpine:latest",
						Target:   "registry:5000/library/alpine:latest",
						Type:     "image",
						Schedule: "15 3 * * *",
						RateLimit: ConfigRateLimit{
							Retry: rateLimitRetryMin,
						},
						MediaTypes:      defaultMediaTypes,
						DigestTags:      &bFalse,
						Referrers:       &bFalse,
						FastCheck:       &bFalse,
						ForceRecursive:  &bFalse,
						IncludeExternal: &bFalse,
					},
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cRead, err := ConfigLoadFile(filepath.Join("./testdata", tc.file))
			if tc.expErr != nil {
				if !errors.Is(err, tc.expErr) {
					t.Errorf("expected error %v, received %v", tc.expErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("failed to read: %v", err)
			}
			if !reflect.DeepEqual(tc.expect, *cRead) {
				t.Errorf("parsing mismatch, expected:\n%#v\n  received:\n%#v", tc.expect, *cRead)
			}
		})
	}
}
