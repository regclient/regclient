package ocidir

import (
	"context"
	"path"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
)

func TestClose(t *testing.T) {
	ctx := context.Background()
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.MkdirAll(fsMem, "testdata/regctl", 0777)
	if err != nil {
		t.Errorf("failed to setup memfs dir: %v", err)
		return
	}
	err = rwfs.CopyRecursive(fsOS, "testdata/regctl", fsMem, "testdata/regctl")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	oMem := New(WithFS(fsMem))
	tRef := "ocidir://testdata/regctl"
	r, err := ref.New(tRef)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", tRef, err)
	}
	rCp := r
	rCp.Tag = ""
	// delete some manifests
	for _, d := range []string{
		"sha256:e57d957b974fb4d852aee59b9b2e9dcd7cb0f04622e9356324864a270afd18a0", // armv6
		"sha256:c5cb89db732b2cd2a21f986cabd094e2fad7870e0176476cf2c566149b50c4e5", // ppc
		"sha256:396f37ccf99628a7a59d5884390a6f07a4aa082595f6ccf0f407ef916a27ef10", // s390x
	} {
		rCp.Digest = d
		err = oMem.ManifestDelete(ctx, rCp)
		if err != nil {
			t.Errorf("failed to delete %s: %v", rCp.CommonName(), err)
		}
	}

	// close to trigger gc
	oMem.Close(ctx, r)

	// check for existence of manifests/blobs
	// note that the test data does not contain all of the blobs listed in the manifests
	for _, d := range []string{
		// common
		"sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14", // common
		"sha256:fa98de7a23a1c3debba4398c982decfd8b31bcfad1ac6e5e7d800375cefbd42f", // common
		"sha256:5ed86438abdd0bd017ae00482e63dc1a85e1edd70789c6eee6462c4ea5cd1ecd", // i386
		"sha256:3615d1937a8fe8708e041e94f4abb544196380e12628bacdcde0a7eaf1a693ba", // amd64
		"sha256:09f080e915a8fdfc7f17fa53922c55eb7aae58614d16c507e65346b124e702af", // armv7
		"sha256:c6ea124eb0b1104042165fdbfed1d6abb00017e116ce2fbc15795e9091fc4672", // arm64
	} {
		dp := digest.Digest(d)
		filename := path.Join("testdata/regctl/blobs", dp.Algorithm().String(), dp.Encoded())
		fh, err := fsMem.Open(filename)
		if err != nil {
			t.Errorf("blob file was gc'd: %s", filename)
		} else {
			fh.Close()
		}
	}

	// check for gc of manifests/blobs
	for _, d := range []string{
		"sha256:7bb8aa6d91c4638208c4f0824b3482dc443f43fb72cad6b077fda0d1fc50f866", // armv6
		"sha256:b400baeb1aac4b98770fea823425e8488ee59fb4af604e565572563248a4028b", // ppc
		"sha256:fff1053dad518155548812182575c07e3d3184533e9c4217bebde4e54c0cc8a8", // s390
	} {
		dp := digest.Digest(d)
		filename := path.Join("testdata/regctl/blobs", dp.Algorithm().String(), dp.Encoded())
		fh, err := fsMem.Open(filename)
		if err == nil {
			t.Errorf("blob file not gc'd: %s", filename)
			fh.Close()
		}
	}

}
