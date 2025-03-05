package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/regclient/regclient"
)

type cobraTestOpts struct {
	stdin  io.Reader
	rcOpts []regclient.Opt
}

func cobraTest(t *testing.T, opts *cobraTestOpts, args ...string) (string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	rootTopCmd, rootOpts := NewRootCmd()
	if opts != nil && opts.rcOpts != nil {
		rootOpts.rcOpts = opts.rcOpts
	}
	if opts != nil && opts.stdin != nil {
		rootTopCmd.SetIn(opts.stdin)
	}
	rootTopCmd.SetOut(buf)
	rootTopCmd.SetErr(buf)
	rootTopCmd.SetArgs(args)

	err := rootTopCmd.Execute()
	return strings.TrimSpace(buf.String()), err
}
