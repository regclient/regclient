package sandbox

import (
	"context"
	"encoding/json"
	"time"

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
	conf regclient.BlobOCIConfig
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
			"getList":    s.manifestGetList,
			"head":       s.manifestHead,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"config":        s.configGet,
				"delete":        s.manifestDelete,
				"get":           s.manifestGet,
				"head":          s.manifestHead,
				"ratelimit":     s.imageRateLimit,
				"ratelimitWait": s.imageRateLimitWait,
			},
		},
	)
	s.setupMod(
		luaImageName,
		map[string]lua.LGFunction{
			"config":        s.configGet,
			"copy":          s.imageCopy,
			"manifest":      s.manifestGet,
			"manifestHead":  s.manifestHead,
			"manifestList":  s.manifestGetList,
			"ratelimitWait": s.imageRateLimitWait,
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

func (s *Sandbox) checkManifest(ls *lua.LState, i int, list bool, head bool) *manifest {
	var m *manifest
	switch ls.Get(i).Type() {
	case lua.LTString:
		ref, err := regclient.NewRef(ls.CheckString(1))
		if err != nil {
			ls.RaiseError("reference parsing failed: %v", err)
		}
		if head {
			rcM, err := s.rc.ManifestHead(s.ctx, ref)
			if err != nil {
				ls.RaiseError("Failed retrieving \"%s\" manifest: %v", ref.CommonName(), err)
			}
			m = &manifest{m: rcM, ref: ref}
		} else {
			rcM, err := s.rcManifestGet(ref, list, "")
			if err != nil {
				ls.RaiseError("manifest pull failed: %v", err)
			}
			m = &manifest{m: rcM, ref: ref}
		}
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *manifest:
			m = ud.Value.(*manifest)
		case *config:
			c := ud.Value.(*config)
			m = &manifest{ref: c.ref, m: c.m}
		case *reference:
			r := ud.Value.(*reference)
			if head {
				rcM, err := s.rc.ManifestHead(s.ctx, r.ref)
				if err != nil {
					ls.RaiseError("Failed retrieving \"%s\" manifest: %v", r.ref.CommonName(), err)
				}
				m = &manifest{m: rcM, ref: r.ref}
			} else {
				rcM, err := s.rcManifestGet(r.ref, list, "")
				if err != nil {
					ls.RaiseError("manifest pull failed: %v", err)
				}
				m = &manifest{m: rcM, ref: r.ref}
			}
		default:
			ls.ArgError(i, "manifest expected")
		}
	default:
		ls.ArgError(i, "manifest expected")
	}
	return m
}

func (s *Sandbox) configGet(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false, false)
	if s.sem != nil {
		s.sem.Acquire(s.ctx, 1)
		defer s.sem.Release(1)
	}
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"image":  m.ref.CommonName(),
	}).Debug("Retrieve image config")
	confDigest, err := m.m.GetConfigDigest()
	if err != nil {
		ls.RaiseError("Failed looking up \"%s\" config digest: %v", m.ref.CommonName(), err)
	}

	confBlob, err := s.rc.BlobGetOCIConfig(s.ctx, m.ref, confDigest.String())
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" config: %v", m.ref.CommonName(), err)
	}
	ud, err := wrapUserData(ls, &config{conf: confBlob, m: m.m, ref: m.ref}, confBlob.GetConfig(), luaImageConfigName)
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
	if s.sem != nil {
		s.sem.Acquire(s.ctx, 1)
		defer s.sem.Release(1)
	}
	s.log.WithFields(logrus.Fields{
		"script":  s.name,
		"source":  src.ref.CommonName(),
		"target":  tgt.ref.CommonName(),
		"dry-run": s.dryRun,
	}).Info("Copy image")
	if s.dryRun {
		return 0
	}
	err := s.rc.ImageCopy(s.ctx, src.ref, tgt.ref)
	if err != nil {
		ls.RaiseError("Failed copying \"%s\" to \"%s\": %v", src.ref.CommonName(), tgt.ref.CommonName(), err)
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

	ud, err := wrapUserData(ls, &manifest{m: m, ref: ref.ref}, m.GetOrigManifest(), luaManifestName)
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

	ud, err := wrapUserData(ls, &manifest{m: m, ref: ref.ref}, m, luaManifestName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" manifest: %v", ref.ref.CommonName(), err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) imageRateLimit(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false, true)
	rl := go2lua.Convert(ls, m.m.GetRateLimit())
	ls.Push(rl)
	return 1
}

// imageRateLimitWait takes a ref, limit, poll freq, timeout, returns a bool for success
func (s *Sandbox) imageRateLimitWait(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	limit := ls.CheckInt(2)
	top := ls.GetTop()
	var freq time.Duration
	if top >= 3 {
		freqStr := ls.CheckString(3)
		freqParsed, err := time.ParseDuration(freqStr)
		if err != nil {
			ls.RaiseError("Failed parsing rate limit frequency %s: %v", freqStr, err)
			return 0
		}
		freq = freqParsed
	} else {
		freq, _ = time.ParseDuration("5m")
	}
	var timeout time.Duration
	if top >= 4 {
		timeoutStr := ls.CheckString(4)
		timeoutParsed, err := time.ParseDuration(timeoutStr)
		if err != nil {
			ls.RaiseError("Failed parsing timeout %s: %v", timeoutStr, err)
			return 0
		}
		timeout = timeoutParsed
	} else {
		timeout, _ = time.ParseDuration("6h")
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	for {
		// check the current manifest head
		mh, err := s.rc.ManifestHead(ctx, ref.ref)
		if err != nil {
			ls.RaiseError("Failed checking \"%s\" manifest: %v", ref.ref.CommonName(), err)
			return 0
		}
		// success if rate limit not set or remaining is above our limit
		rl := mh.GetRateLimit()
		if !rl.Set || rl.Remain >= limit {
			ls.Push(lua.LBool(true))
			return 1
		}
		// delay for freq (until timeout reached), and then retry
		s.log.WithFields(logrus.Fields{
			"script":  s.name,
			"image":   ref.ref.CommonName(),
			"current": rl.Remain,
			"target":  limit,
			"delay":   freq.String(),
		}).Info("Delaying for ratelimit")
		select {
		case <-ctx.Done():
			ls.Push(lua.LBool(false))
			return 1
		case <-time.After(freq):
		}
	}
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

func (s *Sandbox) manifestJSON(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false, false)
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
