package regclient

import (
	"math/rand"
	"net/http"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/reqresp"
)

var rrBaseEntries = []reqresp.ReqResp{
	{
		ReqEntry: reqresp.ReqEntry{
			Method: "GET",
			Path:   "/v2/",
		},
		RespEntry: reqresp.RespEntry{
			Status: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Docker-Distribution-API-Version": {"registry/2.0"},
			}),
		},
	},
}

// not using a cryptographically secure rand, instead use a reproducible one for testing
func newRandomBlob(size int, seed int64) (digest.Digest, []byte) {
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, size)
	if n, err := r.Read(b); err != nil {
		panic(err)
	} else if n != size {
		panic("unable to read enough bytes")
	}
	return digest.FromBytes(b), b
}
