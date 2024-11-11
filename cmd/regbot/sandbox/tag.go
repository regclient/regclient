package sandbox

import (
	"log/slog"

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
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	s.log.Info("Delete tag",
		slog.String("script", s.name),
		slog.String("image", r.r.CommonName()),
		slog.Bool("dry-run", s.dryRun))
	if s.dryRun {
		return 0
	}
	err = s.rc.TagDelete(s.ctx, r.r)
	if err != nil {
		ls.RaiseError("Failed deleting \"%s\": %v", r.r.CommonName(), err)
	}
	err = s.rc.Close(s.ctx, r.r)
	if err != nil {
		ls.RaiseError("Failed closing reference \"%s\": %v", r.r.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) tagLs(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	s.log.Debug("Listing tags",
		slog.String("script", s.name),
		slog.String("repo", r.r.CommonName()))
	tl, err := s.rc.TagList(s.ctx, r.r)
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
