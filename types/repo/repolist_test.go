package repo

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/regclient/regclient/types/errs"
)

func TestNew(t *testing.T) {
	t.Parallel()
	emptyRaw := []byte("{}")
	registryList := []string{"library/alpine", "library/debian", "library/golang"}
	registryRaw := fmt.Appendf(nil, `{"repositories":["%s"]}`, strings.Join(registryList, `","`))
	registryHost := "localhost:5000"
	registryMT := "application/json; charset=utf-8"
	registryHeaders := http.Header{
		"Content-Type": []string{registryMT},
	}

	tests := []struct {
		name string
		opts []Opts
		// all remaining fields are expected results from creating a tag with opts
		err   error
		raw   []byte
		repos []string
	}{
		{
			name: "Empty",
			opts: []Opts{
				WithRaw(emptyRaw),
			},
			raw: emptyRaw,
		},
		{
			name: "Registry",
			opts: []Opts{
				WithHost(registryHost),
				WithRaw(registryRaw),
				WithHeaders(registryHeaders),
				WithMT(registryMT),
			},
			raw:   registryRaw,
			repos: registryList,
		},
		{
			name: "Unknown MT",
			opts: []Opts{
				WithHost(registryHost),
				WithRaw(registryRaw),
				WithHeaders(registryHeaders),
				WithMT("application/unknown"),
			},
			err: errs.ErrUnsupportedMediaType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl, err := New(tt.opts...)
			if tt.err != nil {
				if err == nil || !errors.Is(err, tt.err) {
					t.Errorf("expected error not found, expected %v, received %v", tt.err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error creating tag list: %v", err)
			}
			raw, err := rl.RawBody()
			if err != nil {
				t.Fatalf("error from RawBody: %v", err)
			}
			if !bytes.Equal(tt.raw, raw) {
				t.Errorf("unexpected raw body: expected %s, received %s", tt.raw, raw)
			}
			repos, err := rl.GetRepos()
			if err != nil {
				t.Errorf("error from GetRepos: %v", err)
			} else if cmpSliceString(tt.repos, repos) == false {
				t.Errorf("unexpected repo list: expected %v, received %v", tt.repos, repos)
			}
		})
	}
}

func cmpSliceString(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
