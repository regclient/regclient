package reqresp

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
)

type ReqResp struct {
	ReqEntry  ReqEntry
	RespEntry RespEntry
}

type ReqEntry struct {
	Name     string
	DelOnUse bool
	Method   string
	Path     string
	Query    map[string][]string
	Headers  http.Header
	Body     []byte
}

type RespEntry struct {
	Status  int
	Headers http.Header
	Body    []byte
}

var rrBaseEntries = []ReqResp{
	{
		ReqEntry: ReqEntry{
			Method: "GET",
			Path:   "/v2/",
		},
		RespEntry: RespEntry{
			Status: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Docker-Distribution-API-Version": {"registry/2.0"},
			}),
		},
	},
}

func NewHandler(t *testing.T, rrs []ReqResp) http.Handler {
	r := rrHandler{
		t:   t,
		rrs: rrs,
	}
	return &r
}

type rrHandler struct {
	t   *testing.T
	rrs []ReqResp
}

// return false if any item in a is not found in b
func strMapMatch(a, b map[string][]string) bool {
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		for _, ave := range av {
			found := false
			for _, bve := range bv {
				if ave == bve {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func (r *rrHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	reqBody, err := ioutil.ReadAll(req.Body)
	if err != nil {
		r.t.Errorf("Error reading request body: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte("Error reading request body"))
		return
	}
	for i, rr := range r.rrs {
		reqMatch := rr.ReqEntry
		if reqMatch.Method != req.Method ||
			reqMatch.Path != req.URL.Path ||
			!strMapMatch(reqMatch.Query, req.URL.Query()) ||
			!strMapMatch(reqMatch.Headers, req.Header) ||
			bytes.Compare(reqMatch.Body, reqBody) != 0 {
			// skip if any field does not match
			continue
		}

		// respond
		r.t.Logf("Sending response %s", reqMatch.Name)
		rwHeader := rw.Header()
		for k, v := range rr.RespEntry.Headers {
			rwHeader[k] = v
		}
		if rr.RespEntry.Status != 0 {
			rw.WriteHeader(rr.RespEntry.Status)
		}
		io.Copy(rw, bytes.NewReader(rr.RespEntry.Body))

		// for single use test cases, delete this entry
		if reqMatch.DelOnUse {
			r.rrs = append(r.rrs[:i], r.rrs[i+1:]...)
		}
		return
	}
	r.t.Errorf("Unhandled request: %v", req)
	rw.WriteHeader(http.StatusInternalServerError)
	rw.Write([]byte("Unsupported request"))
	return
}
