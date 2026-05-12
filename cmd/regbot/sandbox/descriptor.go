package sandbox

import (
	"encoding/json"

	lua "github.com/yuin/gopher-lua"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/ref"
)

func setupDescriptor(s *Sandbox) {
	s.setupMod(
		luaDescriptorName,
		map[string]lua.LGFunction{
			"__tostring": s.descriptorJSON,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
		},
	)
}

type sbDescriptor struct {
	d descriptor.Descriptor
	r ref.Ref
}

func (s *Sandbox) checkDescriptor(ls *lua.LState, i int) *sbDescriptor {
	fetchDescriptor := func(r ref.Ref) *sbDescriptor {
		rcM, err := s.rc.ManifestHead(s.ctx, r, regclient.WithManifestRequireDigest())
		if err != nil {
			ls.RaiseError("Failed retrieving \"%s\" manifest: %v", r.CommonName(), err)
		}
		return &sbDescriptor{d: rcM.GetDescriptor(), r: r}
	}

	var d *sbDescriptor
	switch ls.Get(i).Type() {
	case lua.LTString:
		r, err := ref.New(ls.CheckString(1))
		if err != nil {
			ls.RaiseError("reference parsing failed: %v", err)
		}
		d = fetchDescriptor(r)
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *sbDescriptor:
			d = ud.Value.(*sbDescriptor)
		case *sbManifest:
			m := ud.Value.(*sbManifest)
			d = &sbDescriptor{d: m.m.GetDescriptor(), r: m.r}
		case *config:
			c := ud.Value.(*config)
			m := &sbManifest{r: c.r, m: c.m}
			d = &sbDescriptor{d: m.m.GetDescriptor(), r: m.r}
		case *reference:
			r := ud.Value.(*reference)
			d = fetchDescriptor(r.r)
		default:
			ls.ArgError(i, "descriptor expected")
		}
	default:
		ls.ArgError(i, "descriptor expected")
	}
	return d
}

func (s *Sandbox) descriptorJSON(ls *lua.LState) int {
	d := s.checkDescriptor(ls, 1)

	mJSON, err := json.MarshalIndent(d.d, "", "  ")
	if err != nil {
		ls.RaiseError("Failed outputing descriptor: %v", err)
	}
	ls.Push(lua.LString(string(mJSON)))
	return 1
}
