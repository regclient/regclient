//go:build !windows

// Package godbg provides tooling for debugging Go
package godbg

import (
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

func SignalTrace(sigs ...os.Signal) {
	if len(sigs) == 0 {
		sigs = append(sigs, syscall.SIGUSR1)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, sigs...)
	go func() {
		<-sig
		_ = pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
	}()
}
