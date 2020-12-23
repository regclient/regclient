package sandbox

import (
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
	err := s.rc.TagDelete(s.ctx, ref.Ref)
	if err != nil {
		ls.RaiseError("Failed deleting \"%s\": %v", ref.Ref.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) tagLs(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	tl, err := s.rc.TagList(s.ctx, ref.Ref)
	if err != nil {
		ls.RaiseError("Failed retrieving tag list: %v", err)
	}
	lTags := ls.NewTable()
	for _, tag := range tl.Tags {
		lTags.Append(lua.LString(tag))
	}
	ls.Push(lTags)
	return 1
}
