package sandbox

import (
	"github.com/regclient/regclient/regclient/types"
	lua "github.com/yuin/gopher-lua"
)

func setupReference(s *Sandbox) {
	s.setupMod(
		luaReferenceName,
		map[string]lua.LGFunction{
			"new":        s.newReference,
			"__tostring": s.referenceString,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"digest": s.referenceGetSetDigest,
				"tag":    s.referenceGetSetTag,
			},
		},
	)
}

// reference refers to a repository or image name
type reference struct {
	ref types.Ref
}

// newReference creates a reference
func (s *Sandbox) newReference(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	ud := ls.NewUserData()
	ud.Value = &reference{ref: ref.ref}
	ls.SetMetatable(ud, ls.GetTypeMetatable(luaReferenceName))
	ls.Push(ud)
	return 1
}

func (s *Sandbox) checkReference(ls *lua.LState, i int) *reference {
	var ref *reference
	switch ls.Get(i).Type() {
	case lua.LTString:
		nr, err := types.NewRef(ls.CheckString(i))
		if err != nil {
			ls.ArgError(i, "reference parsing failed: "+err.Error())
		}
		ref = &reference{ref: nr}
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *reference:
			ref = ud.Value.(*reference)
		case *sbManifest:
			m := ud.Value.(*sbManifest)
			ref = &reference{ref: m.ref}
		case *config:
			c := ud.Value.(*config)
			ref = &reference{ref: c.ref}
		default:
			ls.ArgError(i, "reference expected")
		}
	default:
		ls.ArgError(i, "reference expected")
	}
	return ref
}

func isReference(ls *lua.LState, i int) bool {
	if ls.Get(i).Type() != lua.LTUserData {
		return false
	}
	ud := ls.CheckUserData(i)
	if _, ok := ud.Value.(*reference); ok {
		return true
	}
	return false
}

// referenceString converts a reference back to a common name
func (s *Sandbox) referenceString(ls *lua.LState) int {
	r := s.checkReference(ls, 1)
	ls.Push(lua.LString(r.ref.CommonName()))
	return 1
}

func (s *Sandbox) referenceGetSetDigest(ls *lua.LState) int {
	r := s.checkReference(ls, 1)
	if ls.GetTop() == 2 {
		r.ref.Digest = ls.CheckString(2)
		return 0
	}
	ls.Push(lua.LString(r.ref.Digest))
	return 1
}

func (s *Sandbox) referenceGetSetTag(ls *lua.LState) int {
	r := s.checkReference(ls, 1)
	if ls.GetTop() == 2 {
		r.ref.Tag = ls.CheckString(2)
		return 0
	}
	ls.Push(lua.LString(r.ref.Tag))
	return 1
}
