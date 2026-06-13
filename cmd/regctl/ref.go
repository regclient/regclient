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
	"fmt"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/ref"
)

type refOpts struct {
	rootOpts *rootOpts
	format   string
}

func NewRefCmd(rOpts *rootOpts) *cobra.Command {
	opts := refOpts{
		rootOpts: rOpts,
	}
	// TODO(bmitch): consider if this should be moved out of hidden/experimental
	cmd := &cobra.Command{
		Hidden: true,
		Use:    "ref",
		Short:  "parse an image ref",
		Long: `Parse an image reference so that it may be output with formatting.
This command is EXPERIMENTAL and could be removed in the future.`,
		Example: `
# extract the registry (docker.io)
regctl ref nginx --format '{{ .Registry }}'
`,
		Args: cobra.ExactArgs(1),
		RunE: opts.runRef,
	}
	cmd.Flags().StringVar(&opts.format, "format", "{{.CommonName}}", "Format the output using a Go template")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)

	return cmd
}

func (opts *refOpts) runRef(cmd *cobra.Command, args []string) error {
	r, err := ref.New(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", args[0], err)
	}

	return template.Writer(cmd.OutOrStdout(), opts.format, r)
}
