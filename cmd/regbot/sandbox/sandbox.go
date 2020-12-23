package sandbox

import (
	"context"

	"github.com/regclient/regclient/pkg/go2lua"
	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

const (
	luaRepoName        = "repo"
	luaReferenceName   = "reference"
	luaTagName         = "tag"
	luaManifestName    = "manifest"
	luaImageName       = "image"
	luaImageConfigName = "imageconfig"
)

// Sandbox defines a lua sandbox
type Sandbox struct {
	name string
	ctx  context.Context
	log  *logrus.Logger
	ls   *lua.LState
	rc   regclient.RegClient
}

// LuaMod defines a mod to add to Lua's sandbox
type LuaMod func(*Sandbox)

var luaMods = []LuaMod{
	setupRepo,
	setupReference,
	setupTag,
	setupImage,
}

// New creates a new sandbox
func New(ctx context.Context, name string, rc regclient.RegClient, log *logrus.Logger) *Sandbox {
	ls := lua.NewState()
	// TODO: consider removing default methods from lua
	ls.SetContext(ctx)
	s := &Sandbox{name: name, ctx: ctx, log: log, ls: ls, rc: rc}

	for _, mod := range luaMods {
		mod(s)
	}
	return s
}

func (s *Sandbox) setupMod(name string, funcs map[string]lua.LGFunction, tables map[string]map[string]lua.LGFunction) {
	mt := s.ls.NewTypeMetatable(name)
	s.ls.SetGlobal(name, mt)
	for key, fn := range funcs {
		s.ls.SetField(mt, key, s.ls.NewFunction(fn))
	}
	for key, fns := range tables {
		s.ls.SetField(mt, key, s.ls.SetFuncs(s.ls.NewTable(), fns))
	}
}

// RunScript is used to execute a script in the sandbox
func (s *Sandbox) RunScript(script string) error {
	var err error
	defer func() {
		if r := recover(); r != nil {
			s.log.WithFields(logrus.Fields{
				"name":  s.name,
				"error": r,
			}).Error("Runtime error from script")
		}
		err = ErrScriptFailed
	}()
	err = s.ls.DoString(script)
	return err
}

// Close is use to stop the sandbox
func (s *Sandbox) Close() {
	s.ls.Close()
}

// wrapUserData creates a userdata -> wrapped table -> userdata metatable
// structure. This allows references to a struct to resolve for read access,
// while providing access to only the desired methods on the userdata.
func wrapUserData(ls *lua.LState, udVal interface{}, wrapVal interface{}, udType string) (lua.LValue, error) {
	ud := ls.NewUserData()
	ud.Value = udVal
	udTypeMT, ok := (ls.GetTypeMetatable(udType)).(*lua.LTable)
	if !ok {
		return nil, ErrInvalidInput
	}
	wrapTab := go2lua.Convert(ls, wrapVal)
	if wrapTab.Type() == lua.LTTable {
		wrapMT := ls.NewTable()
		// copy "__tostring" method instead of all methods, overwrite default method on table
		udToString := udTypeMT.RawGetString("__tostring")
		if udToString.Type() == lua.LTFunction {
			wrapMT.RawSetString("__tostring", udToString)
		}
		// alternate method copies most fields from the userdata metatable
		// udTypeMT.ForEach(func(k, v lua.LValue) {
		// 	if k.Type() != lua.LTString || k.String() == "__index" {
		// 		return
		// 	}
		// 	wrapMT.RawSet(k, v)
		// })
		wrapMT.RawSetString("__index", wrapTab)
		ls.SetMetatable(ud, wrapMT)
		ls.SetMetatable(wrapTab, udTypeMT)
	} else {
		ls.SetMetatable(ud, udTypeMT)
	}

	return ud, nil
}
