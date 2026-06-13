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
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
)

type completeFunc func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

// completeArgList takes a list of completion functions and completes each arg separately
func completeArgList(funcList []completeFunc) completeFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		pos := len(args)
		if pos >= len(funcList) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return funcList[pos](cmd, args, toComplete)
	}
}

func completeArgNone(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeArgDefault(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveDefault
}

func completeArgPlatform(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"local", "linux", "windows", "freebsd",
		"linux/amd64", "linux/386",
		"linux/arm/v5", "linux/arm/v6", "linux/arm/v7", "linux/arm64",
		"linux/mips64le", "linux/ppc64le", "linux/riscv64", "linux/s390x",
		"windows/amd64", "freebsd/amd64",
	}, cobra.ShellCompDirectiveNoFileComp
}

func completeArgMediaTypeManifest(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		mediatype.Docker2Manifest,
		mediatype.Docker2ManifestList,
		mediatype.OCI1Manifest,
		mediatype.OCI1ManifestList,
		mediatype.Docker1Manifest,
		mediatype.Docker1ManifestSigned,
	}, cobra.ShellCompDirectiveNoFileComp
}

func (opts *rootOpts) completeArgTag(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	result := []string{}
	// TODO: is it possible to expand registry, then repo, then tag?
	input := strings.TrimRight(toComplete, ":")
	r, err := ref.New(input)
	if err != nil || r.Digest != "" {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	rc := opts.newRegClient()
	tl, err := rc.TagList(context.Background(), r)
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	tags, err := tl.GetTags()
	if err != nil {
		return result, cobra.ShellCompDirectiveNoFileComp
	}
	for _, tag := range tags {
		resultRef, _ := ref.New(input)
		resultRef = resultRef.SetTag(tag)
		resultCN := resultRef.CommonName()
		if strings.HasPrefix(resultCN, toComplete) {
			result = append(result, resultCN)
		}
	}
	return result, cobra.ShellCompDirectiveNoFileComp
}
