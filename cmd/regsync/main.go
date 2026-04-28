package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/csirmazbendeguz/regclient/internal/godbg"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rootTopCmd, rootOpts := NewRootCmd()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		rootOpts.log.Debug("Interrupt received, stopping")
		// clean shutdown
		cancel()
	}()
	godbg.SignalTrace()

	if err := rootTopCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
