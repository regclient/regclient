package rwfs

import "testing"

func TestMem(t *testing.T) {
	t.Parallel()
	fs := MemNew()
	if fs == nil {
		t.Fatalf("MemNew returned nil")
	}
	testRWFS(t, fs)
}
