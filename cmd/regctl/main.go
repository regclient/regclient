package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
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
