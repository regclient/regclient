package version

import (
	"encoding/json"
	"testing"
)

func TestVersion(t *testing.T) {
	i := GetInfo()
	ij, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		t.Errorf("failed to marshal info: %v", err)
		return
	}
	t.Logf("received info:\n%s", string(ij))
}
