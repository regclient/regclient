package sandbox

import (
	lua "github.com/yuin/gopher-lua"

	"github.com/regclient/regclient/types/ref"
)

func setupReference(s *Sandbox) {
	s.setupMod(
		luaReferenceName,
		map[string]lua.LGFunction{
			"new":        s.newReference,
			"close":      s.closeReference,
			"__tostring": s.referenceString,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"close":  s.closeReference,
				"digest": s.referenceGetSetDigest,
				"tag":    s.referenceGetSetTag,
			},
		},
	)
}

// reference refers to a repository or image name
type reference struct {
	r ref.Ref
}

// newReference creates a reference
func (s *Sandbox) newReference(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	ud := ls.NewUserData()
	ud.Value = &reference{r: r.r}
	ls.SetMetatable(ud, ls.GetTypeMetatable(luaReferenceName))
	ls.Push(ud)
	return 1
}

func (s *Sandbox) closeReference(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	err = s.rc.Close(s.ctx, r.r)
	if err != nil {
		ls.ArgError(1, "reference close failed: "+err.Error())
	}
	return 0
}

func (s *Sandbox) checkReference(ls *lua.LState, i int) *reference {
	var r *reference
	switch ls.Get(i).Type() {
	case lua.LTString:
		nr, err := ref.New(ls.CheckString(i))
		if err != nil {
			ls.ArgError(i, "reference parsing failed: "+err.Error())
		}
		r = &reference{r: nr}
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *reference:
			r = ud.Value.(*reference)
		case *sbManifest:
			m := ud.Value.(*sbManifest)
			r = &reference{r: m.r}
		case *config:
			c := ud.Value.(*config)
			r = &reference{r: c.r}
		default:
			ls.ArgError(i, "reference expected")
		}
	default:
		ls.ArgError(i, "reference expected")
	}
	return r
}

// func isReference(ls *lua.LState, i int) bool {
// 	if ls.Get(i).Type() != lua.LTUserData {
// 		return false
// 	}
// 	ud := ls.CheckUserData(i)
// 	if _, ok := ud.Value.(*reference); ok {
// 		return true
// 	}
// 	return false
// }

// referenceString converts a reference back to a common name
func (s *Sandbox) referenceString(ls *lua.LState) int {
	r := s.checkReference(ls, 1)
	ls.Push(lua.LString(r.r.CommonName()))
	return 1
}

func (s *Sandbox) referenceGetSetDigest(ls *lua.LState) int {
	r := s.checkReference(ls, 1)
	if ls.GetTop() == 2 {
		r.r.Digest = ls.CheckString(2)
		return 0
	}
	ls.Push(lua.LString(r.r.Digest))
	return 1
}

func (s *Sandbox) referenceGetSetTag(ls *lua.LState) int {
	r := s.checkReference(ls, 1)
	if ls.GetTop() == 2 {
		r.r.Tag = ls.CheckString(2)
		return 0
	}
	ls.Push(lua.LString(r.r.Tag))
	return 1
}
