package mod

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

func TestMod(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// copy testdata images into memory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../testdata", fsMem, ".")
	if err != nil {
		t.Fatalf("failed to setup memfs copy: %v", err)
	}
	baseTime, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse test time: %v", err)
	}
	oldTime, err := time.Parse(time.RFC3339, "1999-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse test time: %v", err)
	}
	bDig := digest.FromString("digest for base image")
	bRef, err := ref.New("base:latest")
	if err != nil {
		t.Fatalf("failed to parse base image: %v", err)
	}
	bTrue := true
	regSrc := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../testdata",
			ReadOnly:  &bTrue,
		},
	})
	tSrc := httptest.NewServer(regSrc)
	tSrcURL, _ := url.Parse(tSrc.URL)
	tSrcHost := tSrcURL.Host
	t.Cleanup(func() {
		tSrc.Close()
		_ = regSrc.Close()
	})
	regTgt := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
		},
	})
	tTgt := httptest.NewServer(regTgt)
	tTgtURL, _ := url.Parse(tTgt.URL)
	tTgtHost := tTgtURL.Host
	t.Cleanup(func() {
		tTgt.Close()
		_ = regTgt.Close()
	})

	// create regclient
	rcHosts := []config.Host{
		{
			Name:      tSrcHost,
			Hostname:  tSrcHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: 1000,
		},
		{
			Name:      tTgtHost,
			Hostname:  tTgtHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: 1000,
		},
	}
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := regclient.New(
		regclient.WithFS(fsMem),
		regclient.WithConfigHost(rcHosts...),
		regclient.WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)

	rTgt1, err := ref.New(tTgtHost + "/tgtrepo1:v1")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rTgt2, err := ref.New(tTgtHost + "/tgtrepo2:v2")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rTgt3, err := ref.New(tTgtHost + "/tgtrepo3:v3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rb1, err := ref.New("ocidir://testrepo:b1")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rb2, err := ref.New("ocidir://testrepo:b2")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rb3, err := ref.New("ocidir://testrepo:b3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	// r1, err := ref.New("ocidir://testrepo:v1")
	// if err != nil {
	// 	t.Fatalf("failed to parse ref: %v", err)
	// }
	// r2, err := ref.New("ocidir://testrepo:v2")
	// if err != nil {
	// 	t.Fatalf("failed to parse ref: %v", err)
	// }
	r3, err := ref.New("ocidir://testrepo:v3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	m3, err := rc.ManifestGet(ctx, r3)
	if err != nil {
		t.Fatalf("failed to retrieve v3 ref: %v", err)
	}
	pAMD, err := platform.Parse("linux/amd64")
	if err != nil {
		t.Fatalf("failed to parse platform: %v", err)
	}
	m3DescAmd, err := manifest.GetPlatformDesc(m3, &pAMD)
	if err != nil {
		t.Fatalf("failed to get amd64 descriptor: %v", err)
	}
	r3amd, err := ref.New(fmt.Sprintf("ocidir://testrepo@%s", m3DescAmd.Digest.String()))
	if err != nil {
		t.Fatalf("failed to parse platform specific descriptor: %v", err)
	}
	plat, err := platform.Parse("linux/amd64/v3")
	if err != nil {
		t.Fatalf("failed to parse the platform: %v", err)
	}

	// define tests
	tests := []struct {
		name     string
		opts     []Opts
		ref      string
		wantErr  error
		wantSame bool // if the resulting image should be unchanged
	}{
		{
			name: "To OCI",
			opts: []Opts{
				WithManifestToOCI(),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "To OCI Copy",
			opts: []Opts{
				WithManifestToOCI(),
				WithRefTgt(rTgt1),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "To Docker Copy",
			opts: []Opts{
				WithManifestToDocker(),
				WithRefTgt(rTgt1),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Docker To OCI",
			opts: []Opts{
				WithManifestToOCI(),
			},
			ref: rTgt1.CommonName(),
		},
		{
			name: "To OCI Referrers",
			opts: []Opts{
				WithManifestToOCIReferrers(),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Annotation",
			opts: []Opts{
				WithAnnotation("test", "hello"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Annotation All",
			opts: []Opts{
				WithAnnotation("[*]test", "hello"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Annotation AMD64/ARM64",
			opts: []Opts{
				WithAnnotation("[linux/amd64,linux/arm64]test", "hello"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Annotation Missing",
			opts: []Opts{
				WithAnnotation("[linux/i386,linux/s390x]test", "hello"),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Add Annotation Platform Parse Error",
			opts: []Opts{
				WithAnnotation("[linux/invalid.arch!]test", "hello"),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("failed to parse annotation platform linux/invalid.arch!: invalid platform component invalid.arch! in linux/invalid.arch!"),
		},
		{
			name: "Delete Annotation",
			opts: []Opts{
				WithAnnotation("org.example.version", ""),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Delete Missing Annotation",
			opts: []Opts{
				WithAnnotation("[*]missing", ""),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Add Base Annotations",
			opts: []Opts{
				WithAnnotationOCIBase(bRef, bDig),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Label",
			opts: []Opts{
				WithLabel("test", "hello"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Label to All",
			opts: []Opts{
				WithLabel("[*]test", "hello"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Label AMD64/ARM64",
			opts: []Opts{
				WithLabel("[linux/amd64,linux/arm64]test", "hello"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Add Label Missing",
			opts: []Opts{
				WithLabel("[linux/i386,linux/s390x]test", "hello"),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Add Label Platform Parse Error",
			opts: []Opts{
				WithLabel("[linux/invalid.arch!]test", "hello"),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("failed to parse label platform linux/invalid.arch!: invalid platform component invalid.arch! in linux/invalid.arch!"),
		},
		{
			name: "Delete Label",
			opts: []Opts{
				WithLabel("version", ""),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Delete Missing Label",
			opts: []Opts{
				WithLabel("[*]missing", ""),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Label to Annotation",
			opts: []Opts{
				WithLabelToAnnotation(),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Time",
			opts: []Opts{
				WithConfigTimestampFromLabel("org.opencontainers.image.created"),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("label not found: org.opencontainers.image.created"),
		},
		{
			name: "Config Time",
			opts: []Opts{
				WithConfigTimestampMax(baseTime),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Config Time Artifact",
			opts: []Opts{
				WithConfigTimestampMax(baseTime),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "Config Time Unchanged",
			opts: []Opts{
				WithConfigTimestampMax(time.Now()),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Config Time Missing Set",
			opts: []Opts{
				WithConfigTimestamp(OptTime{}),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("WithConfigTimestamp requires a time to set"),
		},
		{
			name: "Config Time Set",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set: baseTime,
				}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Config Time Base Ref",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:     baseTime,
					BaseRef: rb1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Config Time Base Count",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:        baseTime,
					BaseLayers: 1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Config Time Label Missing",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					FromLabel: "org.opencontainers.image.created",
				}),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("label not found: org.opencontainers.image.created"),
		},
		{
			name: "Config Time Artifact",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set: baseTime,
				}),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "Config Time After Unchanged",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:   baseTime,
					After: time.Now(),
				}),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Config Platform",
			opts: []Opts{
				WithConfigPlatform(plat),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Expose Port",
			opts: []Opts{
				WithExposeAdd("8080"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Expose Port Delete Unchanged",
			opts: []Opts{
				WithExposeRm("8080"),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Expose Port Artifact",
			opts: []Opts{
				WithExposeAdd("8080"),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "External layer remove unchanged",
			opts: []Opts{
				WithExternalURLsRm(),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Reproducible",
			opts: []Opts{
				WithLayerReproducible(),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Timestamp Missing Label",
			opts: []Opts{
				WithLayerTimestampFromLabel("missing"),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("label not found: missing"),
		},
		{
			name: "Layer Timestamp",
			opts: []Opts{
				WithLayerTimestampMax(baseTime),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Layer Timestamp Unchanged",
			opts: []Opts{
				WithLayerTimestampMax(time.Now()),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Timestamp Artifact",
			opts: []Opts{
				WithLayerTimestampMax(baseTime),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "Layer Trim File",
			opts: []Opts{
				WithLayerStripFile("/layer2"),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Timestamp Set Missing",
			opts: []Opts{
				WithLayerTimestamp(OptTime{}),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("WithLayerTimestamp requires a time to set"),
		},
		{
			name: "Layer Timestamp Missing Label",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					FromLabel: "missing",
				}),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: fmt.Errorf("label not found: missing"),
		},
		{
			name: "Layer Timestamp",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set: baseTime,
				}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Layer Timestamp After Unchanged",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:   baseTime,
					After: time.Now(),
				}),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Timestamp Base Ref",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:     baseTime,
					BaseRef: rb1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Timestamp Base Ref Same",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:     baseTime,
					BaseRef: r3,
				}),
			},
			ref:      "ocidir://testrepo:v3",
			wantSame: true,
		},
		{
			name: "Layer Timestamp Base Count Same",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:        baseTime,
					BaseLayers: 99,
				}),
			},
			ref:      "ocidir://testrepo:v3",
			wantSame: true,
		},
		{
			name: "Layer Timestamp Base Count",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:        baseTime,
					BaseLayers: 1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Timestamp Artifact",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:   baseTime,
					After: baseTime,
				}),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "Layer File Tar Time Max",
			opts: []Opts{
				WithFileTarTimeMax("/dir/layer.tar", baseTime),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer File Tar Time Max Unchanged",
			opts: []Opts{
				WithFileTarTimeMax("/dir/layer.tar", time.Now()),
			},
			ref:      "ocidir://testrepo:v3",
			wantSame: true,
		},
		{
			name: "Layer File Tar Time",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:     baseTime,
					BaseRef: rb1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer File Tar Time After",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:   baseTime,
					After: baseTime,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer File Tar Time Same Base",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:     baseTime,
					BaseRef: r3,
				}),
			},
			ref:      "ocidir://testrepo:v3",
			wantSame: true,
		},
		{
			name: "Layer Trim By Created RE",
			opts: []Opts{
				WithLayerRmCreatedBy(*regexp.MustCompile("^COPY layer2.txt /layer2")),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Remove by index from Index",
			opts: []Opts{
				WithLayerRmIndex(1),
			},
			ref:     "ocidir://testrepo:v3",
			wantErr: fmt.Errorf("remove layer by index requires v2 image manifest"),
		},
		{
			name: "Layer Remove by index",
			opts: []Opts{
				WithLayerRmIndex(1),
			},
			ref: r3amd.CommonName(),
		},
		{
			name: "Layer Remove by index missing",
			opts: []Opts{
				WithLayerRmIndex(10),
			},
			ref:     r3amd.CommonName(),
			wantErr: fmt.Errorf("layer not found"),
		},
		{
			name: "Add volume",
			opts: []Opts{
				WithVolumeAdd("/new"),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Add volume again",
			opts: []Opts{
				WithVolumeAdd("/volume"),
			},
			ref:      "ocidir://testrepo:v2",
			wantSame: true,
		},
		{
			name: "Rm volume",
			opts: []Opts{
				WithVolumeRm("/volume"),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Rm volume missing",
			opts: []Opts{
				WithVolumeRm("/test"),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Data field",
			opts: []Opts{
				WithData(2048),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Remove Command",
			opts: []Opts{
				WithConfigCmd([]string{}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Set Command",
			opts: []Opts{
				WithConfigCmd([]string{"/app", "-v"}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Set Command Shell",
			opts: []Opts{
				WithConfigCmd([]string{"/bin/sh", "-c", "/app -v"}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Remove Entrypoint",
			opts: []Opts{
				WithConfigEntrypoint([]string{}),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Set Entrypoint",
			opts: []Opts{
				WithConfigEntrypoint([]string{"/app", "-v"}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Build arg rm",
			opts: []Opts{
				WithBuildArgRm("arg_label", regexp.MustCompile("arg_for_[a-z]*")),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Build arg with value rm",
			opts: []Opts{
				WithBuildArgRm("arg_label", regexp.MustCompile("arg_for_[a-z]*")),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Build arg missing",
			opts: []Opts{
				WithBuildArgRm("no_such_arg", regexp.MustCompile("no_such_value")),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
		},
		{
			name: "Rebase with annotations v2",
			opts: []Opts{
				WithRebase(),
				WithRefTgt(rTgt2),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Rebase with annotations v3",
			opts: []Opts{
				WithRebase(),
				WithRefTgt(rTgt2),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Rebase missing annotations",
			opts: []Opts{
				WithRebase(),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: errs.ErrMissingAnnotation,
		},
		{
			name: "Rebase refs",
			opts: []Opts{
				WithRebaseRefs(rb1, rb2),
				WithRefTgt(rTgt2),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Rebase mismatch",
			opts: []Opts{
				WithRebaseRefs(rb2, rb3),
			},
			ref:     "ocidir://testrepo:v3",
			wantErr: errs.ErrMismatch,
		},
		{
			name: "Rebase mismatch",
			opts: []Opts{
				WithRebaseRefs(rb3, rb2),
			},
			ref:     "ocidir://testrepo:v3",
			wantErr: errs.ErrMismatch,
		},
		{
			name: "Rebase and backdate",
			opts: []Opts{
				WithRefTgt(rTgt3),
				WithRebase(),
				WithLayerTimestamp(OptTime{
					Set: oldTime,
				}),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Pull up labels and common annotations v1",
			opts: []Opts{
				WithManifestToOCIReferrers(),
				WithLabelToAnnotation(),
				WithAnnotationPromoteCommon(),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Setup Annotations v2",
			opts: []Opts{
				WithAnnotation("[*]common", "annotation on all images"),
				WithAnnotation("[linux/amd64,linux/arm64,linux/arm/v7]child", "annotation on all child images"),
				WithAnnotation("[linux/amd64]unique", "amd64"),
				WithAnnotation("[linux/arm64]unique", "arm64"),
				WithAnnotation("[linux/arm/v7]unique", "arm/v7"),
				WithAnnotation("[linux/amd64]amd64only", "value for amd64"),
				WithRefTgt(rTgt2),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Pull up common annotations v2",
			opts: []Opts{
				WithAnnotationPromoteCommon(),
			},
			ref: rTgt2.CommonName(),
		},
	}

	// run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rSrc, err := ref.New(tt.ref)
			if err != nil {
				t.Fatalf("failed creating ref: %v", err)
			}
			// run mod with opts
			rMod, err := Apply(ctx, rc, rSrc, tt.opts...)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ModImage did not fail")
				} else if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Errorf("unexpected error, wanted %v, received %v", tt.wantErr, err)
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			mSrc, err := rc.ManifestHead(ctx, rSrc, regclient.WithManifestRequireDigest())
			if err != nil {
				t.Fatalf("failed to get manifest from src: %v", err)
			}
			mTgt, err := rc.ManifestHead(ctx, rMod, regclient.WithManifestRequireDigest())
			if err != nil {
				t.Fatalf("failed to get manifest from mod \"%s\": %v", rMod.CommonName(), err)
			}

			if tt.wantSame {
				if mSrc.GetDescriptor().Digest != mTgt.GetDescriptor().Digest {
					t.Errorf("digest changed")
				}
			} else {
				if mSrc.GetDescriptor().Digest == mTgt.GetDescriptor().Digest {
					t.Errorf("digest did not change")
				}
			}
		})
	}
}

func TestInList(t *testing.T) {
	t.Parallel()
	t.Run("match", func(t *testing.T) {
		if !inListStr(mediatype.Docker2LayerGzip, mtWLTar) {
			t.Errorf("did not find docker layer in tar whitelist")
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		if inListStr(mediatype.Docker2LayerGzip, mtWLConfig) {
			t.Errorf("found docker layer in config whitelist")
		}
	})
}
