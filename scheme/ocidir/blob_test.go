package ocidir

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
)

func TestBlob(t *testing.T) {
	ctx := context.Background()
	f := rwfs.OSNew("")
	o := New(WithFS(f))
	// get manifest to lookup config digest
	rs := "ocidir://testdata/regctl:latest"
	rl, err := ref.New(rs)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", rs, err)
		return
	}
	ml, err := o.ManifestGet(ctx, rl)
	if err != nil {
		t.Errorf("manifest get: %v", err)
		return
	}
	if !ml.IsList() {
		t.Errorf("expected manifest list")
		return
	}
	dl, err := ml.GetDescriptorList()
	if err != nil || len(dl) < 1 {
		t.Errorf("descriptor list (%d): %v", len(dl), err)
		return
	}
	rs = fmt.Sprintf("%s@%s", rs, dl[0].Digest)
	r, err := ref.New(rs)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", rs, err)
		return
	}
	m, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Errorf("manifest get: %v", err)
		return
	}
	cd, err := m.GetConfigDigest()
	if err != nil {
		t.Errorf("config digest: %v", err)
		return
	}
	// blob head
	bh, err := o.BlobHead(ctx, r, cd)
	if err != nil {
		t.Errorf("blob head: %v", err)
		return
	}
	err = bh.Close()
	if err != nil {
		t.Errorf("blob head close: %v", err)
	}
	// blob get
	bg, err := o.BlobGet(ctx, r, cd)
	if err != nil {
		t.Errorf("blob get: %v", err)
		return
	}
	bBytes, err := io.ReadAll(bg)
	if err != nil {
		t.Errorf("blob readall: %v", err)
		return
	}
	if bg.Digest() != cd {
		t.Errorf("blob digest mismatch, expected %s, received %s", cd.String(), bg.Digest().String())
	}
	err = bg.Close()
	if err != nil {
		t.Errorf("blob get close: %v", err)
	}
	bFS, err := os.ReadFile(fmt.Sprintf("testdata/regctl/blobs/%s/%s", cd.Algorithm().String(), cd.Encoded()))
	if bytes.Compare(bBytes, bFS) != 0 {
		t.Errorf("blob read mismatch, expected %s, received %s", string(bBytes), string(bFS))
	}

	// toOCIConfig
	bg, err = o.BlobGet(ctx, r, cd)
	if err != nil {
		t.Errorf("blob get 2: %v", err)
		return
	}
	ociConf, err := bg.ToOCIConfig()
	if err != nil {
		t.Errorf("to oci config: %v", err)
	}
	if ociConf.Digest() != cd {
		t.Errorf("config digest mismatch, expected %s, received %s", cd.String(), ociConf.Digest().String())
	}

	// blob put (to memfs)
	fm := rwfs.MemNew()
	om := New(WithFS(fm))
	bRdr := bytes.NewReader(bBytes)
	bpd, bpl, err := om.BlobPut(ctx, r, cd, bRdr, int64(len(bBytes)))
	if err != nil {
		t.Errorf("blob put: %v", err)
		return
	}
	if bpl != int64(len(bBytes)) {
		t.Errorf("blob put length, expected %d, received %d", len(bBytes), bpl)
	}
	if bpd != cd {
		t.Errorf("blob put digest, expected %s, received %s", cd, bpd)
	}
	fd, err := fm.Open(fmt.Sprintf("testdata/regctl/blobs/%s/%s", cd.Algorithm().String(), cd.Encoded()))
	if err != nil {
		t.Errorf("blob put open file: %v", err)
	}
	defer fd.Close()
	fBytes, err := io.ReadAll(fd)
	if err != nil {
		t.Errorf("blob put readall: %v", err)
	}
	if bytes.Compare(fBytes, bBytes) != 0 {
		t.Errorf("blob put bytes, expected %s, saw %s", string(bBytes), string(fBytes))
	}
}
