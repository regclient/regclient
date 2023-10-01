// Package sandbox defines the Lua sandbox used to run user scripts
package sandbox

import (
	"context"

	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/cmd/regbot/internal/go2lua"
	"github.com/regclient/regclient/internal/throttle"
)

const (
	luaRepoName        = "repo"
	luaReferenceName   = "reference"
	luaTagName         = "tag"
	luaManifestName    = "manifest"
	luaImageName       = "image"
	luaImageConfigName = "imageconfig"
	luaBlobName        = "blob"
)

// Sandbox defines a lua sandbox
type Sandbox struct {
	name      string
	ctx       context.Context
	log       *logrus.Logger
	ls        *lua.LState
	rc        *regclient.RegClient
	throttleC *throttle.Throttle
	dryRun    bool
}

// LuaMod defines a mod to add to Lua's sandbox
type LuaMod func(*Sandbox)

var luaMods = []LuaMod{
	setupRepo,
	setupReference,
	setupTag,
	setupImage,
	setupManifest,
	setupBlob,
}

// Opt function to process options on sandbox
type Opt func(*Sandbox)

// New creates a new sandbox
func New(name string, opts ...Opt) *Sandbox {
	// TODO: consider removing default methods from lua state
	ls := lua.NewState()

	s := &Sandbox{
		name:   name,
		ls:     ls,
		dryRun: false,
	}
	for _, opt := range opts {
		opt(s)
	}
	// default values for various options
	if s.ctx == nil {
		s.ctx = context.Background()
	}
	if s.log == nil {
		s.log = &logrus.Logger{}
	}
	if s.rc == nil {
		s.rc = regclient.New()
	}

	// setup modules for the sandbox
	for _, mod := range luaMods {
		mod(s)
	}

	// add other global functions to sandbox
	fn := s.ls.NewFunction(s.sandboxLog)
	s.ls.SetGlobal("log", fn)

	return s
}

// WithContext defines the context for a sandbox
func WithContext(ctx context.Context) Opt {
	return func(s *Sandbox) {
		s.ctx = ctx
	}
}

// WithDryRun indicates external changes should only be logged
func WithDryRun() Opt {
	return func(s *Sandbox) {
		s.dryRun = true
	}
}

// WithLog specifies a logrus logger
func WithLog(log *logrus.Logger) Opt {
	return func(s *Sandbox) {
		s.log = log
	}
}

// WithRegClient specifies a regclient interface
func WithRegClient(rc *regclient.RegClient) Opt {
	return func(s *Sandbox) {
		s.rc = rc
	}
}

// WithThrottle is used to limit various actions
func WithThrottle(t *throttle.Throttle) Opt {
	return func(s *Sandbox) {
		s.throttleC = t
	}
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
func (s *Sandbox) RunScript(script string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.log.WithFields(logrus.Fields{
				"script": s.name,
				"error":  r,
			}).Error("Runtime error from script")
			err = ErrScriptFailed
		}
	}()
	return s.ls.DoString(script)
}

// Close is use to stop the sandbox
func (s *Sandbox) Close() {
	s.ls.Close()
}

func (s *Sandbox) sandboxLog(ls *lua.LState) int {
	msg := ls.CheckString(1)
	s.log.WithFields(logrus.Fields{
		"script":  s.name,
		"message": msg,
	}).Info("User script message")
	return 0
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
	wrapTab := go2lua.Export(ls, wrapVal)
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
		// returned ud looks like:
		// ud:
		//   Value: udVal
		//   MetaTable: wrapMT
		//     __tostring: points to __tostring from ud Type's MT
		//     __index: wrapTab (exported table from go2lua)
		//       Metatable: ud Type's MT
	} else {
		ls.SetMetatable(ud, udTypeMT)
	}

	return ud, nil
}

// unwrapUserData extracts the user visible table from the userdata
func unwrapUserData(ls *lua.LState, lv lua.LValue) (lua.LValue, error) {
	if lv.Type() != lua.LTUserData {
		return nil, ErrInvalidInput
	}

	udMT := ls.GetMetaField(lv, "__index")
	if udMT.Type() != lua.LTTable {
		return nil, ErrInvalidInput
	}

	return udMT, nil
}
