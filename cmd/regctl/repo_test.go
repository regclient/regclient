package main

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/ref"
)

func TestRepoCopy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../testdata",
		},
		API: oConfig.ConfigAPI{},
	})
	ts := httptest.NewServer(regHandler)
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	t.Cleanup(func() {
		ts.Close()
		_ = regHandler.Close()
	})
	rcOpts := []regclient.Opt{
		regclient.WithConfigHost(
			config.Host{
				Name: tsHost,
				TLS:  config.TLSDisabled,
			},
			config.Host{
				Name:     "invalid-tls." + tsHost,
				Hostname: tsHost,
				TLS:      config.TLSEnabled,
			},
		),
		regclient.WithRegOpts(reg.WithDelay(time.Millisecond*10, time.Millisecond*100), reg.WithRetryLimit(2)),
	}
	rc := regclient.New(rcOpts...)

	tt := []struct {
		name              string
		args              []string
		expectErr         error
		expectOut         string
		outContains       bool
		expectRepo        string
		expectTags        []string
		expectNoTag       []string
		expectReferrers   []string
		expectNoReferrers []string
	}{
		{
			name:      "Missing arg",
			args:      []string{"repo", "copy"},
			expectErr: fmt.Errorf("accepts 2 arg(s), received 0"),
		},
		{
			name:       "Copy testrepo to full",
			args:       []string{"repo", "copy", tsHost + "/testrepo", tsHost + "/full"},
			expectRepo: tsHost + "/full",
			expectTags: []string{"a1", "a2", "a3", "ai", "child", "loop", "mirror", "v1", "v2", "v3"},
			// note, referrers on v2 are included via the a* tagged images
		},
		{
			name:              "Copy testrepo to full-exc without sha and artifact tags",
			args:              []string{"repo", "copy", "--exclude", "sha.*", "--exclude", "a.*", tsHost + "/testrepo", tsHost + "/full-exc"},
			expectRepo:        tsHost + "/full-exc",
			expectTags:        []string{"child", "loop", "mirror", "v1", "v2", "v3"},
			expectNoTag:       []string{"a1", "a2", "a3", "ai"},
			expectNoReferrers: []string{"v2"},
		},
		{
			name:            "Copy testrepo to full-referrers with referrers",
			args:            []string{"repo", "copy", "--referrers", tsHost + "/testrepo", tsHost + "/full-referrers"},
			expectRepo:      tsHost + "/full-referrers",
			expectTags:      []string{"a1", "a2", "a3", "ai", "child", "loop", "mirror", "v1", "v2", "v3"},
			expectReferrers: []string{"v2"},
		},
		{
			name:              "Copy testrepo to vx without referrers",
			args:              []string{"repo", "copy", "--include", "v.*", tsHost + "/testrepo", tsHost + "/vx"},
			expectRepo:        tsHost + "/vx",
			expectTags:        []string{"v1", "v2", "v3"},
			expectNoTag:       []string{"a1", "a2", "a3", "ai", "child", "loop", "mirror"},
			expectNoReferrers: []string{"v2"},
		},
		{
			name:            "Copy testrepo to vx-ref with referrers",
			args:            []string{"repo", "copy", "--include", "v.*", "--referrers", tsHost + "/testrepo", tsHost + "/vx-ref"},
			expectRepo:      tsHost + "/vx-ref",
			expectTags:      []string{"v1", "v2", "v3"},
			expectNoTag:     []string{"a1", "a2", "a3", "ai", "child", "loop", "mirror"},
			expectReferrers: []string{"v2"},
		},
		{
			name:       "Copy testrepo to concurrent without throttle",
			args:       []string{"repo", "copy", "--concurrent", "-1", tsHost + "/testrepo", tsHost + "/concurrent"},
			expectRepo: tsHost + "/concurrent",
			expectTags: []string{"a1", "a2", "a3", "ai", "child", "loop", "mirror", "v1", "v2", "v3"},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, &cobraTestOpts{rcOpts: rcOpts}, tc.args...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("did not receive expected error: %v", tc.expectErr)
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, received %v, expected %v", err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("returned unexpected error: %v", err)
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
			if tc.expectRepo != "" {
				r, err := ref.New(tc.expectRepo)
				if err != nil {
					t.Fatalf("failed to parse expected repo: %s: %v", tc.expectRepo, err)
				}
				tl, err := rc.TagList(ctx, r)
				if err != nil {
					t.Fatalf("failed to list tags for %s: %v", tc.expectRepo, err)
				}
				if tl == nil || tl.Tags == nil {
					t.Fatalf("tag list is nil: %v", tl)
				}
				for _, tag := range tc.expectTags {
					if !slices.Contains(tl.Tags, tag) {
						t.Errorf("did not find tag %s", tag)
					}
				}
				for _, tag := range tc.expectNoTag {
					if slices.Contains(tl.Tags, tag) {
						t.Errorf("found tag that should not be copied %s", tag)
					}
				}
				for _, tag := range tc.expectReferrers {
					r := r.SetTag(tag)
					rl, err := rc.ReferrerList(ctx, r)
					if err != nil {
						t.Errorf("failed to list referrers for %s: %v", tag, err)
					} else if len(rl.Descriptors) == 0 {
						t.Errorf("referrers list is empty for %s", tag)
					}
				}
				for _, tag := range tc.expectNoReferrers {
					r := r.SetTag(tag)
					rl, err := rc.ReferrerList(ctx, r)
					if err != nil {
						t.Errorf("failed to list referrers for %s: %v", tag, err)
					} else if len(rl.Descriptors) > 0 {
						t.Errorf("referrers list is not empty for %s", tag)
					}
				}
			}
		})
	}
}
