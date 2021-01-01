package sandbox

import (
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

func (s *Sandbox) repoLs(ls *lua.LState) int {
	hostLV := ls.Get(1)
	hostLVS, ok := hostLV.(lua.LString)
	if !ok {
		ls.ArgError(1, "Expected registry name (host and optional port)")
	}
	host := hostLVS.String()
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"host":   host,
	}).Debug("Listing repositories")
	repos, err := s.rc.RepoList(s.ctx, host)
	if err != nil {
		ls.RaiseError("Failed retrieving repo list: %v", err)
	}
	lRepos := ls.NewTable()
	for _, repo := range repos.Repositories {
		lRepos.Append(lua.LString(repo))
	}
	ls.Push(lRepos)
	return 1
}
