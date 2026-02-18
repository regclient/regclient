//go:build windows

package godbg

import "os"

func SignalTrace(sigs ...os.Signal) {
	// SignalTrace is not implemented on windows
}
