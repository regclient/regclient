package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/regclient/regclient/internal/godbg"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.WithFields(logrus.Fields{}).Debug("Interrupt received, stopping")
		// clean shutdown
		cancel()
	}()
	godbg.SignalTrace()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		// provide tips for common error messages
		switch {
		case strings.Contains(err.Error(), "http: server gave HTTP response to HTTPS client"):
			fmt.Fprintf(os.Stderr, "Try updating your registry with \"regctl registry set --tls disabled <registry>\"\n")
		}
		os.Exit(1)
	}
	os.Exit(0)
}
