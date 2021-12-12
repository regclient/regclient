//go:build windows
// +build windows

package main

import "os"

func getFileOwner(stat os.FileInfo) (int, int, error) {
	return 0, 0, nil
}
