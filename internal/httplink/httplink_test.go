package httplink

import (
	"errors"
	"testing"

	"github.com/regclient/regclient/types"
)

func TestParseErr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		headers []string
	}{
		{
			name:    "open uri",
			headers: []string{"</path"},
		},
		{
			name:    "open quote",
			headers: []string{`</path>; rel="next`},
		},
		{
			name:    "backslash",
			headers: []string{"\\"},
		},
		{
			name:    "backslash uri",
			headers: []string{"/\\"},
		},
		{
			name:    "missing separator",
			headers: []string{`</path>; rel="next" media="print"`},
		},
		{
			name:    "invalid parm",
			headers: []string{`</path>; r&d="nope"`},
		},
		{
			name:    "invalid star",
			headers: []string{`</path>; r*d="nope"`},
		},
		{
			name:    "invalid val start",
			headers: []string{`</path>; rel=\`},
		},
		{
			name:    "invalid val mid",
			headers: []string{`</path>; rel=a\b`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.headers)
			if err == nil {
				t.Error("parse did not fail")
			}
		})
	}
}

func TestParseGet(t *testing.T) {
	t.Parallel()
	headerComplex := []string{`</path1>; rel="next", </path2> ; rel="next", / ; rel=index `, `</print>; media=print`, `/link, /reader; media=reader; type=x`}
	tests := []struct {
		name          string
		headers       []string
		parm, val     string
		expectURI     string
		expectMissing bool
	}{
		{
			name:      "quoted rel next",
			headers:   []string{`</path>; rel="next"`},
			parm:      "rel",
			val:       "next",
			expectURI: "/path",
		},
		{
			name:      "complex rel=next",
			headers:   headerComplex,
			parm:      "rel",
			val:       "next",
			expectURI: "/path1",
		},
		{
			name:      "complex rel=index",
			headers:   headerComplex,
			parm:      "rel",
			val:       "index",
			expectURI: "/",
		},
		{
			name:      "complex media=print",
			headers:   headerComplex,
			parm:      "media",
			val:       "print",
			expectURI: "/print",
		},
		{
			name:          "complex not found",
			headers:       headerComplex,
			parm:          "rel",
			val:           "unknown",
			expectMissing: true,
		},
		{
			name: "rfc example",
			headers: []string{`</TheBook/chapter2>;	rel="previous"; title*=UTF-8'de'letztes%20Kapitel,
			                   </TheBook/chapter4>; rel="next"; title*=UTF-8'de'n%c3%a4chstes%20Kapitel`},
			parm:      "rel",
			val:       "next",
			expectURI: "/TheBook/chapter4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, err := Parse(tt.headers)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			link, err := links.Get(tt.parm, tt.val)
			if tt.expectMissing {
				if err == nil || !errors.Is(err, types.ErrNotFound) {
					t.Errorf("did not find missing error: %v, %v", link, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("failed to run get: %v", err)
			}
			if link.URI != tt.expectURI {
				t.Errorf("URI mismatch: expected %s, received %s", tt.expectURI, link.URI)
			}
		})
	}
}
