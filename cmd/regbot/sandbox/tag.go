package sandbox

import (
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

func setupTag(s *Sandbox) {
	s.setupMod(
		luaTagName,
		map[string]lua.LGFunction{
			// "new": s.newTag,
			// "__tostring": s.tagString,
			"delete": s.tagDelete,
			"ls":     s.tagLs,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
		},
	)
}

func (s *Sandbox) tagDelete(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	s.log.WithFields(logrus.Fields{
		"script":  s.name,
		"image":   ref.ref.CommonName(),
		"dry-run": s.dryRun,
	}).Info("Delete tag")
	if s.dryRun {
		return 0
	}
	err := s.rc.TagDelete(s.ctx, ref.ref)
	if err != nil {
		ls.RaiseError("Failed deleting \"%s\": %v", ref.ref.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) tagLs(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"repo":   ref.ref.CommonName(),
	}).Debug("Listing tags")
	tl, err := s.rc.TagList(s.ctx, ref.ref)
	if err != nil {
		ls.RaiseError("Failed retrieving tag list: %v", err)
	}
	lTags := ls.NewTable()
	lTagsList, err := tl.GetTags()
	if err != nil {
		ls.RaiseError("Failed retrieving tag list: %v", err)
	}
	for _, tag := range lTagsList {
		lTags.Append(lua.LString(tag))
	}
	ls.Push(lTags)
	return 1
}
