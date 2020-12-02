package main

import (
	"fmt"
	"os"
)

func main() {
	// regclient.ConfigDir = ".regsync"
	// regclient.ConfigEnv = "REGSYNC_CONFIG"

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}
