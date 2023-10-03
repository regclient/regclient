package rwfs

import "testing"

func TestMem(t *testing.T) {
	t.Parallel()
	fs := MemNew()
	if fs == nil {
		t.Errorf("MemNew returned nil")
		return
	}
	testRWFS(t, fs)
}
