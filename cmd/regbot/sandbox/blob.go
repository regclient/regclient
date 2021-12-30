package sandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/regclient/blob"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type sbBlob struct {
	d   digest.Digest
	b   blob.Blob
	ref types.Ref
	rdr io.Reader
}

func setupBlob(s *Sandbox) {
	s.setupMod(
		luaBlobName,
		map[string]lua.LGFunction{
			// "__tostring": s.blobContent,
			"get":  s.blobGet,
			"head": s.blobHead,
			"put":  s.blobPut,
		},
		map[string]map[string]lua.LGFunction{
			"__index": {
				"get":  s.blobGet,
				"head": s.blobHead,
				"put":  s.blobPut,
			},
		},
	)
}

func (s *Sandbox) checkBlob(ls *lua.LState, i int, head bool) *sbBlob {
	var b *sbBlob
	switch ls.Get(i).Type() {
	case lua.LTString:
		ref, err := types.NewRef(ls.CheckString(1))
		if err != nil {
			ls.RaiseError("reference parsing failed: %v", err)
		}
		if head {
			rcB, err := s.rc.BlobHead(s.ctx, ref, digest.Digest(ref.Digest))
			if err != nil {
				ls.RaiseError("Failed retrieving \"%s\" blob: %v", ref.CommonName(), err)
			}
			b = &sbBlob{b: rcB, ref: ref, d: digest.Digest(ref.Digest)}
		} else {
			rcB, err := s.rc.BlobGet(s.ctx, ref, digest.Digest(ref.Digest))
			if err != nil {
				ls.RaiseError("Blob pull failed: %v", err)
			}
			b = &sbBlob{b: rcB, ref: ref, d: digest.Digest(ref.Digest)}
		}
	case lua.LTUserData:
		ud := ls.CheckUserData(i)
		switch ud.Value.(type) {
		case *sbBlob:
			b = ud.Value.(*sbBlob)
		case *config:
			c := ud.Value.(*config)
			b = &sbBlob{b: c.conf, ref: c.ref, d: digest.Digest(c.ref.Digest)}
		case *reference:
			ref := ud.Value.(*reference).ref
			if head {
				rcB, err := s.rc.BlobHead(s.ctx, ref, digest.Digest(ref.Digest))
				if err != nil {
					ls.RaiseError("Failed retrieving \"%s\" blob: %v", ref.CommonName(), err)
				}
				b = &sbBlob{b: rcB, ref: ref, d: digest.Digest(ref.Digest)}
			} else {
				rcB, err := s.rc.BlobGet(s.ctx, ref, digest.Digest(ref.Digest))
				if err != nil {
					ls.RaiseError("Blob pull failed: %v", err)
				}
				b = &sbBlob{b: rcB, ref: ref, d: digest.Digest(ref.Digest)}
			}
		default:
			ls.ArgError(i, "blob expected")
		}
	default:
		ls.ArgError(i, "blob expected")
	}
	return b
}

func (s *Sandbox) blobGet(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	d := ref.ref.Digest
	if ls.GetTop() >= 2 {
		d = ls.CheckString(2)
	}
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"ref":    ref.ref.CommonName(),
		"digest": d,
	}).Debug("Retrieve blob")
	b, err := s.rc.BlobGet(s.ctx, ref.ref, digest.Digest(d))
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" blob \"%s\": %v", ref.ref.CommonName(), d, err)
	}

	ud, err := wrapUserData(ls, &sbBlob{b: b, ref: ref.ref, rdr: b}, nil, luaBlobName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" blob \"%s\": %v", ref.ref.CommonName(), d, err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) blobHead(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	d := ref.ref.Digest
	if ls.GetTop() >= 2 {
		d = ls.CheckString(2)
	}
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"ref":    ref.ref.CommonName(),
		"digest": d,
	}).Debug("Retrieve blob")
	b, err := s.rc.BlobHead(s.ctx, ref.ref, digest.Digest(d))
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" blob \"%s\": %v", ref.ref.CommonName(), d, err)
	}

	ud, err := wrapUserData(ls, &sbBlob{b: b, ref: ref.ref}, nil, luaBlobName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" blob \"%s\": %v", ref.ref.CommonName(), d, err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) blobPut(ls *lua.LState) int {
	ref := s.checkReference(ls, 1)
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"ref":    ref.ref.CommonName(),
	}).Debug("Put blob")

	if ls.GetTop() < 2 {
		ls.ArgError(2, "blob content expected")
	}

	var rdr io.Reader
	switch ls.Get(2).Type() {
	case lua.LTString:
		str := ls.CheckString(1)
		rdr = strings.NewReader(str)
	case lua.LTUserData:
		ud := ls.CheckUserData(2)
		switch ud.Value.(type) {
		case *sbBlob:
			b := ud.Value.(*sbBlob)
			rdr = b.rdr
		case *config:
			c := ud.Value.(*config)
			cJSON, _ := json.Marshal(c.conf)
			rdr = bytes.NewReader(cJSON)
		}
	}
	if rdr == nil {
		ls.ArgError(2, "blob content expected")
	}

	d, size, err := s.rc.BlobPut(s.ctx, ref.ref, "", rdr, 0)
	if err != nil {
		ls.RaiseError("Failed to put blob: %v", err)
	}

	ls.Push(lua.LString(d.String()))
	ls.Push(lua.LNumber(size))

	return 2
}
