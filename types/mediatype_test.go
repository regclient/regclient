package types

import "testing"

func TestMediaTypeBase(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name   string
		orig   string
		expect string
	}{
		{
			name:   "OCI Index",
			orig:   MediaTypeOCI1ManifestList,
			expect: MediaTypeOCI1ManifestList,
		},
		{
			name:   "OCI Index with charset",
			orig:   "application/vnd.oci.image.index.v1+json; charset=utf-8",
			expect: MediaTypeOCI1ManifestList,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := MediaTypeBase(tc.orig)
			if tc.expect != result {
				t.Errorf("invalid result: expected \"%s\", received \"%s\"", tc.expect, result)
			}
		})
	}
}
