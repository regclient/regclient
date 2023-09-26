package reg

import "github.com/regclient/regclient/scheme"

// Verify Reg implements various interfaces.
var (
	_ scheme.API       = (*Reg)(nil)
	_ scheme.Throttler = (*Reg)(nil)
)

func stringSliceCmp(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
