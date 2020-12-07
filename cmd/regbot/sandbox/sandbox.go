package sandbox

import (
	"context"

	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

const (
	luaReferenceName = "reference"
	luaTagName       = "tag"
	luaManifestName  = "manifest"
	luaImageName     = "image"
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
