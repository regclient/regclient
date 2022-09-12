//go:build !windows
// +build !windows

package platform

import "runtime"

// Local retrieves the local platform details
func Local() Platform {
	var platform string
	switch runtime.GOOS {
	case "darwin":
		// there are no darwin-docker images as macOS uses Docker for Mac based on linux
		fallthrough
	case "linux":
		platform = "linux"
	default:
		platform = "other"
	}

	return Platform{
		OS:           platform,
		Architecture: runtime.GOARCH,
		Variant:      cpuVariant(),
	}
}
