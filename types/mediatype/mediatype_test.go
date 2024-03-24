package mediatype

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
