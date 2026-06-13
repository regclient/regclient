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

package sandbox

import (
	"fmt"
	"log/slog"

	lua "github.com/yuin/gopher-lua"

	"github.com/regclient/regclient/cmd/regbot/internal/go2lua"
	"github.com/regclient/regclient/scheme"
)

func setupRepo(s *Sandbox) {
	s.setupMod(
		luaRepoName,
		map[string]lua.LGFunction{
			"ls": s.repoLs,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
		},
	)
}

type repoLsOpts struct {
	Limit int    `json:"limit"`
	Last  string `json:"last"`
}

func (s *Sandbox) repoLs(ls *lua.LState) int {
	hostLV := ls.Get(1)
	hostLVS, ok := hostLV.(lua.LString)
	if !ok {
		ls.ArgError(1, "Expected registry name (host and optional port)")
	}
	host := hostLVS.String()
	opts := repoLsOpts{}
	optsArgs := []scheme.RepoOpts{}
	if ls.GetTop() > 1 {
		tab := ls.CheckTable(2)
		err := go2lua.Import(ls, tab, &opts, nil)
		if err != nil {
			ls.ArgError(2, fmt.Sprintf("Failed to parse options: %v", err))
		}
		if opts.Limit > 0 {
			optsArgs = append(optsArgs, scheme.WithRepoLimit(opts.Limit))
		}
		if opts.Last != "" {
			optsArgs = append(optsArgs, scheme.WithRepoLast(opts.Last))
		}
	}
	s.log.Debug("Listing repositories",
		slog.String("script", s.name),
		slog.String("host", host),
		slog.Any("opts", opts))
	repoList, err := s.rc.RepoList(s.ctx, host, optsArgs...)
	if err != nil {
		ls.RaiseError("Failed retrieving repo list: %v", err)
	}
	lRepos := ls.NewTable()
	repos, err := repoList.GetRepos()
	if err != nil {
		ls.RaiseError("Failed retrieving repo list: %v", err)
	}
	for _, repo := range repos {
		lRepos.Append(lua.LString(repo))
	}
	ls.Push(lRepos)
	return 1
}
