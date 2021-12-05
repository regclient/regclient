package sandbox

import (
	"encoding/json"
	"reflect"

	"github.com/containerd/containerd/platforms"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/go2lua"
	"github.com/regclient/regclient/regclient/manifest"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type sbManifest struct {
	m   manifest.Manifest
	ref types.Ref
}

func setupManifest(s *Sandbox) {
	s.setupMod(
		luaManifestName,
		map[string]lua.LGFunction{
			"__tostring": s.manifestJSON,
			"get":        s.manifestGet,
			"getList":    s.manifestGetList,
			"head":       s.manifestHead,
			"put":        s.manifestPut,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"config":        s.configGet,
				"delete":        s.manifestDelete,
				"export":        s.manifestExport,
				"get":           s.manifestGet,
				"head":          s.manifestHead,
				"put":           s.manifestPut,
				"ratelimit":     s.imageRateLimit,
				"ratelimitWait": s.imageRateLimitWait,
			},
		},
	)
}

func (s *Sandbox) checkManifest(ls *lua.LState, i int, list bool, head bool) *sbManifest {
	var m *sbManifest
	switch ls.Get(i).Type() {
	case lua.LTString:
		ref, err := types.NewRef(ls.CheckString(1))
		if err != nil {
			ls.RaiseError("reference parsing failed: %v", err)
		}
		if head {
			rcM, err := s.rc.ManifestHead(s.ctx, ref)
			if err != nil {
				ls.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.CommonName(), err)
			}
			m = &sbManifest{m: rcM, ref: ref}
		} else {
			rcM, err := s.rcManifestGet(ref, list, "")
			if err != nil {
				ls.RaiseError("manifest pull failed: %v", err)
			}
			m = &sbManifest{m: rcM, ref: ref}
		}
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *sbManifest:
			m = ud.Value.(*sbManifest)
		case *config:
			c := ud.Value.(*config)
			m = &sbManifest{ref: c.ref, m: c.m}
		case *reference:
			r := ud.Value.(*reference)
			if head {
				rcM, err := s.rc.ManifestHead(s.ctx, r.ref)
				if err != nil {
					ls.RaiseError("Failed retrieving \"%s\" manifest: %v", r.ref.CommonName(), err)
				}
				m = &sbManifest{m: rcM, ref: r.ref}
			} else {
				rcM, err := s.rcManifestGet(r.ref, list, "")
				if err != nil {
					ls.RaiseError("manifest pull failed: %v", err)
				}
				m = &sbManifest{m: rcM, ref: r.ref}
			}
		default:
			ls.ArgError(i, "manifest expected")
		}
	default:
		ls.ArgError(i, "manifest expected")
	}
	return m
}

func (s *Sandbox) manifestDelete(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, true, true)
	ref := m.ref
	if ref.Digest == "" {
		d := m.m.GetDigest()
		ref.Digest = d.String()
	}
	s.log.WithFields(logrus.Fields{
		"script":  s.name,
		"image":   ref.CommonName(),
		"dry-run": s.dryRun,
	}).Info("Delete manifest")
	if s.dryRun {
		return 0
	}
	err := s.rc.ManifestDelete(s.ctx, ref)
	if err != nil {
		ls.RaiseError("Failed deleting \"%s\": %v", ref.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) manifestExport(ls *lua.LState) int {
	var newM *sbManifest
	i := 1
	switch ls.Get(i).Type() {
	case lua.LTUserData:
		// unpack existing manifest from user data
		ud := ls.CheckUserData(i)
		origM, ok := ud.Value.(*sbManifest)
		if !ok {
			ls.ArgError(i, "manifest expected")
		}
		// unwrap extracts lua table that user may have modified
		utab, err := unwrapUserData(ls, ud)
		if err != nil {
			ls.RaiseError("failed exporting config (unwrap): %v", err)
		}
		// get the original manifest object, used to set fields that can be extracted from lua table
		origMM := origM.m.GetOrigManifest()
		newMMP := reflect.New(reflect.TypeOf(origMM)).Interface()
		// newMMP is interface{} -> *someManifestType
		// because it's an empty interface, it needs to remain a "reflect.New" pointer
		// &newMMP is *interface{} -> *someManifestType, not **someManifestType
		err = go2lua.Import(ls, utab, &newMMP, origMM)
		if err != nil {
			ls.RaiseError("Failed exporting manifest (go2lua): %v", err)
		}
		// save image to a new manifest
		rcM, err := manifest.FromOrig(reflect.ValueOf(newMMP).Elem().Interface()) // reflect is needed again to deref the pointer now
		// rcM, err := manifest.FromOrig(newMM)
		if err != nil {
			ls.RaiseError("Failed exporting manifest (from orig): %v", err)
		}
		newM = &sbManifest{
			m:   rcM,
			ref: origM.ref,
		}
	default:
		ls.ArgError(i, "Manifest expected")
	}
	// wrap manifest to send back to lua
	ud, err := wrapUserData(ls, newM, newM.m.GetOrigManifest(), luaManifestName)
	if err != nil {
		ls.RaiseError("Failed packaging manifest: %v", err)
	}
	ls.Push(ud)
	return 1
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
	s.log.WithFields(logrus.Fields{
		"script":   s.name,
		"image":    ref.ref.CommonName(),
		"list":     list,
		"platform": plat,
	}).Debug("Retrieve manifest")
	m, err := s.rcManifestGet(ref.ref, list, plat)
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.ref.CommonName(), err)
	}

	ud, err := wrapUserData(ls, &sbManifest{m: m, ref: ref.ref}, m.GetOrigManifest(), luaManifestName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" manifest: %v", ref.ref.CommonName(), err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) manifestHead(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)

	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"image":  ref.ref.CommonName(),
	}).Debug("Retrieve manifest with head")

	m, err := s.rc.ManifestHead(s.ctx, ref.ref)
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.ref.CommonName(), err)
	}

	ud, err := wrapUserData(ls, &sbManifest{m: m, ref: ref.ref}, m, luaManifestName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" manifest: %v", ref.ref.CommonName(), err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) manifestJSON(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false, false)
	mJSON, err := json.MarshalIndent(m.m, "", "  ")
	if err != nil {
		ls.RaiseError("Failed outputing manifest: %v", err)
	}
	ls.Push(lua.LString(string(mJSON)))
	return 1
}

func (s *Sandbox) manifestPut(ls *lua.LState) int {
	sbm := s.checkManifest(ls, 1, true, false)
	ref := s.checkReference(ls, 2)
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"image":  ref.ref.CommonName(),
	}).Debug("Put manifest")

	m, err := manifest.FromOrig(sbm.m.GetOrigManifest())
	if err != nil {
		ls.RaiseError("Failed to put manifest: %v", err)
	}

	err = s.rc.ManifestPut(s.ctx, ref.ref, m)
	if err != nil {
		ls.RaiseError("Failed to put manifest: %v", err)
	}

	return 0
}

func (s *Sandbox) rcManifestGet(ref types.Ref, list bool, platform string) (manifest.Manifest, error) {
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
