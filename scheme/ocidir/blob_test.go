package ocidir

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

func TestBlob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := rwfs.OSNew("")
	o := New(WithFS(f))
	// get manifest to lookup config digest
	rs := "ocidir://testdata/regctl:latest"
	rl, err := ref.New(rs)
	if err != nil {
		t.Fatalf("failed to parse ref %s: %v", rs, err)
	}
	ml, err := o.ManifestGet(ctx, rl)
	if err != nil {
		t.Fatalf("manifest get: %v", err)
	}
	if !ml.IsList() {
		t.Fatalf("expected manifest list")
	}
	mli, ok := ml.(manifest.Indexer)
	if !ok {
		t.Fatalf("manifest doesn't support index methods")
	}
	dl, err := mli.GetManifestList()
	if err != nil || len(dl) < 1 {
		t.Fatalf("descriptor list (%d): %v", len(dl), err)
	}
	rs = fmt.Sprintf("%s@%s", rs, dl[0].Digest)
	r, err := ref.New(rs)
	if err != nil {
		t.Fatalf("failed to parse ref %s: %v", rs, err)
	}
	m, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Fatalf("manifest get: %v", err)
	}
	mi, ok := m.(manifest.Imager)
	if !ok {
		t.Fatalf("manifest doesn't support image methods")
	}
	cd, err := mi.GetConfig()
	if err != nil {
		t.Fatalf("config digest: %v", err)
	}
	// blob head
	bh, err := o.BlobHead(ctx, r, cd)
	if err != nil {
		t.Fatalf("blob head: %v", err)
	}
	err = bh.Close()
	if err != nil {
		t.Errorf("blob head close: %v", err)
	}
	// blob get
	bg, err := o.BlobGet(ctx, r, cd)
	if err != nil {
		t.Fatalf("blob get: %v", err)
	}
	bBytes, err := io.ReadAll(bg)
	if err != nil {
		t.Fatalf("blob readall: %v", err)
	}
	if bg.GetDescriptor().Digest != cd.Digest {
		t.Errorf("blob digest mismatch, expected %s, received %s", cd.Digest.String(), bg.GetDescriptor().Digest.String())
	}
	err = bg.Close()
	if err != nil {
		t.Errorf("blob get close: %v", err)
	}
	bFS, err := os.ReadFile(fmt.Sprintf("testdata/regctl/blobs/%s/%s", cd.Digest.Algorithm().String(), cd.Digest.Encoded()))
	if err != nil {
		t.Errorf("blob read file: %v", err)
	}
	if !bytes.Equal(bBytes, bFS) {
		t.Errorf("blob read mismatch, expected %s, received %s", string(bBytes), string(bFS))
	}

	// toOCIConfig
	bg, err = o.BlobGet(ctx, r, cd)
	if err != nil {
		t.Fatalf("blob get 2: %v", err)
	}
	ociConf, err := bg.ToOCIConfig()
	if err != nil {
		t.Fatalf("to oci config: %v", err)
	}
	if ociConf.GetDescriptor().Digest != cd.Digest {
		t.Errorf("config digest mismatch, expected %s, received %s", cd.Digest.String(), ociConf.GetDescriptor().Digest.String())
	}

	// blob put (to memfs)
	fm := rwfs.MemNew()
	om := New(WithFS(fm))
	bRdr := bytes.NewReader(bBytes)
	bpd, err := om.BlobPut(ctx, r, cd, bRdr)
	if err != nil {
		t.Fatalf("blob put: %v", err)
	}
	if bpd.Size != int64(len(bBytes)) {
		t.Errorf("blob put length, expected %d, received %d", len(bBytes), bpd.Size)
	}
	if bpd.Digest != cd.Digest {
		t.Errorf("blob put digest, expected %s, received %s", cd.Digest, bpd.Digest)
	}
	fd, err := fm.Open(fmt.Sprintf("testdata/regctl/blobs/%s/%s", cd.Digest.Algorithm().String(), cd.Digest.Encoded()))
	if err != nil {
		t.Fatalf("blob put open file: %v", err)
	}
	fBytes, err := io.ReadAll(fd)
	_ = fd.Close()
	if err != nil {
		t.Fatalf("blob put readall: %v", err)
	}
	if !bytes.Equal(fBytes, bBytes) {
		t.Errorf("blob put bytes, expected %s, saw %s", string(bBytes), string(fBytes))
	}
	// blob delete (from memfs)
	err = om.BlobDelete(ctx, r, cd)
	if err != nil {
		t.Errorf("failed to delete blob: %v", err)
	}
	_, err = fm.Stat(fmt.Sprintf("testdata/regctl/blobs/%s/%s", cd.Digest.Algorithm().String(), cd.Digest.Encoded()))
	if err == nil {
		t.Errorf("stat of a deleted blob did not fail")
	}
	// concurrent blob put, without the descriptor to test for races
	rPut, err := ref.New(fmt.Sprintf("%s@%s", "ocidir://testdata/put:latest", dl[0].Digest))
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	count := 5
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			bRdr := bytes.NewReader(bBytes)
			bpd, err := om.BlobPut(ctx, rPut, types.Descriptor{}, bRdr)
			if err != nil {
				t.Errorf("blob put: %v", err)
				return
			}
			if bpd.Size != int64(len(bBytes)) {
				t.Errorf("blob put length, expected %d, received %d", len(bBytes), bpd.Size)
			}
			if bpd.Digest != cd.Digest {
				t.Errorf("blob put digest, expected %s, received %s", cd.Digest, bpd.Digest)
			}
		}()
	}
	wg.Wait()
	fd, err = fm.Open(fmt.Sprintf("testdata/put/blobs/%s/%s", cd.Digest.Algorithm().String(), cd.Digest.Encoded()))
	if err != nil {
		t.Fatalf("blob put open file: %v", err)
	}
	fBytes, err = io.ReadAll(fd)
	_ = fd.Close()
	if err != nil {
		t.Fatalf("blob put readall: %v", err)
	}
	if !bytes.Equal(fBytes, bBytes) {
		t.Errorf("blob put bytes, expected %s, saw %s", string(bBytes), string(fBytes))
	}
}
