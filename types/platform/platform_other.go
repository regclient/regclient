//go:build !windows
// +build !windows

package platform

import "runtime"

// Local retrieves the local platform details
func Local() Platform {
	plat := Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Variant:      cpuVariant(),
	}
	switch plat.OS {
	case "macos":
		plat.OS = "darwin"
	}
	return plat
}
