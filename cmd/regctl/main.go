package main

import (
	"fmt"
	"os"

	"github.com/regclient/regclient/regclient"
)

func main() {
	regclient.UserAgent = "regclient/regctl"
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
