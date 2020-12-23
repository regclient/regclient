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
			"__tostring": s.manifestJSON,
			"get":        s.manifestGet,
			"head":       s.manifestHead,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"config": s.configGet,
				"delete": s.manifestDelete,
				"get":    s.manifestGet,
				// "head":      s.manifestHead,
				"ratelimit": s.imageRateLimit,
			},
		},
	)
	s.setupMod(
		luaImageName,
		map[string]lua.LGFunction{
			"config":       s.configGet,
			"copy":         s.imageCopy,
			"manifest":     s.manifestGet,
			"manifestHead": s.manifestHead,
			"manifestList": s.manifestGetList,
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
			"__tostring": s.configJSON,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {},
		},
	)
}

func (s *Sandbox) checkConfig(ls *lua.LState, i int) *config {
	var c *config
	switch ls.Get(i).Type() {
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		udc, ok := ud.Value.(*config)
		if !ok {
			ls.ArgError(i, "config expected")
		}
		c = udc
	default:
		ls.ArgError(i, "config expected")
	}
	return c
}

func (s *Sandbox) checkManifest(ls *lua.LState, i int, list bool) *manifest {
	var m *manifest
	switch ls.Get(i).Type() {
	case lua.LTString:
		ref, err := regclient.NewRef(ls.CheckString(1))
		if err != nil {
			ls.RaiseError("reference parsing failed: %v", err)
		}
		rcM, err := s.rcManifestGet(ref, list, "")
		if err != nil {
			ls.RaiseError("manifest pull failed: %v", err)
		}
		m = &manifest{m: rcM, ref: ref}
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *manifest:
			m = ud.Value.(*manifest)
		case *config:
			c := ud.Value.(*config)
			m = &manifest{ref: c.ref, m: c.m}
		default:
			ls.ArgError(i, "manifest expected")
		}
	default:
		ls.ArgError(i, "manifest expected")
	}
	return m
}

func (s *Sandbox) configGet(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false)
	confDigest, err := m.m.GetConfigDigest()
	if err != nil {
		ls.RaiseError("Failed looking up \"%s\" config digest: %v", m.ref.CommonName(), err)
	}

	conf, err := s.rc.ImageGetConfig(s.ctx, m.ref, confDigest.String())
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" config: %v", m.ref.CommonName(), err)
	}
	ud, err := wrapUserData(ls, &config{conf: &conf, m: m.m, ref: m.ref}, conf, luaImageConfigName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" config: %v", m.ref.CommonName(), err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) configJSON(ls *lua.LState) int {
	c := s.checkConfig(ls, 1)
	cJSON, err := json.MarshalIndent(c.conf, "", "  ")
	if err != nil {
		ls.RaiseError("Failed outputing config: %v", err)
	}
	ls.Push(lua.LString(string(cJSON)))
	return 1
}

func (s *Sandbox) imageCopy(ls *lua.LState) int {
	src := s.checkReference(ls, 1)
	tgt := s.checkReference(ls, 2)
	err := s.rc.ImageCopy(s.ctx, src.Ref, tgt.Ref)
	if err != nil {
		ls.RaiseError("Failed copying \"%s\" to \"%s\": %v", src.Ref.CommonName(), tgt.Ref.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) manifestGet(ls *lua.LState) int {
	return s.manifestGetWithOpts(ls, false)
}

func (s *Sandbox) manifestGetList(ls *lua.LState) int {
	return s.manifestGetWithOpts(ls, true)
}

func (s *Sandbox) manifestGetWithOpts(ls *lua.LState, list bool) int {
	ref := s.checkReference(ls, 1)
	plat := ""
	if !list && ls.GetTop() == 2 {
		plat = ls.CheckString(2)
	}
	m, err := s.rcManifestGet(ref.Ref, list, plat)
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}

	ud, err := wrapUserData(ls, &manifest{m: m, ref: ref.Ref}, m.GetOrigManifest(), luaManifestName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) manifestHead(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)

	m, err := s.rc.ManifestHead(s.ctx, ref.Ref)
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}

	ud, err := wrapUserData(ls, &manifest{m: m, ref: ref.Ref}, m, luaManifestName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" manifest: %v", ref.Ref.CommonName(), err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) imageRateLimit(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false)
	rl := go2lua.Convert(ls, m.m.GetRateLimit())
	ls.Push(rl)
	return 1
}

func (s *Sandbox) manifestDelete(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, true)
	ref := m.ref
	if ref.Digest == "" {
		d := m.m.GetDigest()
		ref.Digest = d.String()
	}
	err := s.rc.ManifestDelete(s.ctx, ref)
	if err != nil {
		ls.RaiseError("Failed deleting \"%s\": %v", ref.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) manifestJSON(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false)
	mJSON, err := json.MarshalIndent(m.m, "", "  ")
	if err != nil {
		ls.RaiseError("Failed outputing manifest: %v", err)
	}
	ls.Push(lua.LString(string(mJSON)))
	return 1
}

func (s *Sandbox) rcManifestGet(ref regclient.Ref, list bool, platform string) (regclient.Manifest, error) {
	m, err := s.rc.ManifestGet(s.ctx, ref)
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
		m, err = s.rc.ManifestGet(s.ctx, ref)
		if err != nil {
			return m, err
		}
	}

	return m, nil
}
