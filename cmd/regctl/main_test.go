// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
