package sandbox

import (
	"context"
	"encoding/json"
	"os"
	"time"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/go2lua"
	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/blob"
	"github.com/regclient/regclient/regclient/manifest"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type config struct {
	m    manifest.Manifest
	ref  types.Ref
	conf blob.OCIConfig
}

func setupImage(s *Sandbox) {
	s.setupMod(
		luaImageName,
		map[string]lua.LGFunction{
			"config":        s.configGet,
			"copy":          s.imageCopy,
			"exportTar":     s.imageExportTar,
			"importTar":     s.imageImportTar,
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
			"__index": {
				"export": s.configExport,
			},
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

	confBlob, err := s.rc.BlobGetOCIConfig(s.ctx, m.ref, confDigest)
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

// configExport recreates a new config object based on any user changes to the lua object
func (s *Sandbox) configExport(ls *lua.LState) int {
	var newC *config
	i := 1
	switch ls.Get(i).Type() {
	case lua.LTUserData:
		// unpack existing config from user data
		ud := ls.CheckUserData(i)
		origC, ok := ud.Value.(*config)
		if !ok {
			ls.ArgError(i, "config expected")
		}
		// unwrap extracts lua table that user may have modified
		utab, err := unwrapUserData(ls, ud)
		if err != nil {
			ls.RaiseError("failed exporting config (unwrap): %v", err)
		}
		// get the original config object, used to set fields that can be extracted from lua table
		origOCIConf := origC.conf.GetConfig()
		// build a new oci image from the lua table
		var ociImage ociv1.Image
		err = go2lua.Import(ls, utab, &ociImage, &origOCIConf)
		if err != nil {
			ls.RaiseError("Failed exporting config (go2lua): %v", err)
		}
		// save image to a new config
		bc := blob.NewOCIConfig(ociImage)
		newC = &config{
			conf: bc,
			m:    origC.m,
			ref:  origC.ref,
		}
	default:
		ls.ArgError(i, "Config expected")
	}
	// wrap config to send back to lua
	ud, err := wrapUserData(ls, newC, newC.conf.GetConfig(), luaImageConfigName)
	if err != nil {
		ls.RaiseError("Failed packaging config: %v", err)
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
	opts := []regclient.ImageOpts{}
	lOpts := struct {
		DigestTags     bool     `json:"digestTags"`
		ForceRecursive bool     `json:"forceRecursive"`
		Platforms      []string `json:"platforms"`
	}{}
	if ls.GetTop() == 3 {
		err := go2lua.Import(ls, ls.Get(3), &lOpts, lOpts)
		if err != nil {
			ls.RaiseError("Failed to parse options: %v", err)
		}
		if lOpts.DigestTags {
			opts = append(opts, regclient.ImageWithDigestTags())
		}
		if lOpts.ForceRecursive {
			opts = append(opts, regclient.ImageWithForceRecursive())
		}
		if len(lOpts.Platforms) > 0 {
			opts = append(opts, regclient.ImageWithPlatforms(lOpts.Platforms))
		}
	}
	if s.sem != nil {
		s.sem.Acquire(s.ctx, 1)
		defer s.sem.Release(1)
	}
	s.log.WithFields(logrus.Fields{
		"script":         s.name,
		"source":         src.ref.CommonName(),
		"target":         tgt.ref.CommonName(),
		"digestTags":     lOpts.DigestTags,
		"forceRecursive": lOpts.ForceRecursive,
		"dry-run":        s.dryRun,
	}).Info("Copy image")
	if s.dryRun {
		return 0
	}
	err := s.rc.ImageCopy(s.ctx, src.ref, tgt.ref, opts...)
	if err != nil {
		ls.RaiseError("Failed copying \"%s\" to \"%s\": %v", src.ref.CommonName(), tgt.ref.CommonName(), err)
	}
	return 0
}

func (s *Sandbox) imageExportTar(ls *lua.LState) int {
	src := s.checkReference(ls, 1)
	file := ls.CheckString(2)
	if s.sem != nil {
		s.sem.Acquire(s.ctx, 1)
		defer s.sem.Release(1)
	}
	fh, err := os.Create(file)
	if err != nil {
		ls.RaiseError("Failed to open \"%s\": %v", file, err)
	}
	err = s.rc.ImageExport(s.ctx, src.ref, fh)
	if err != nil {
		ls.RaiseError("Failed to export image \"%s\" to \"%s\": %v", src.ref.CommonName(), file, err)
	}
	return 0
}

func (s *Sandbox) imageImportTar(ls *lua.LState) int {
	tgt := s.checkReference(ls, 1)
	file := ls.CheckString(2)
	if s.sem != nil {
		s.sem.Acquire(s.ctx, 1)
		defer s.sem.Release(1)
	}
	rs, err := os.Open(file)
	if err != nil {
		ls.RaiseError("Failed to read from \"%s\": %v", file, err)
	}
	err = s.rc.ImageImport(s.ctx, tgt.ref, rs)
	if err != nil {
		ls.RaiseError("Failed to import image \"%s\" from \"%s\": %v", tgt.ref.CommonName(), file, err)
	}
	return 0
}

func (s *Sandbox) imageRateLimit(ls *lua.LState) int {
	m := s.checkManifest(ls, 1, false, true)
	rl := go2lua.Export(ls, m.m.GetRateLimit())
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
