package sandbox

import (
	"fmt"

	"github.com/regclient/regclient/cmd/regbot/internal/go2lua"
	"github.com/regclient/regclient/scheme"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
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
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"host":   host,
		"opts":   opts,
	}).Debug("Listing repositories")
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
