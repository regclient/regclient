package mediatype

import (
	"strings"
	"testing"
)

func TestBase(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name   string
		orig   string
		expect string
	}{
		{
			name:   "OCI Index",
			orig:   OCI1ManifestList,
			expect: OCI1ManifestList,
		},
		{
			name:   "OCI Index with charset",
			orig:   "application/vnd.oci.image.index.v1+json; charset=utf-8",
			expect: OCI1ManifestList,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := Base(tc.orig)
			if tc.expect != result {
				t.Errorf("invalid result: expected \"%s\", received \"%s\"", tc.expect, result)
			}
		})
	}
}

func TestValid(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name   string
		mt     string
		expect bool
	}{
		{
			name:   "Empty",
			mt:     "",
			expect: false,
		},
		{
			name:   "OCI-Index",
			mt:     OCI1ManifestList,
			expect: true,
		},
		{
			name:   "OCI-Index-param",
			mt:     "application/vnd.oci.image.index.v1+json; charset=utf-8",
			expect: false,
		},
		{
			name:   "no-slash",
			mt:     "application",
			expect: false,
		},
		{
			name:   "no-subtype",
			mt:     "application/",
			expect: false,
		},
		{
			name:   "invalid-character",
			mt:     "application/star.*",
			expect: false,
		},
		{
			name:   "missing-major-type",
			mt:     "/json",
			expect: false,
		},
		{
			name:   "too-long",
			mt:     "application/" + strings.Repeat("a", 128),
			expect: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			valid := Valid(tc.mt)
			if tc.expect != valid {
				t.Errorf("invalid result: expected \"%t\", received \"%t\"", tc.expect, valid)
			}
		})
	}
}
