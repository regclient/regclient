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
	f := rwfs.OSNew("")
	o := New(WithFS(f))
	rs := "ocidir://testdata/regctl:latest"
	r, err := ref.New(rs)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", rs, err)
	}
	// manifest head
	_, err = o.ManifestHead(ctx, r)
	if err != nil {
		t.Errorf("manifest head: %v", err)
		return
	}
	// manifest list
	ml, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Errorf("manifest get: %v", err)
	}
	if manifest.GetMediaType(ml) != types.MediaTypeDocker2ManifestList {
		t.Errorf("manifest mt, expected %s, received %s", types.MediaTypeDocker2ManifestList, manifest.GetMediaType(ml))
	}
	if !ml.IsList() {
		t.Errorf("expected manifest list")
		return
	}
	dl, err := ml.GetManifestList()
	if err != nil || len(dl) < 1 {
		t.Errorf("descriptor list (%d): %v", len(dl), err)
		return
	}
	// image manifest
	rs = fmt.Sprintf("%s@%s", rs, dl[0].Digest)
	r, err = ref.New(rs)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", rs, err)
		return
	}
	m, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Errorf("manifest get: %v", err)
		return
	}
	_, err = m.GetConfig()
	if err != nil {
		t.Errorf("config: %v", err)
		return
	}
	// test manifest put to a memfs
	fm := rwfs.MemNew()
	om := New(WithFS(fm))
	err = om.ManifestPut(ctx, r, m)
	if err != nil {
		t.Errorf("manifest put: %v", err)
	}
	fh, err := fm.Open("testdata/regctl/" + imageLayoutFile)
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
	if len(tlt) != 1 || tlt[0] != "latest" {
		t.Errorf("tag list, expected latest, received %v", tlt)
	}
	// test manifest delete
	t.Logf("deleting %s", r.CommonName())
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

}
