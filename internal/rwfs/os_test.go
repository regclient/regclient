package rwfs

import (
	"os"
	"testing"
)

func TestOS(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "os-test-")
	if err != nil {
		t.Fatalf("failed creating tempdir: %v", err)
	}
	t.Logf("tempdir: %s", tempDir)
	defer os.RemoveAll(tempDir)
	fs := OSNew(tempDir)
	if fs == nil {
		t.Errorf("OSNew returned nil")
		return
	}
	testRWFS(t, fs)

	// TODO: attempt to escape tempdir
}
