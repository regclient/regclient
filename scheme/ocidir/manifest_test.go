package ocidir

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

func TestManifest(t *testing.T) {
	ctx := context.Background()
	// copy testdata images into memory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	o := New(WithFS(fsMem))
	rs := "ocidir://testrepo:v1"
	r, err := ref.New(rs)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", rs, err)
		return
	}
	// manifest head
	_, err = o.ManifestHead(ctx, r)
	if err != nil {
		t.Errorf("manifest head: %v", err)
	}
	// manifest list
	ml, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Errorf("manifest get: %v", err)
	}
	if manifest.GetMediaType(ml) != types.MediaTypeOCI1ManifestList {
		t.Errorf("manifest mt, expected %s, received %s", types.MediaTypeOCI1ManifestList, manifest.GetMediaType(ml))
	}
	if !ml.IsList() {
		t.Errorf("expected manifest list")
	}
	mli, ok := ml.(manifest.Indexer)
	if !ok {
		t.Errorf("manifest doesn't support index methods")
		return
	}
	dl, err := mli.GetManifestList()
	if err != nil || len(dl) < 1 {
		t.Errorf("descriptor list (%d): %v", len(dl), err)
	}
	// manifest head on a child digest
	rs = fmt.Sprintf("%s@%s", rs, dl[0].Digest)
	r, err = ref.New(rs)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", rs, err)
		return
	}
	_, err = o.ManifestHead(ctx, r)
	if err != nil {
		t.Errorf("manifest head failed on child digest: %v", err)
	}
	rMissing := r
	rMissing.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err = o.ManifestHead(ctx, rMissing)
	if err == nil {
		t.Errorf("manifest head succeeded on missing digest: %s", rMissing.CommonName())
	}
	// image manifest
	m, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Errorf("manifest get: %v", err)
		return
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		t.Errorf("manifest doesn't support image methods")
		return
	}
	_, err = mi.GetConfig()
	if err != nil {
		t.Errorf("config: %v", err)
	}

	// test manifest put to a memfs
	fm := rwfs.MemNew()
	om := New(WithFS(fm))
	err = om.ManifestPut(ctx, r, m)
	if err != nil {
		t.Errorf("manifest put: %v", err)
	}
	fh, err := fm.Open("testrepo/" + imageLayoutFile)
	if err != nil {
		t.Errorf("open oci-layout: %v", err)
		return
	}
	lb, err := io.ReadAll(fh)
	if err != nil {
		t.Errorf("readall oci-layout: %v", err)
	}
	l := v1.ImageLayout{}
	err = json.Unmarshal(lb, &l)
	if err != nil {
		t.Errorf("json unmarshal oci-layout: %v", err)
	}
	if l.Version != "1.0.0" {
		t.Errorf("oci-layout version, expected 1.0.0, received %s", l.Version)
	}
	d := digest.Digest(r.Digest)
	fh, err = fm.Open(path.Join(r.Path, "blobs", d.Algorithm().String(), d.Encoded()))
	if err != nil {
		t.Errorf("failed to open manifest blob: %v", err)
	}
	bRaw, err := io.ReadAll(fh)
	if err != nil {
		t.Errorf("failed to read manifest blob: %v", err)
	}
	mRaw, err := m.RawBody()
	if err != nil {
		t.Errorf("failed to run RawBody: %v", err)
	}
	if !bytes.Equal(bRaw, mRaw) {
		t.Errorf("blob and raw do not match, raw %s, blob %s", string(mRaw), string(bRaw))
	}
	tl, err := om.TagList(ctx, r)
	if err != nil {
		t.Errorf("tag list: %v", err)
	}
	tlt, err := tl.GetTags()
	if err != nil {
		t.Errorf("tag list tags: %v", err)
	}
	if len(tlt) != 1 || tlt[0] != "v1" {
		t.Errorf("tag list, expected v1, received %v", tlt)
	}
	// test manifest delete
	err = om.ManifestDelete(ctx, r)
	if err != nil {
		t.Errorf("failed to delete tag: %v", err)
	}
	tl, err = om.TagList(ctx, r)
	if err != nil {
		t.Errorf("tag list: %v", err)
	}
	tlt, err = tl.GetTags()
	if err != nil {
		t.Errorf("tag list tags: %v", err)
	}
	if len(tlt) != 0 {
		t.Errorf("tag list, expected empty list, received %v", tlt)
	}
	err = om.ManifestDelete(ctx, r)
	if err == nil {
		t.Errorf("deleted tag twice")
	}

	// test tag delete on v1 and v2
	r.Digest = ""
	r.Tag = "v1"
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
	// verify digest for v1 still exists but v2 has been deleted
	mh1D := mh1.GetDescriptor().Digest
	fh, err = fsMem.Open(path.Join(r.Path, "blobs", mh1D.Algorithm().String(), mh1D.Encoded()))
	if err == nil {
		t.Errorf("manifest blob exists for %s: %v", r.Tag, err)
		fh.Close()
	}
	// verify v1 tag removed
	o = New(WithFS(fsMem))
	_, err = o.ManifestHead(ctx, r)
	if err == nil {
		t.Errorf("succeeded getting deleted tag %s: %v", r.Tag, err)
	}

	// push a dup tag
	r11 := r
	r11.Tag = "v1.1"
	err = o.ManifestPut(ctx, r11, ml)
	if err != nil {
		t.Errorf("failed pushing manifest: %v", err)
	}
	// push by digest
	rd := r
	rd.Tag = ""
	rd.Digest = ml.GetDescriptor().Digest.String()
	err = o.ManifestPut(ctx, rd, ml)
	if err != nil {
		t.Errorf("failed pushing manifest: %v", err)
	}
	// push second tag
	r12 := r
	r12.Tag = "v1.2"
	err = o.ManifestPut(ctx, r12, ml)
	if err != nil {
		t.Errorf("failed pushing manifest: %v", err)
	}
	err = o.Close(ctx, r)
	if err != nil {
		t.Errorf("failed closing: %v", err)
	}
	o = New(WithFS(fsMem))
	// verify original tag has not been deleted
	_, err = o.ManifestHead(ctx, r11)
	if err != nil {
		t.Errorf("could not query manifest after pushing dup tag")
	}

}
