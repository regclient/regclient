// Package reqresp is used to create mock web servers for testing
package reqresp

import (
	"bytes"
	"encoding/base64"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"testing"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
)

type ReqResp struct {
	ReqEntry  ReqEntry
	RespEntry RespEntry
}

type ReqEntry struct {
	Name     string
	DelOnUse bool
	IfState  []string
	SetState string
	Method   string
	Path     string
	PathRE   *regexp.Regexp
	Query    map[string][]string
	Headers  http.Header
	Body     []byte
}

type RespEntry struct {
	Status  int
	Headers http.Header
	Body    []byte
	Fail    bool // triggers a handler panic that simulates a server/connection failure
}

func NewHandler(t *testing.T, rrs []ReqResp) http.Handler {
	r := rrHandler{
		t:   t,
		rrs: rrs,
	}
	return &r
}

type rrHandler struct {
	t     *testing.T
	rrs   []ReqResp
	state string
	mu    sync.Mutex
}

// return false if any item in a is not found in b
func strMapMatch(a, b map[string][]string) bool {
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		for _, ave := range av {
			if !slices.Contains(bv, ave) {
				return false
			}
		}
	}
	return true
}

func (r *rrHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		r.t.Errorf("Error reading request body: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte("Error reading request body"))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, rr := range r.rrs {
		reqMatch := rr.ReqEntry
		if (len(reqMatch.IfState) > 0 && !slices.Contains(reqMatch.IfState, r.state)) ||
			reqMatch.Method != req.Method ||
			(reqMatch.PathRE != nil && !reqMatch.PathRE.MatchString(req.URL.Path)) ||
			(reqMatch.Path != "" && reqMatch.Path != req.URL.Path) ||
			!strMapMatch(reqMatch.Query, req.URL.Query()) ||
			!strMapMatch(reqMatch.Headers, req.Header) ||
			(len(reqMatch.Body) > 0 && !bytes.Equal(reqMatch.Body, reqBody)) {
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
		_, _ = io.Copy(rw, bytes.NewReader(rr.RespEntry.Body))

		// for single use test cases, delete this entry
		if reqMatch.DelOnUse {
			r.rrs = slices.Delete(r.rrs, i, i+1)
		}
		// update current state
		if reqMatch.SetState != "" {
			r.state = reqMatch.SetState
		}

		// handle failures
		if rr.RespEntry.Fail {
			panic(http.ErrAbortHandler)
		}
		return
	}
	r.t.Errorf("Unhandled request: %v, body: %s, state: %s", req, reqBody, r.state)
	rw.WriteHeader(http.StatusInternalServerError)
	_, _ = rw.Write([]byte("Unsupported request"))
}

// BaseEntries initial entries for a generic docker registry
var BaseEntries = []ReqResp{
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

// NewRandomBlob outputs a reproducible random blob (based on the seed) for testing
func NewRandomBlob(size int, seed int64) (digest.Digest, []byte) {
	//#nosec G404 regresp is only used for testing
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, size)
	if n, err := r.Read(b); err != nil {
		panic(err)
	} else if n != size {
		panic("unable to read enough bytes")
	}
	return digest.Canonical.FromBytes(b), b
}

// NewRandomID outputs a reproducible random ID (based on the seed) appropriate for blob upload URLs.
func NewRandomID(seed int64) string {
	//#nosec G404 regresp is only used for testing
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, 16)
	_, err := r.Read(b)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
