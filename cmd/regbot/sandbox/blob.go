package sandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type sbBlob struct {
	d   digest.Digest
	b   blob.Blob
	r   ref.Ref
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

// func (s *Sandbox) checkBlob(ls *lua.LState, i int, head bool) *sbBlob {
// 	var b *sbBlob
// 	switch ls.Get(i).Type() {
// 	case lua.LTString:
// 		r, err := ref.New(ls.CheckString(1))
// 		if err != nil {
// 			ls.RaiseError("reference parsing failed: %v", err)
// 		}
// 		if head {
// 			rcB, err := s.rc.BlobHead(s.ctx, r, digest.Digest(r.Digest))
// 			if err != nil {
// 				ls.RaiseError("Failed retrieving \"%s\" blob: %v", r.CommonName(), err)
// 			}
// 			b = &sbBlob{b: rcB, r: r, d: digest.Digest(r.Digest)}
// 		} else {
// 			rcB, err := s.rc.BlobGet(s.ctx, r, digest.Digest(r.Digest))
// 			if err != nil {
// 				ls.RaiseError("Blob pull failed: %v", err)
// 			}
// 			b = &sbBlob{b: rcB, r: r, d: digest.Digest(r.Digest)}
// 		}
// 	case lua.LTUserData:
// 		ud := ls.CheckUserData(i)
// 		switch ud.Value.(type) {
// 		case *sbBlob:
// 			b = ud.Value.(*sbBlob)
// 		case *config:
// 			c := ud.Value.(*config)
// 			b = &sbBlob{b: c.conf, r: c.r, d: digest.Digest(c.r.Digest)}
// 		case *reference:
// 			r := ud.Value.(*reference).r
// 			if head {
// 				rcB, err := s.rc.BlobHead(s.ctx, r, digest.Digest(r.Digest))
// 				if err != nil {
// 					ls.RaiseError("Failed retrieving \"%s\" blob: %v", r.CommonName(), err)
// 				}
// 				b = &sbBlob{b: rcB, r: r, d: digest.Digest(r.Digest)}
// 			} else {
// 				rcB, err := s.rc.BlobGet(s.ctx, r, digest.Digest(r.Digest))
// 				if err != nil {
// 					ls.RaiseError("Blob pull failed: %v", err)
// 				}
// 				b = &sbBlob{b: rcB, r: r, d: digest.Digest(r.Digest)}
// 			}
// 		default:
// 			ls.ArgError(i, "blob expected")
// 		}
// 	default:
// 		ls.ArgError(i, "blob expected")
// 	}
// 	return b
// }

func (s *Sandbox) blobGet(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	d := r.r.Digest
	if ls.GetTop() >= 2 {
		d = ls.CheckString(2)
	}
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"ref":    r.r.CommonName(),
		"digest": d,
	}).Debug("Retrieve blob")
	b, err := s.rc.BlobGet(s.ctx, r.r, digest.Digest(d))
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" blob \"%s\": %v", r.r.CommonName(), d, err)
	}

	ud, err := wrapUserData(ls, &sbBlob{b: b, r: r.r, rdr: b, d: digest.Digest(d)}, nil, luaBlobName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" blob \"%s\": %v", r.r.CommonName(), d, err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) blobHead(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	d := r.r.Digest
	if ls.GetTop() >= 2 {
		d = ls.CheckString(2)
	}
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"ref":    r.r.CommonName(),
		"digest": d,
	}).Debug("Retrieve blob")
	b, err := s.rc.BlobHead(s.ctx, r.r, digest.Digest(d))
	if err != nil {
		ls.RaiseError("Failed retrieving \"%s\" blob \"%s\": %v", r.r.CommonName(), d, err)
	}

	ud, err := wrapUserData(ls, &sbBlob{b: b, r: r.r, d: digest.Digest(d)}, nil, luaBlobName)
	if err != nil {
		ls.RaiseError("Failed packaging \"%s\" blob \"%s\": %v", r.r.CommonName(), d, err)
	}
	ls.Push(ud)
	return 1
}

func (s *Sandbox) blobPut(ls *lua.LState) int {
	err := s.ctx.Err()
	if err != nil {
		ls.RaiseError("Context error: %v", err)
	}
	r := s.checkReference(ls, 1)
	var d digest.Digest
	s.log.WithFields(logrus.Fields{
		"script": s.name,
		"ref":    r.r.CommonName(),
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
			d = b.d
		case *config:
			c := ud.Value.(*config)
			cJSON, _ := json.Marshal(c.conf)
			rdr = bytes.NewReader(cJSON)
		}
	}
	if rdr == nil {
		ls.ArgError(2, "blob content expected")
	}

	d, size, err := s.rc.BlobPut(s.ctx, r.r, d, rdr, 0)
	if err != nil {
		ls.RaiseError("Failed to put blob: %v", err)
	}

	ls.Push(lua.LString(d.String()))
	ls.Push(lua.LNumber(size))

	return 2
}
