package version

import (
	"encoding/json"
	"testing"
)

func TestVersion(t *testing.T) {
	t.Parallel()
	i := GetInfo()
	ij, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal info: %v", err)
	}
	t.Logf("received info:\n%s", string(ij))
}
