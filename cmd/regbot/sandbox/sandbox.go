package sandbox

import (
	"context"

	"github.com/regclient/regclient/pkg/go2lua"
	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

const (
	luaReferenceName   = "reference"
	luaTagName         = "tag"
	luaManifestName    = "manifest"
	luaImageName       = "image"
	luaImageConfigName = "imageconfig"
)

// Sandbox defines a lua sandbox
type Sandbox struct {
	L   *lua.LState
	RC  regclient.RegClient
	Ctx context.Context
	log *logrus.Logger
}

// LuaMod defines a mod to add to Lua's sandbox
type LuaMod func(*Sandbox)

var luaMods = []LuaMod{
	setupReference,
	setupTag,
	setupImage,
}

// New creates a new sandbox
func New(ctx context.Context, rc regclient.RegClient, log *logrus.Logger) *Sandbox {
	ls := lua.NewState()
	ls.SetContext(ctx)
	s := &Sandbox{Ctx: ctx, L: ls, RC: rc, log: log}

	for _, mod := range luaMods {
		mod(s)
	}
	return s
}

func (s *Sandbox) setupMod(name string, funcs map[string]lua.LGFunction, tables map[string]map[string]lua.LGFunction) {
	mt := s.L.NewTypeMetatable(name)
	s.L.SetGlobal(name, mt)
	for key, fn := range funcs {
		s.L.SetField(mt, key, s.L.NewFunction(fn))
	}
	for key, fns := range tables {
		s.L.SetField(mt, key, s.L.SetFuncs(s.L.NewTable(), fns))
	}
}

// RunScript is used to execute a script in the sandbox
func (s *Sandbox) RunScript(script string) error {
	return s.L.DoString(script)
}

// Close is use to stop the sandbox
func (s *Sandbox) Close() {
	s.L.Close()
}

// wrapUserData creates a userdata -> wrapped table -> userdata metatable
// structure. This allows references to a struct to resolve for read access,
// while providing access to only the desired methods on the userdata.
func wrapUserData(L *lua.LState, udVal interface{}, wrapVal interface{}, udType string) (lua.LValue, error) {
	ud := L.NewUserData()
	ud.Value = udVal
	wrapTab := go2lua.Convert(L, wrapVal)
	if wrapTab.Type() != lua.LTTable {
		return nil, ErrInvalidWrappedValue
	}
	wrapMTLV := L.GetTypeMetatable(udType)
	wrapMT, ok := wrapMTLV.(*lua.LTable)
	if !ok {
		return nil, ErrInvalidInput
	}
	L.SetMetatable(wrapTab, wrapMT)
	udMT := L.NewTable()
	// TODO: this may only be needed for the "__tostring" method instead of all methods
	wrapMT.ForEach(func(k, v lua.LValue) {
		if k.Type() != lua.LTString || k.String() == "__index" {
			return
		}
		// copy k/v from wrapMT to udMT, handles things like "__tostring"
		udMT.RawSet(k, v)
	})
	udMT.RawSetString("__index", wrapTab)
	L.SetMetatable(ud, udMT)
	return ud, nil
}
