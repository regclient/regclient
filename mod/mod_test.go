package mod

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

func TestMod(t *testing.T) {
	ctx := context.Background()
	// copy testdata images into memory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	tTime, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
	if err != nil {
		t.Errorf("failed to parse test time: %v", err)
	}
	bDig := digest.FromString("digest for base image")
	bRef, err := ref.New("base:latest")
	if err != nil {
		t.Errorf("failed to parse base image: %v", err)
	}
	// create regclient
	rc := regclient.New(regclient.WithFS(fsMem))

	rTgt1, err := ref.New("ocidir://tgtrepo:v1")
	if err != nil {
		t.Errorf("failed to parse ref: %v", err)
	}
	rb1, err := ref.New("ocidir://testrepo:b1")
	if err != nil {
		t.Errorf("failed to parse ref: %v", err)
	}
	rb2, err := ref.New("ocidir://testrepo:b2")
	if err != nil {
		t.Errorf("failed to parse ref: %v", err)
	}
	rb3, err := ref.New("ocidir://testrepo:b3")
	if err != nil {
		t.Errorf("failed to parse ref: %v", err)
	}
	// r1, err := ref.New("ocidir://testrepo:v1")
	// if err != nil {
	// 	t.Errorf("failed to parse ref: %v", err)
	// }
	// r2, err := ref.New("ocidir://testrepo:v2")
	// if err != nil {
	// 	t.Errorf("failed to parse ref: %v", err)
	// }
	r3, err := ref.New("ocidir://testrepo:v3")
	if err != nil {
		t.Errorf("failed to parse ref: %v", err)
	}
	m3, err := rc.ManifestGet(ctx, r3)
	if err != nil {
		t.Errorf("failed to retrieve v3 ref: %v", err)
	}
	pAMD, err := platform.Parse("linux/amd64")
	if err != nil {
		t.Errorf("failed to parse platform: %v", err)
	}
	m3DescAmd, err := manifest.GetPlatformDesc(m3, &pAMD)
	if err != nil {
		t.Errorf("failed to get amd64 descriptor: %v", err)
	}
	r3amd, err := ref.New(fmt.Sprintf("ocidir://testrepo@%s", m3DescAmd.Digest.String()))
	if err != nil {
		t.Errorf("failed to parse platform specific descriptor: %v", err)
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
				WithConfigTimestampMax(tTime),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Config Time Artifact",
			opts: []Opts{
				WithConfigTimestampMax(tTime),
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
					Set: tTime,
				}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Config Time Base Ref",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:     tTime,
					BaseRef: rb1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Config Time Base Count",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:        tTime,
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
					Set: tTime,
				}),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "Config Time After Unchanged",
			opts: []Opts{
				WithConfigTimestamp(OptTime{
					Set:   tTime,
					After: time.Now(),
				}),
			},
			ref:      "ocidir://testrepo:v1",
			wantSame: true,
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
				WithLayerTimestampMax(tTime),
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
				WithLayerTimestampMax(tTime),
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
					Set: tTime,
				}),
			},
			ref: "ocidir://testrepo:v1",
		},
		{
			name: "Layer Timestamp After Unchanged",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:   tTime,
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
					Set:     tTime,
					BaseRef: rb1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Timestamp Base Ref Same",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:     tTime,
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
					Set:        tTime,
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
					Set:        tTime,
					BaseLayers: 1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer Timestamp Artifact",
			opts: []Opts{
				WithLayerTimestamp(OptTime{
					Set:   tTime,
					After: tTime,
				}),
			},
			ref:      "ocidir://testrepo:a1",
			wantSame: true,
		},
		{
			name: "Layer File Tar Time Max",
			opts: []Opts{
				WithFileTarTimeMax("/dir/layer.tar", tTime),
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
					Set:     tTime,
					BaseRef: rb1,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer File Tar Time After",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:   tTime,
					After: tTime,
				}),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Layer File Tar Time Same Base",
			opts: []Opts{
				WithFileTarTime("/dir/layer.tar", OptTime{
					Set:     tTime,
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
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Rebase with annotations v3",
			opts: []Opts{
				WithRebase(),
			},
			ref: "ocidir://testrepo:v3",
		},
		{
			name: "Rebase missing annotations",
			opts: []Opts{
				WithRebase(),
			},
			ref:     "ocidir://testrepo:v1",
			wantErr: types.ErrMissingAnnotation,
		},
		{
			name: "Rebase refs",
			opts: []Opts{
				WithRebaseRefs(rb1, rb2),
			},
			ref: "ocidir://testrepo:v2",
		},
		{
			name: "Rebase mismatch",
			opts: []Opts{
				WithRebaseRefs(rb2, rb3),
			},
			ref:     "ocidir://testrepo:v3",
			wantErr: types.ErrMismatch,
		},
		{
			name: "Rebase mismatch",
			opts: []Opts{
				WithRebaseRefs(rb3, rb2),
			},
			ref:     "ocidir://testrepo:v3",
			wantErr: types.ErrMismatch,
		},
	}

	// run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ref.New(tt.ref)
			if err != nil {
				t.Errorf("failed creating ref: %v", err)
				return
			}
			// run mod with opts
			rMod, err := Apply(ctx, rc, r, tt.opts...)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ModImage did not fail")
				} else if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Errorf("unexpected error, wanted %v, received %v", tt.wantErr, err)
				}
				return
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			mSrc, err := rc.ManifestHead(ctx, r, regclient.WithManifestRequireDigest())
			if err != nil {
				t.Errorf("failed to get manifest from src: %v", err)
				return
			}
			mTgt, err := rc.ManifestHead(ctx, rMod, regclient.WithManifestRequireDigest())
			if err != nil {
				t.Errorf("failed to get manifest from mod \"%s\": %v", rMod.CommonName(), err)
				return
			}

			if tt.wantSame {
				if !mSrc.GetDescriptor().Equal(mTgt.GetDescriptor()) {
					t.Errorf("digest changed")
				}
			} else {
				if mSrc.GetDescriptor().Equal(mTgt.GetDescriptor()) {
					t.Errorf("digest did not change")
				}
			}
		})
	}
}

func TestInList(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		if !inListStr(types.MediaTypeDocker2LayerGzip, mtWLTar) {
			t.Errorf("did not find docker layer in tar whitelist")
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		if inListStr(types.MediaTypeDocker2LayerGzip, mtWLConfig) {
			t.Errorf("found docker layer in config whitelist")
		}
	})
}
