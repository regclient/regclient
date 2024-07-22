package mod

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/pkg/archive"
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
	tarBytes, err := os.ReadFile("../testdata/layer.tar")
	if err != nil {
		t.Fatalf("failed to read testdata/layer.tar: %v", err)
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
	regTgt := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../testdata",
		},
	})
	tTgt := httptest.NewServer(regTgt)
	tTgtURL, _ := url.Parse(tTgt.URL)
	tTgtHost := tTgtURL.Host
	t.Cleanup(func() {
		tSrc.Close()
		_ = regSrc.Close()
		tTgt.Close()
		_ = regTgt.Close()
	})
	tempDir := t.TempDir()
	err = copyfs.Copy(filepath.Join(tempDir, "testrepo"), "../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to setup tempDir: %v", err)
	}

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
		{
			Name:      "registry.example.org",
			Hostname:  tSrcHost,
			TLS:       config.TLSDisabled,
			ReqPerSec: 1000,
		},
	}
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := regclient.New(
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
	rb1, err := ref.New("registry.example.org/testrepo:b1")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rb2, err := ref.New("registry.example.org/testrepo:b2")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rb3, err := ref.New("registry.example.org/testrepo:b3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	// r1, err := ref.New("registry.example.org/testrepo:v1")
	// if err != nil {
	// 	t.Fatalf("failed to parse ref: %v", err)
	// }
	// r2, err := ref.New("registry.example.org/testrepo:v2")
	// if err != nil {
	// 	t.Fatalf("failed to parse ref: %v", err)
	// }
	r3, err := ref.New(tTgtHost + "/testrepo:v3")
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
	r3amd, err := ref.New(fmt.Sprintf("%s/testrepo@%s", tTgtHost, m3DescAmd.Digest.String()))
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
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "To OCI Copy",
			opts: []Opts{
				WithManifestToOCI(),
				WithRefTgt(rTgt1),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "To Docker Copy",
			opts: []Opts{
				WithManifestToDocker(),
				WithRefTgt(rTgt1),
			},
			ref: tTgtHost + "/testrepo:v1",
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
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Annotation",
			opts: []Opts{
				WithAnnotation("test", "hello"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Annotation All",
			opts: []Opts{
				WithAnnotation("[*]test", "hello"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Annotation AMD64/ARM64",
			opts: []Opts{
				WithAnnotation("[linux/amd64,linux/arm64]test", "hello"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Annotation Missing",
			opts: []Opts{
				WithAnnotation("[linux/i386,linux/s390x]test", "hello"),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Add Annotation Platform Parse Error",
			opts: []Opts{
				WithAnnotation("[linux/invalid.arch!]test", "hello"),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("failed to parse annotation platform linux/invalid.arch!: invalid platform component invalid.arch! in linux/invalid.arch!"),
		},
		{
			name: "Delete Annotation",
			opts: []Opts{
				WithAnnotation("org.example.version", ""),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Delete Missing Annotation",
			opts: []Opts{
				WithAnnotation("[*]missing", ""),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Add Base Annotations",
			opts: []Opts{
				WithAnnotationOCIBase(bRef, bDig),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Label",
			opts: []Opts{
				WithLabel("test", "hello"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Label to All",
			opts: []Opts{
				WithLabel("[*]test", "hello"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Label AMD64/ARM64",
			opts: []Opts{
				WithLabel("[linux/amd64,linux/arm64]test", "hello"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Add Label Missing",
			opts: []Opts{
				WithLabel("[linux/i386,linux/s390x]test", "hello"),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Add Label Platform Parse Error",
			opts: []Opts{
				WithLabel("[linux/invalid.arch!]test", "hello"),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("failed to parse label platform linux/invalid.arch!: invalid platform component invalid.arch! in linux/invalid.arch!"),
		},
		{
			name: "Delete Label",
			opts: []Opts{
				WithLabel("version", ""),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Delete Missing Label",
			opts: []Opts{
				WithLabel("[*]missing", ""),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Label to Annotation",
			opts: []Opts{
				WithLabelToAnnotation(),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Time",
			opts: []Opts{
				WithConfigTimestampFromLabel("org.opencontainers.image.created"),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("label not found: org.opencontainers.image.created"),
		},
		{
			name: "Config Digest sha256",
			opts: []Opts{
				WithConfigDigestAlgo(digest.SHA256),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Config Digest sha512 ocidir",
			opts: []Opts{
				WithConfigDigestAlgo(digest.SHA512),
			},
			ref: "ocidir://" + tempDir + "/testrepo:v1",
		},
		{
			name: "Config Time",
			opts: []Opts{
				WithConfigTimestampMax(baseTime),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Config Time Artifact",
			opts: []Opts{
				WithConfigTimestampMax(baseTime),
			},
			ref:      tTgtHost + "/testrepo:a1",
			wantSame: true,
		},
		{
			name: "Config Time Unchanged",
			opts: []Opts{
				WithConfigTimestampMax(time.Now()),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Config Time Missing Set",
			opts: []Opts{
				WithConfigTimestamp(OptTime{}),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("WithConfigTimestamp requires a time to set"),
		},
		{
			name: "Config Time Set",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set: baseTime,
				}),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Config Time Base Ref",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:     baseTime,
					BaseRef: rb1,
				}),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Config Time Base Count",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:        baseTime,
					BaseLayers: 1,
				}),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Config Time Label Missing",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					FromLabel: "org.opencontainers.image.created",
				}),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("label not found: org.opencontainers.image.created"),
		},
		{
			name: "Config Time Artifact",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set: baseTime,
				}),
			},
			ref:      tTgtHost + "/testrepo:a1",
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
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Config Platform",
			opts: []Opts{
				WithConfigPlatform(plat),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Digest sha256",
			opts: []Opts{
				WithDigestAlgo(digest.SHA256),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Digest sha512 ocidir",
			opts: []Opts{
				WithDigestAlgo(digest.SHA512),
			},
			ref: "ocidir://" + tempDir + "/testrepo:v1",
		},
		{
			name: "Expose Port",
			opts: []Opts{
				WithExposeAdd("8080"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Expose Port Delete Unchanged",
			opts: []Opts{
				WithExposeRm("8080"),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Expose Port Artifact",
			opts: []Opts{
				WithExposeAdd("8080"),
			},
			ref:      tTgtHost + "/testrepo:a1",
			wantSame: true,
		},
		{
			name: "External layer remove unchanged",
			opts: []Opts{
				WithExternalURLsRm(),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Add",
			opts: []Opts{
				WithLayerAddTar(bytes.NewReader(tarBytes), "", nil),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Layer Uncompressed",
			opts: []Opts{
				WithLayerCompression(archive.CompressNone),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Layer Compressed gzip",
			opts: []Opts{
				WithLayerCompression(archive.CompressGzip),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Compressed zstd",
			opts: []Opts{
				WithLayerCompression(archive.CompressZstd),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Layer Digest sha256",
			opts: []Opts{
				WithLayerDigestAlgo(digest.SHA256),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Digest sha512 ocidir",
			opts: []Opts{
				WithLayerDigestAlgo(digest.SHA512),
			},
			ref: "ocidir://" + tempDir + "/testrepo:v1",
		},
		// TODO(bmitch): enable when registry support is added
		// {
		// 	name: "Layer Digest sha512 registry",
		// 	opts: []Opts{
		// 		WithLayerDigestAlgo(digest.SHA512),
		// 	},
		// 	ref: tTgtHost + "/testrepo:v1",
		// },
		{
			name: "Layer Reproducible",
			opts: []Opts{
				WithLayerReproducible(),
			},
			ref:      tTgtHost + "/testrepo:v3",
			wantSame: true,
		},
		{
			name: "Layer Timestamp Missing Label",
			opts: []Opts{
				WithLayerTimestampFromLabel("missing"),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("label not found: missing"),
		},
		{
			name: "Layer Timestamp",
			opts: []Opts{
				WithLayerTimestampMax(baseTime),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Layer Timestamp Unchanged",
			opts: []Opts{
				WithLayerTimestampMax(time.Now()),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Layer Timestamp Artifact",
			opts: []Opts{
				WithLayerTimestampMax(baseTime),
			},
			ref:      tTgtHost + "/testrepo:a1",
			wantSame: true,
		},
		{
			name: "Layer Trim File",
			opts: []Opts{
				WithLayerStripFile("/layer2"),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer Trim File With Local Separator",
			opts: []Opts{
				WithLayerStripFile(string(filepath.Separator) + "layer2"),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer Timestamp Set Missing",
			opts: []Opts{
				WithLayerTimestamp(OptTime{}),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("WithLayerTimestamp requires a time to set"),
		},
		{
			name: "Layer Timestamp Missing Label",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					FromLabel: "missing",
				}),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: fmt.Errorf("label not found: missing"),
		},
		{
			name: "Layer Timestamp",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set: baseTime,
				}),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Layer Timestamp After Unchanged",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:   baseTime,
					After: time.Now(),
				}),
			},
			ref:      tTgtHost + "/testrepo:v1",
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
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer Timestamp Base Ref Same",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:     baseTime,
					BaseRef: r3,
				}),
			},
			ref:      tTgtHost + "/testrepo:v3",
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
			ref:      tTgtHost + "/testrepo:v3",
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
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer Timestamp Artifact",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:   baseTime,
					After: baseTime,
				}),
			},
			ref:      tTgtHost + "/testrepo:a1",
			wantSame: true,
		},
		{
			name: "Layer File Tar Time Max",
			opts: []Opts{
				WithFileTarTimeMax("/dir/layer.tar", baseTime),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer File Tar Time Max Unchanged",
			opts: []Opts{
				WithFileTarTimeMax("/dir/layer.tar", time.Now()),
			},
			ref:      tTgtHost + "/testrepo:v3",
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
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer File Tar Time After",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:   baseTime,
					After: baseTime,
				}),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer File Tar Time Same Base",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:     baseTime,
					BaseRef: r3,
				}),
			},
			ref:      tTgtHost + "/testrepo:v3",
			wantSame: true,
		},
		{
			name: "Layer Trim By Created RE",
			opts: []Opts{
				WithLayerRmCreatedBy(*regexp.MustCompile("^COPY layer2.txt /layer2")),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Layer Remove by index from Index",
			opts: []Opts{
				WithLayerRmIndex(1),
			},
			ref:     tTgtHost + "/testrepo:v3",
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
			name: "Manifest Digest sha256",
			opts: []Opts{
				WithManifestDigestAlgo(digest.SHA256),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Manifest Digest sha512 ocidir",
			opts: []Opts{
				WithManifestDigestAlgo(digest.SHA512),
			},
			ref: "ocidir://" + tempDir + "/testrepo:v1",
		},
		{
			name: "Add volume",
			opts: []Opts{
				WithVolumeAdd("/new"),
			},
			ref: tTgtHost + "/testrepo:v2",
		},
		{
			name: "Add volume again",
			opts: []Opts{
				WithVolumeAdd("/volume"),
			},
			ref:      tTgtHost + "/testrepo:v2",
			wantSame: true,
		},
		{
			name: "Rm volume",
			opts: []Opts{
				WithVolumeRm("/volume"),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Rm volume missing",
			opts: []Opts{
				WithVolumeRm("/test"),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Data field",
			opts: []Opts{
				WithData(2048),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Remove data config",
			opts: []Opts{
				WithData(0),
			},
			ref: tTgtHost + "/testrepo:a-example",
		},
		{
			name: "Keep data config",
			opts: []Opts{
				WithData(4),
			},
			ref:      tTgtHost + "/testrepo:a-example",
			wantSame: true,
		},
		{
			name: "Remove Command",
			opts: []Opts{
				WithConfigCmd([]string{}),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Set Command",
			opts: []Opts{
				WithConfigCmd([]string{"/app", "-v"}),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Set Command Shell",
			opts: []Opts{
				WithConfigCmd([]string{"/bin/sh", "-c", "/app -v"}),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Remove Entrypoint",
			opts: []Opts{
				WithConfigEntrypoint([]string{}),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Set Entrypoint",
			opts: []Opts{
				WithConfigEntrypoint([]string{"/app", "-v"}),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Build arg rm",
			opts: []Opts{
				WithBuildArgRm("arg_label", regexp.MustCompile("arg_for_[a-z]*")),
			},
			ref: tTgtHost + "/testrepo:v1",
		},
		{
			name: "Build arg with value rm",
			opts: []Opts{
				WithBuildArgRm("arg_label", regexp.MustCompile("arg_for_[a-z]*")),
			},
			ref: tTgtHost + "/testrepo:v2",
		},
		{
			name: "Build arg missing",
			opts: []Opts{
				WithBuildArgRm("no_such_arg", regexp.MustCompile("no_such_value")),
			},
			ref:      tTgtHost + "/testrepo:v1",
			wantSame: true,
		},
		{
			name: "Rebase with annotations v2",
			opts: []Opts{
				WithRebase(),
				WithRefTgt(rTgt2),
			},
			ref: tTgtHost + "/testrepo:v2",
		},
		{
			name: "Rebase with annotations v3",
			opts: []Opts{
				WithRebase(),
				WithRefTgt(rTgt2),
			},
			ref: tTgtHost + "/testrepo:v3",
		},
		{
			name: "Rebase missing annotations",
			opts: []Opts{
				WithRebase(),
			},
			ref:     tTgtHost + "/testrepo:v1",
			wantErr: errs.ErrMissingAnnotation,
		},
		{
			name: "Rebase refs",
			opts: []Opts{
				WithRebaseRefs(rb1, rb2),
				WithRefTgt(rTgt2),
			},
			ref: tTgtHost + "/testrepo:v2",
		},
		{
			name: "Rebase mismatch",
			opts: []Opts{
				WithRebaseRefs(rb2, rb3),
			},
			ref:     tTgtHost + "/testrepo:v3",
			wantErr: errs.ErrMismatch,
		},
		{
			name: "Rebase mismatch",
			opts: []Opts{
				WithRebaseRefs(rb3, rb2),
			},
			ref:     tTgtHost + "/testrepo:v3",
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
			ref: tTgtHost + "/testrepo:v2",
		},
		{
			name: "Pull up labels and common annotations v1",
			opts: []Opts{
				WithManifestToOCIReferrers(),
				WithLabelToAnnotation(),
				WithAnnotationPromoteCommon(),
			},
			ref: tTgtHost + "/testrepo:v1",
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
			ref: tTgtHost + "/testrepo:v2",
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
		if !inListStr(mediatype.Docker2LayerGzip, mtKnownTar) {
			t.Errorf("did not find docker layer in known tar list")
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		if inListStr(mediatype.Docker2LayerGzip, mtKnownConfig) {
			t.Errorf("found docker layer in known config list")
		}
	})
}
