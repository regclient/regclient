package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/regclient/regclient/internal/godbg"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cmd, opts := NewRootCmd()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		opts.log.Debug("Interrupt received, stopping")
		// clean shutdown
		cancel()
	}()
	godbg.SignalTrace()

	if err := cmd.ExecuteContext(ctx); err != nil {
		if err.Error() != "" {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		}
		// provide tips for common error messages
		switch {
		case strings.Contains(err.Error(), "http: server gave HTTP response to HTTPS client"):
			fmt.Fprintf(os.Stderr, "Try updating your registry with \"regctl registry set --tls disabled <registry>\"\n")
		}
		os.Exit(1)
	}
	os.Exit(0)
}
