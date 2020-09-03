package auth

import "testing"

func TestParseAuthHeader(t *testing.T) {
	var tests = []struct {
		name, in string
		wantC    []Challenge
		wantE    error
	}{
		{
			name:  "Bearer to auth.docker.io",
			in:    `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:docker/docker:pull"`,
			wantC: []Challenge{{authType: "bearer", params: map[string]string{"realm": "https://auth.docker.io/token", "service": "registry.docker.io", "scope": "repository:docker/docker:pull"}}},
			wantE: nil,
		},
		{
			name:  "Basic to GitHub",
			in:    `Basic realm="GitHub Package Registry"`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "GitHub Package Registry"}}},
			wantE: nil,
		},
		{
			name:  "Basic case insensitive type and key",
			in:    `BaSiC ReAlM="Case insensitive key"`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "Case insensitive key"}}},
			wantE: nil,
		},
		{
			name:  "Basic unquoted realm",
			in:    `Basic realm=unquoted`,
			wantC: []Challenge{{authType: "basic", params: map[string]string{"realm": "unquoted"}}},
			wantE: nil,
		},
		{
			name:  "Missing close quote",
			in:    `Basic realm="GitHub Package Registry`,
			wantC: []Challenge{},
			wantE: ErrParseFailure,
		},
		{
			name:  "Missing value after escape",
			in:    `Basic realm="GitHub Package Registry\\`,
			wantC: []Challenge{},
			wantE: ErrParseFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseAuthHeader(tt.in)
			if err != tt.wantE {
				t.Errorf("got error %v, want %v", err, tt.wantE)
			}
			if err != nil || tt.wantE != nil {
				return
			}
			if len(c) != len(tt.wantC) {
				t.Errorf("got number of challenges %d, want %d", len(c), len(tt.wantC))
			}
			for i := range tt.wantC {
				if c[i].authType != tt.wantC[i].authType {
					t.Errorf("c[%d] got authtype %s, want %s", i, c[i].authType, tt.wantC[i].authType)
				}
				for k := range tt.wantC[i].params {
					if c[i].params[k] != tt.wantC[i].params[k] {
						t.Errorf("c[%d] param %s got %s, want %s", i, k, c[i].params[k], tt.wantC[i].params[k])
					}
				}
			}
		})
	}
}
