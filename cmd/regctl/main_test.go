package main

import (
	"bytes"
	"strings"
	"testing"
)

func cobraTest(t *testing.T, args ...string) (string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()
	return strings.TrimSpace(buf.String()), err
}
