package ocidir

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func TestReferrer(t *testing.T) {
	ctx := context.Background()
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	log := &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}
	o := New(
		WithFS(fsMem),
		WithLog(log),
	)
	repo := "ocidir://testrepo"
	tagName := "v3"
	aType := "application/example.sbom"
	bType := "application/example.sig"
	cType := "application/example.attestation"
	extraAnnot := "org.opencontainers.artifact.sbom.format"
	extraValueA := "json"
	extraValueB := "yaml"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	mRef, err := ref.New(repo + ":" + tagName)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", repo+":"+tagName, err)
		return
	}
	m, err := o.ManifestGet(ctx, mRef)
	if err != nil {
		t.Errorf("failed to get manifest: %v", err)
	}
	mDigest := m.GetDescriptor().Digest
	tagRef := fmt.Sprintf("%s-%s", mDigest.Algorithm().String(), mDigest.Hex())
	// artifact being attached
	artifactAAnnot := map[string]string{
		extraAnnot: extraValueA,
	}
	mDesc := m.GetDescriptor()
	pAMDStr := "linux/amd64"
	pAMD, err := platform.Parse(pAMDStr)
	if err != nil {
		t.Errorf("failed to parse platform: %v", err)
		return
	}
	mAMDDesc, err := manifest.GetPlatformDesc(m, &pAMD)
	if err != nil {
		t.Errorf("failed to get AMD descriptor: %v", err)
		return
	}
	artifactA := v1.Manifest{
		Versioned: v1.ManifestSchemaVersion,
		MediaType: types.MediaTypeOCI1Manifest,
		Config: types.Descriptor{
			MediaType: aType,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []types.Descriptor{
			{
				MediaType: types.MediaTypeOCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifactAAnnot,
		Subject:     &mDesc,
	}
	artifactAM, err := manifest.New(manifest.WithOrig(artifactA))
	if err != nil {
		t.Errorf("failed creating artifact manifest: %v", err)
	}
	artifactABody, err := artifactAM.RawBody()
	if err != nil {
		t.Errorf("failed extracting raw body from artifact: %v", err)
	}
	artifactBAnnot := map[string]string{
		extraAnnot: extraValueB,
	}
	artifactB := v1.ArtifactManifest{
		MediaType:    types.MediaTypeOCI1Artifact,
		ArtifactType: bType,
		Blobs: []types.Descriptor{
			{
				MediaType: types.MediaTypeOCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifactBAnnot,
		Subject:     &mDesc,
	}
	artifactBM, err := manifest.New(manifest.WithOrig(artifactB))
	if err != nil {
		t.Errorf("failed creating artifact manifest: %v", err)
		return
	}
	artifactBBody, err := artifactBM.RawBody()
	if err != nil {
		t.Errorf("failed extracting raw body from artifact: %v", err)
		return
	}
	artifactC := v1.ArtifactManifest{
		MediaType:    types.MediaTypeOCI1Artifact,
		ArtifactType: cType,
		Blobs: []types.Descriptor{
			{
				MediaType: types.MediaTypeOCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Subject: mAMDDesc,
	}
	artifactCM, err := manifest.New(manifest.WithOrig(artifactC))
	if err != nil {
		t.Errorf("failed creating artifact manifest: %v", err)
		return
	}

	// list empty
	t.Run("List empty", func(t *testing.T) {
		r := mRef
		rl, err := o.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
		}
	})

	// attach to image
	t.Run("Put", func(t *testing.T) {
		r := mRef
		r.Tag = ""
		r.Digest = artifactAM.GetDescriptor().Digest.String()
		err = o.ManifestPut(ctx, r, artifactAM, scheme.WithManifestChild())
		if err != nil {
			t.Errorf("Failed running ManifestPut on Manifest: %v", err)
			return
		}
		err = o.ManifestPut(ctx, r, artifactAM, scheme.WithManifestChild())
		if err != nil {
			t.Errorf("Failed running ManifestPut on Manifest again: %v", err)
			return
		}
		r.Digest = artifactBM.GetDescriptor().Digest.String()
		err = o.ManifestPut(ctx, r, artifactBM, scheme.WithManifestChild())
		if err != nil {
			t.Errorf("Failed running ManifestPut on Artifact: %v", err)
			return
		}
		r.Digest = artifactCM.GetDescriptor().Digest.String()
		err = o.ManifestPut(ctx, r, artifactCM, scheme.WithManifestChild())
		if err != nil {
			t.Errorf("Failed running ManifestPut on Artifact: %v", err)
			return
		}
	})

	// list referrers to v1
	t.Run("List", func(t *testing.T) {
		r, err := ref.New(repo + ":" + tagName)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := o.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 2 {
			t.Errorf("descriptor list length, expected 2, received %d", len(rl.Descriptors))
			return
		}
		// expecting artifact A in index 0
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactABody)) ||
			rl.Descriptors[0].Digest != artifactAM.GetDescriptor().Digest ||
			rl.Descriptors[0].ArtifactType != aType ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAAnnot) {
			t.Errorf("returned descriptor A mismatch: %v", rl.Descriptors[0])
		}
		// expecting artifact B in index 1
		if rl.Descriptors[1].MediaType != types.MediaTypeOCI1Artifact ||
			rl.Descriptors[1].Size != int64(len(artifactBBody)) ||
			rl.Descriptors[1].Digest != artifactBM.GetDescriptor().Digest ||
			rl.Descriptors[1].ArtifactType != bType ||
			!mapStringStringEq(rl.Descriptors[1].Annotations, artifactBAnnot) {
			t.Errorf("returned descriptor B mismatch: %v", rl.Descriptors[1])
		}
		if len(rl.Tags) != 1 || rl.Tags[0] != tagRef {
			t.Errorf("tag list missing entries, received: %v", rl.Tags)
		}
	})
	t.Run("List with artifact filter", func(t *testing.T) {
		r, err := ref.New(repo + ":" + tagName)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := o.ReferrerList(ctx, r, scheme.WithReferrerAT(aType))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 1 {
			t.Errorf("descriptor list length, expected 1, received %d", len(rl.Descriptors))
			return
		}
		rl, err = o.ReferrerList(ctx, r, scheme.WithReferrerAT("application/vnd.example.unknown"))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("unexpected descriptors")
			return
		}
	})
	t.Run("List with annotation filter", func(t *testing.T) {
		r, err := ref.New(repo + ":" + tagName)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := o.ReferrerList(ctx, r, scheme.WithReferrerAnnotations(map[string]string{extraAnnot: extraValueB}))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 1 {
			t.Errorf("descriptor list length, expected 1, received %d", len(rl.Descriptors))
			return
		}
		rl, err = o.ReferrerList(ctx, r, scheme.WithReferrerAnnotations(map[string]string{extraAnnot: "unknown value"}))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("unexpected descriptors")
			return
		}
		rl, err = o.ReferrerList(ctx, r, scheme.WithReferrerAnnotations(map[string]string{extraAnnot: ""}))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 2 {
			t.Errorf("descriptor list length, expected 2, received %d", len(rl.Descriptors))
			return
		}
	})
	// list platform=linux/amd64
	t.Run("List Annotation for Platform", func(t *testing.T) {
		r, err := ref.New(repo + ":" + tagName)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := o.ReferrerList(ctx, r, scheme.WithReferrerPlatform(pAMDStr))
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) != 1 {
			t.Errorf("descriptor list length, expected 1, received %d", len(rl.Descriptors))
			return
		}
	})

	// delete manifests with referrers
	t.Run("Delete", func(t *testing.T) {
		r := mRef
		r.Tag = ""
		r.Digest = artifactAM.GetDescriptor().Digest.String()
		err = o.ManifestDelete(ctx, r, scheme.WithManifest(artifactAM))
		if err != nil {
			t.Errorf("Failed running ManifestDelete on Manifest: %v", err)
			return
		}
		r.Digest = artifactBM.GetDescriptor().Digest.String()
		err = o.ManifestDelete(ctx, r, scheme.WithManifestCheckReferrers())
		if err != nil {
			t.Errorf("Failed running ManifestDelete on Artifact: %v", err)
			return
		}
	})

	// list after delete, verify 0 entries
	t.Run("List empty after delete", func(t *testing.T) {
		r := mRef
		rl, err := o.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) > 0 {
			t.Errorf("descriptors exist")
			return
		}
	})

}

func mapStringStringEq(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}
