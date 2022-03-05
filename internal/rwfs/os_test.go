package rwfs

import (
	"testing"
)

func TestOS(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("tempdir: %s", tempDir)
	fs := OSNew(tempDir)
	if fs == nil {
		t.Errorf("OSNew returned nil")
		return
	}
	testRWFS(t, fs)

	// TODO: attempt to escape tempdir
}
