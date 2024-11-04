package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/internal/godbg"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	log := &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.InfoLevel,
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.WithFields(logrus.Fields{}).Debug("Interrupt received, stopping")
		// clean shutdown
		cancel()
	}()
	godbg.SignalTrace()

	rootTopCmd := NewRootCmd(log)
	if err := rootTopCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
