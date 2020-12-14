package sandbox

import (
	"github.com/regclient/regclient/regclient"
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
				"tag": s.referenceGetSetTag,
			},
		},
	)
}

// reference refers to a repository or image name
type reference struct {
	Ref regclient.Ref
}

// newReference creates a reference
func (s *Sandbox) newReference(L *lua.LState) int {
	ref, err := regclient.NewRef(L.CheckString(1))
	if err != nil {
		L.ArgError(1, "reference parsing failed: "+err.Error())
	}
	reference := &reference{Ref: ref}
	ud := L.NewUserData()
	ud.Value = reference
	L.SetMetatable(ud, L.GetTypeMetatable(luaReferenceName))
	L.Push(ud)
	return 1
}

// referenceString converts a reference back to a common name
func (s *Sandbox) referenceString(L *lua.LState) int {
	r := checkReference(L, 1)
	L.Push(lua.LString(r.Ref.CommonName()))
	return 1
}

func (s *Sandbox) referenceGetSetTag(L *lua.LState) int {
	r := checkReference(L, 1)
	if L.GetTop() == 2 {
		r.Ref.Tag = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(r.Ref.Tag))
	return 1
}

func checkReference(L *lua.LState, i int) *reference {
	var ref *reference
	switch L.Get(i).Type() {
	case lua.LTString:
		nr, err := regclient.NewRef(L.CheckString(1))
		if err != nil {
			L.ArgError(i, "reference parsing failed: "+err.Error())
		}
		ref = &reference{Ref: nr}
	case lua.LTUserData:
		ud := L.CheckUserData(i)
		r, ok := ud.Value.(*reference)
		if !ok {
			L.ArgError(i, "reference expected")
		}
		ref = r
	default:
		L.ArgError(i, "reference expected")
	}
	return ref
}

func isReference(L *lua.LState, i int) bool {
	if L.Get(i).Type() != lua.LTUserData {
		return false
	}
	ud := L.CheckUserData(i)
	if _, ok := ud.Value.(*reference); ok {
		return true
	}
	return false
}
