package ocidir

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

func TestClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tempDir := t.TempDir()
	err := copyfs.Copy(filepath.Join(tempDir, "testrepo"), "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to setup tempDir: %v", err)
	}
	o := New()
	rStr := "ocidir://" + tempDir + "/testrepo:v3"
	r, err := ref.New(rStr)
	if err != nil {
		t.Fatalf("failed to parse ref %s: %v", rStr, err)
	}
	// delete every other entry in the manifest list, tracking the config descriptor of each
	delDesc := []descriptor.Descriptor{}
	keepDesc := []descriptor.Descriptor{}
	m, err := o.ManifestGet(ctx, r)
	if err != nil {
		t.Fatalf("failed to get manifest: %v", err)
	}
	if !m.IsList() {
		t.Fatalf("manifest is not an index: %s", rStr)
	}
	mInd := m.(manifest.Indexer)
	ml, err := mInd.GetManifestList()
	if err != nil {
		t.Fatalf("failed to get manifest list: %v", err)
	}
	for i, d := range ml {
		rImg := r.SetDigest(d.Digest.String())
		m, err := o.ManifestGet(ctx, rImg)
		if err != nil {
			t.Fatalf("failed to get index entry %d from %s: %v", i, rStr, err)
		}
		if m.IsList() {
			continue
		}
		mImg := m.(manifest.Imager)
		cd, err := mImg.GetConfig()
		if err != nil {
			t.Fatalf("failed to get config descriptor for %s: %v", rImg.CommonName(), err)
		}
		if i%2 == 0 {
			delDesc = append(delDesc, cd)
			err = o.ManifestDelete(ctx, rImg)
			if err != nil {
				t.Fatalf("failed to delete %s: %v", rImg.CommonName(), err)
			}
		} else {
			keepDesc = append(keepDesc, cd)
		}
	}

	// close to trigger gc
	o.Close(ctx, r)

	// check for existence of blobs
	for _, d := range keepDesc {
		file := filepath.Join(tempDir, "testrepo/blobs", d.Digest.Algorithm().String(), d.Digest.Encoded())
		_, err = os.Stat(file)
		if err != nil {
			t.Errorf("failed to stat file being preserved: %s: %v", file, err)
		}
	}
	for _, d := range delDesc {
		file := filepath.Join(tempDir, "testrepo/blobs", d.Digest.Algorithm().String(), d.Digest.Encoded())
		_, err = os.Stat(file)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("file was not deleted by GC: %s: %v", file, err)
		}
	}
}
