package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type cobraTestOpts struct {
	stdin io.Reader
}

func cobraTest(t *testing.T, opts *cobraTestOpts, args ...string) (string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	rootTopCmd := NewRootCmd()
	if opts != nil && opts.stdin != nil {
		rootTopCmd.SetIn(opts.stdin)
	}
	rootTopCmd.SetOut(buf)
	rootTopCmd.SetErr(buf)
	rootTopCmd.SetArgs(args)

	err := rootTopCmd.Execute()
	return strings.TrimSpace(buf.String()), err
}
