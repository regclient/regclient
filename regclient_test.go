package regclient

import (
	"testing"

	"github.com/regclient/regclient/scheme/reg"
	"github.com/sirupsen/logrus"
)

func TestNew(t *testing.T) {
	logPtr := logrus.New()
	tt := []struct {
		name   string
		opts   []Opt
		expect RegClient
	}{
		{
			name:   "default",
			opts:   []Opt{},
			expect: RegClient{},
		},
		{
			name: "regOpt",
			opts: []Opt{
				WithRegOpts(
					reg.WithBlobLimit(1234),
					reg.WithBlobSize(64, 128),
				),
			},
			expect: RegClient{
				regOpts: []reg.Opts{
					reg.WithBlobLimit(1234),
					reg.WithBlobSize(64, 128),
				},
			},
		},
		{
			name: "regOpt separate",
			opts: []Opt{
				WithRegOpts(reg.WithBlobLimit(1234)),
				WithRegOpts(reg.WithBlobSize(64, 128)),
			},
			expect: RegClient{
				regOpts: []reg.Opts{
					reg.WithBlobLimit(1234),
					reg.WithBlobSize(64, 128),
				},
			},
		},
		{
			name: "log",
			opts: []Opt{
				WithLog(logPtr),
			},
			expect: RegClient{
				log: logPtr,
			},
		},
		{
			name: "userAgent",
			opts: []Opt{
				WithUserAgent("unit-test"),
			},
			expect: RegClient{
				userAgent: "unit-test",
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := New(tc.opts...)
			if tc.expect.hosts != nil {
				if result.hosts == nil {
					t.Errorf("host is nil")
				} else {
					for name := range tc.expect.hosts {
						if _, ok := result.hosts[name]; !ok {
							t.Errorf("host entry missing for %s", name)
						}
					}
				}
			}
			if tc.expect.log != nil {
				if result.log == nil {
					t.Errorf("log is nil")
				} else if result.log != tc.expect.log {
					t.Errorf("log pointer mismatch")
				}
			}
			if len(tc.expect.regOpts) > 0 {
				if len(tc.expect.regOpts)+3 != len(result.regOpts) {
					t.Errorf("regOpts length mismatch, expected %d, received %d", len(tc.expect.regOpts), len(result.regOpts))
				}
				// TODO: can content of each regOpt be compared?
			}
			if tc.expect.userAgent != "" && tc.expect.userAgent != result.userAgent {
				t.Errorf("userAgent, expected %s, received %s", tc.expect.userAgent, result.userAgent)
			}
		})
	}

}
