package sandbox

import (
	"fmt"
	"log/slog"

	lua "github.com/yuin/gopher-lua"

	"github.com/regclient/regclient/cmd/regbot/internal/go2lua"
	"github.com/regclient/regclient/scheme"
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

type tagLsOpts struct {
	Limit int    `json:"limit"`
	Last  string `json:"last"`
}

func (s *Sandbox) tagLs(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	opts := tagLsOpts{}
	optsArgs := []scheme.TagOpts{}
	if ls.GetTop() > 1 {
		tab := ls.CheckTable(2)
		err := go2lua.Import(ls, tab, &opts, nil)
		if err != nil {
			ls.ArgError(2, fmt.Sprintf("Failed to parse options: %v", err))
		}
		if opts.Limit > 0 {
			optsArgs = append(optsArgs, scheme.WithTagLimit(opts.Limit))
		}
		if opts.Last != "" {
			optsArgs = append(optsArgs, scheme.WithTagLast(opts.Last))
		}
	}
	s.log.Debug("Listing tags",
		slog.String("script", s.name),
		slog.String("repo", r.r.CommonName()),
		slog.Any("opts", opts))
	tl, err := s.rc.TagList(s.ctx, r.r, optsArgs...)
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
