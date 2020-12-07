package sandbox

import (
	"encoding/json"

	"github.com/containerd/containerd/platforms"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type manifest struct {
	m regclient.Manifest
}

func setupImage(s *Sandbox) {
	s.setupMod(
		luaManifestName,
		map[string]lua.LGFunction{
			"__tostring": s.imageManifestJSON,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
		},
	)
	s.setupMod(
		luaImageName,
		map[string]lua.LGFunction{
			"manifest": s.imageManifest,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				// "inspect": s.imageInspect,
			},
		},
	)
}

func (s *Sandbox) checkManifest(L *lua.LState, i int, list bool) *manifest {
	var m *manifest
	switch L.Get(i).Type() {
	case lua.LTString:
		ref, err := regclient.NewRef(L.CheckString(1))
		if err != nil {
			L.RaiseError("reference parsing failed: %v", err)
		}
		rcM, err := s.getManifest(ref, list, "")
		if err != nil {
			L.RaiseError("manifest pull failed: %v", err)
		}
		m = &manifest{m: rcM}
	case lua.LTUserData:
		ud := L.CheckUserData(i)
		udM, ok := ud.Value.(*manifest)
		if !ok {
			L.ArgError(i, "manifest expected")
		}
		m = udM
	}
	return m
}

func (s *Sandbox) getManifest(ref regclient.Ref, list bool, platform string) (regclient.Manifest, error) {
	m, err := s.RC.ManifestGet(s.Ctx, ref)
	if err != nil {
		return m, err
	}

	if m.IsList() && !list {
		var plat ociv1.Platform
		if platform != "" {
			plat, err = platforms.Parse(platform)
			if err != nil {
				s.log.WithFields(logrus.Fields{
					"platform": platform,
					"err":      err,
				}).Warn("Could not parse platform")
			}
		}
		if plat.OS == "" {
			plat = platforms.DefaultSpec()
		}
		desc, err := m.GetPlatformDesc(&plat)
		if err != nil {
			return m, err
		}
		ref.Digest = desc.Digest.String()
		m, err = s.RC.ManifestGet(s.Ctx, ref)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}

func (s *Sandbox) imageManifest(L *lua.LState) int {
	return s.imageManifestWithOpts(L, false)
}

func (s *Sandbox) imageManifestList(L *lua.LState) int {
	return s.imageManifestWithOpts(L, true)
}

func (s *Sandbox) imageManifestWithOpts(L *lua.LState, list bool) int {
	ref := checkReference(L, 1)
	plat := ""
	if !list && L.GetTop() == 2 {
		plat = L.CheckString(2)
	}
	m, err := s.getManifest(ref.Ref, list, plat)
	if err != nil {
		L.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}

	ud := L.NewUserData()
	ud.Value = &manifest{m: m}
	L.SetMetatable(ud, L.GetTypeMetatable(luaManifestName))
	L.Push(ud)
	return 1
}

func (s *Sandbox) imageManifestJSON(L *lua.LState) int {
	m := s.checkManifest(L, 1, false)
	mJSON, err := json.MarshalIndent(m.m, "", "  ")
	if err != nil {
		L.RaiseError("Failed outputing manifest: %v", err)
	}
	L.Push(lua.LString(string(mJSON)))
	return 1
}
