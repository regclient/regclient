package regclient

import (
	"context"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
)

func TestImageCheckBase(t *testing.T) {
	ctx := context.Background()
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	rc := New(WithFS(fsMem))
	r1, err := ref.New("ocidir://testrepo:v1")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}
	r3, err := ref.New("ocidir://testrepo:v3")
	if err != nil {
		t.Errorf("failed to setup ref: %v", err)
		return
	}

	err = rc.ImageCheckBase(ctx, r1, ImageWithCheckBaseRef(r1.CommonName()))
	if err != nil {
		t.Errorf("base image does not match: %v", err)
	}
	err = rc.ImageCheckBase(ctx, r3, ImageWithCheckBaseRef(r1.CommonName()), ImageWithPlatform("linux/amd64"), ImageWithCheckSkipConfig())
	if err != nil {
		t.Errorf("base image does not match: %v", err)
	}
	err = rc.ImageCheckBase(ctx, r3, ImageWithCheckBaseRef(r1.CommonName()), ImageWithPlatform("linux/amd64"))
	if err == nil {
		t.Errorf("base image with different config matched")
	}
	// TODO: add more test cases by adding test data with annotations and proper base images
}
