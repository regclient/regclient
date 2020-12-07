package sandbox

import (
	lua "github.com/yuin/gopher-lua"
)

func setupTag(s *Sandbox) {
	s.setupMod(
		luaTagName,
		map[string]lua.LGFunction{
			// "new": s.newTag,
			// "__tostring": s.referenceString,
			"ls": s.tagLs,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
		},
	)
}

func (s *Sandbox) tagLs(L *lua.LState) int {
	ref := checkReference(L, 1)
	tl, err := s.RC.TagList(s.Ctx, ref.Ref)
	if err != nil {
		L.RaiseError("Failed retrieving tag list: %v", err)
	}
	lTags := L.NewTable()
	for _, tag := range tl.Tags {
		lTags.Append(lua.LString(tag))
	}
	L.Push(lTags)
	return 1
}
