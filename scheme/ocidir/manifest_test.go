package ocidir

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tempDir := t.TempDir()
	err := copyfs.Copy(filepath.Join(tempDir, "testrepo"), "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to setup tempDir: %v", err)
	}
	o := New()
	rStr := "ocidir://" + tempDir + "/testrepo:v1"
	r, err := ref.New(rStr)
	if err != nil {
		t.Fatalf("failed to parse ref %s: %v", rStr, err)
	}
	// manifest head
	_, err = o.ManifestHead(ctx, r)
	if err != nil {
		t.Errorf("manifest head: %v", err)
	}
	// manifest list
	ml, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Fatalf("manifest get: %v", err)
	}
	if manifest.GetMediaType(ml) != mediatype.OCI1ManifestList {
		t.Errorf("manifest mt, expected %s, received %s", mediatype.OCI1ManifestList, manifest.GetMediaType(ml))
	}
	if !ml.IsList() {
		t.Errorf("expected manifest list")
	}
	mlb, err := ml.RawBody()
	if err != nil {
		t.Fatalf("failed to get body of manifest: %v", err)
	}
	mli, ok := ml.(manifest.Indexer)
	if !ok {
		t.Fatalf("manifest doesn't support index methods")
	}
	dl, err := mli.GetManifestList()
	if err != nil || len(dl) < 1 {
		t.Fatalf("descriptor list (%d): %v", len(dl), err)
	}
	// manifest head on a child digest
	r = r.SetDigest(dl[0].Digest.String())
	_, err = o.ManifestHead(ctx, r)
	if err != nil {
		t.Errorf("manifest head failed on child digest: %v", err)
	}
	digMissing := digest.Canonical.FromString("missing")
	digSHA512 := digest.SHA512.FromBytes(mlb)
	rMissing := r.SetDigest(digMissing.String())
	rSHA512 := r.SetDigest(digSHA512.String())
	_, err = o.ManifestHead(ctx, rMissing)
	if err == nil {
		t.Errorf("manifest head succeeded on missing digest: %s", rMissing.CommonName())
	}
	_, err = o.ManifestHead(ctx, rSHA512)
	if err == nil {
		t.Errorf("manifest head succeeded on alternate algorithm: %s", rSHA512.CommonName())
	}
	// image manifest
	m, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Fatalf("manifest get: %v", err)
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		t.Fatalf("manifest doesn't support image methods")
	}
	_, err = mi.GetConfig()
	if err != nil {
		t.Errorf("config: %v", err)
	}

	// test manifest put to a memfs
	rStr = "ocidir://" + tempDir + "/put:v1"
	putPath := filepath.Join(tempDir, "put")
	rPut, err := ref.New(rStr)
	if err != nil {
		t.Fatalf("failed to parse ref %s: %v", rStr, err)
	}
	err = o.ManifestPut(ctx, rPut, m)
	if err != nil {
		t.Errorf("manifest put: %v", err)
	}
	fh, err := os.Open(filepath.Join(putPath, imageLayoutFile))
	if err != nil {
		t.Fatalf("open oci-layout: %v", err)
	}
	lb, err := io.ReadAll(fh)
	if err != nil {
		t.Fatalf("readall oci-layout: %v", err)
	}
	l := v1.ImageLayout{}
	err = json.Unmarshal(lb, &l)
	if err != nil {
		t.Fatalf("json unmarshal oci-layout: %v", err)
	}
	if l.Version != "1.0.0" {
		t.Errorf("oci-layout version, expected 1.0.0, received %s", l.Version)
	}
	d := m.GetDescriptor().Digest
	fh, err = os.Open(path.Join(putPath, "blobs", d.Algorithm().String(), d.Encoded()))
	if err != nil {
		t.Errorf("failed to open manifest blob: %v", err)
	}
	bRaw, err := io.ReadAll(fh)
	if err != nil {
		t.Fatalf("failed to read manifest blob: %v", err)
	}
	mRaw, err := m.RawBody()
	if err != nil {
		t.Fatalf("failed to run RawBody: %v", err)
	}
	if !bytes.Equal(bRaw, mRaw) {
		t.Errorf("blob and raw do not match, raw %s, blob %s", string(mRaw), string(bRaw))
	}
	tl, err := o.TagList(ctx, rPut)
	if err != nil {
		t.Fatalf("tag list: %v", err)
	}
	tlt, err := tl.GetTags()
	if err != nil {
		t.Fatalf("tag list tags: %v", err)
	}
	if len(tlt) != 1 || tlt[0] != "v1" {
		t.Errorf("tag list, expected v1, received %v", tlt)
	}
	// test manifest delete
	err = o.ManifestDelete(ctx, rPut.SetDigest(d.String()))
	if err != nil {
		t.Errorf("failed to delete tag: %v", err)
	}
	tl, err = o.TagList(ctx, rPut)
	if err != nil {
		t.Fatalf("tag list: %v", err)
	}
	tlt, err = tl.GetTags()
	if err != nil {
		t.Fatalf("tag list tags: %v", err)
	}
	if len(tlt) != 0 {
		t.Errorf("tag list, expected empty list, received %v", tlt)
	}
	err = o.ManifestDelete(ctx, rPut)
	if err == nil {
		t.Errorf("deleted tag twice")
	}

	// test tag delete on v1 and v2
	r = r.SetTag("v1")
	mh1, err := o.ManifestHead(ctx, r)
	if err != nil {
		t.Errorf("failed getting %s manifest head: %v", r.Tag, err)
	}
	err = o.TagDelete(ctx, r)
	if err != nil {
		t.Errorf("failed deleting tag %s: %v", r.Tag, err)
	}
	err = o.Close(ctx, r)
	if err != nil {
		t.Errorf("failed closing: %v", err)
	}
	// verify digest for v1 has been deleted
	mh1D := mh1.GetDescriptor().Digest
	fh, err = os.Open(path.Join(tempDir, "testrepo/blobs", mh1D.Algorithm().String(), mh1D.Encoded()))
	if err == nil {
		t.Errorf("manifest blob exists for %s: %v", r.Tag, err)
		fh.Close()
	}
	// verify v1 tag removed
	o = New()
	_, err = o.ManifestHead(ctx, r)
	if err == nil {
		t.Errorf("succeeded getting deleted tag %s: %v", r.Tag, err)
	}

	// push a dup tag
	r11 := r.SetTag("v1.1")
	err = o.ManifestPut(ctx, r11, ml)
	if err != nil {
		t.Errorf("failed pushing manifest: %v", err)
	}
	// push by digest
	rd := r.SetDigest(ml.GetDescriptor().Digest.String())
	err = o.ManifestPut(ctx, rd, ml)
	if err != nil {
		t.Errorf("failed pushing manifest: %v", err)
	}
	// push second tag
	r12 := r.SetTag("v1.2")
	err = o.ManifestPut(ctx, r12, ml)
	if err != nil {
		t.Errorf("failed pushing manifest: %v", err)
	}
	// push invalid digest
	err = o.manifestPut(ctx, rMissing, ml)
	if err == nil {
		t.Errorf("succeeded pushing with invalid digest")
	}
	// push with alternate digest
	err = o.manifestPut(ctx, rSHA512, ml)
	if err != nil {
		t.Errorf("failed pushing with alternate digest algorithm: %v", err)
	}
	// close and reopen
	err = o.Close(ctx, r)
	if err != nil {
		t.Errorf("failed closing: %v", err)
	}
	o = New()
	// verify original tag has not been deleted
	_, err = o.ManifestHead(ctx, r11)
	if err != nil {
		t.Errorf("could not query manifest after pushing dup tag")
	}
}
