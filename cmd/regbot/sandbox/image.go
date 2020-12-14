package sandbox

import (
	"encoding/json"

	"github.com/containerd/containerd/platforms"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/go2lua"
	"github.com/regclient/regclient/regclient"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type config struct {
	m    regclient.Manifest
	ref  regclient.Ref
	conf *ociv1.Image
}

type manifest struct {
	m   regclient.Manifest
	ref regclient.Ref
}

func setupImage(s *Sandbox) {
	s.setupMod(
		luaManifestName,
		map[string]lua.LGFunction{
			"__tostring": s.imageManifestJSON,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"config":    s.imageConfig,
				"ratelimit": s.imageRateLimit,
			},
		},
	)
	s.setupMod(
		luaImageName,
		map[string]lua.LGFunction{
			"config":       s.imageConfig,
			"manifest":     s.imageManifest,
			"manifestHead": s.imageManifestHead,
			"manifestList": s.imageManifestList,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				// "inspect": s.imageInspect,
			},
		},
	)
	s.setupMod(
		luaImageConfigName,
		map[string]lua.LGFunction{
			"__tostring": s.imageConfigJSON,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
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
		m = &manifest{m: rcM, ref: ref}
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

func (s *Sandbox) imageConfig(L *lua.LState) int {
	m := s.checkManifest(L, 1, false)
	confDigest, err := m.m.GetConfigDigest()
	if err != nil {
		L.RaiseError("Failed looking up \"%s\" config digest: %v", m.ref.CommonName(), err)
	}

	conf, err := s.RC.ImageGetConfig(s.Ctx, m.ref, confDigest.String())
	if err != nil {
		L.RaiseError("Failed retrieving \"%s\" config: %v", m.ref.CommonName(), err)
	}
	ud, err := wrapUserData(L, &config{conf: &conf, m: m.m, ref: m.ref}, conf, luaImageConfigName)
	if err != nil {
		L.RaiseError("Failed packaging \"%s\" config: %v", m.ref.CommonName(), err)
	}
	L.Push(ud)
	return 1
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

	ud, err := wrapUserData(L, &manifest{m: m, ref: ref.Ref}, m.GetOrigManifest(), luaManifestName)
	if err != nil {
		L.RaiseError("Failed packaging \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}
	L.Push(ud)
	return 1
}

func (s *Sandbox) imageManifestHead(L *lua.LState) int {
	ref := checkReference(L, 1)

	m, err := s.RC.ManifestHead(s.Ctx, ref.Ref)
	if err != nil {
		L.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}

	ud, err := wrapUserData(L, &manifest{m: m, ref: ref.Ref}, m, luaManifestName)
	if err != nil {
		L.RaiseError("Failed packaging \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}
	L.Push(ud)
	return 1
}

func (s *Sandbox) imageConfigJSON(L *lua.LState) int {
	c := checkConfig(L, 1)
	cJSON, err := json.MarshalIndent(c.conf, "", "  ")
	if err != nil {
		L.RaiseError("Failed outputing config: %v", err)
	}
	L.Push(lua.LString(string(cJSON)))
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

func (s *Sandbox) imageRateLimit(L *lua.LState) int {
	m := s.checkManifest(L, 1, false)
	rl := go2lua.Convert(L, m.m.GetRateLimit())
	L.Push(rl)
	return 1
}

func checkConfig(L *lua.LState, i int) *config {
	var c *config
	switch L.Get(i).Type() {
	case lua.LTUserData:
		ud := L.CheckUserData(i)
		udc, ok := ud.Value.(*config)
		if !ok {
			L.ArgError(i, "config expected")
		}
		c = udc
	default:
		L.ArgError(i, "config expected")
	}
	return c
}

/* func checkManifest(L *lua.LState, i int) *manifest {
	var man *manifest
	switch L.Get(i).Type() {
	case lua.LTUserData:
		ud := L.CheckUserData(i)
		m, ok := ud.Value.(*manifest)
		if !ok {
			L.ArgError(i, "manifest expected")
		}
		man = m
	default:
		L.ArgError(i, "manifest expected")
	}
	return man
} */

/* func isManifest(L *lua.LState, i int) bool {
	if L.Get(i).Type() != lua.LTUserData {
		return false
	}
	ud := L.CheckUserData(i)
	if _, ok := ud.Value.(*manifest); ok {
		return true
	}
	return false
} */
