package regclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/ref"
)

func TestReferrerList(t *testing.T) {
	// setup: copy testdata to tempdir and also start two olareg instances in memory backed by testdata, one with referrers api enabled
	ctx := context.Background()
	t.Parallel()
	testRepo := "testrepo"
	externalRepo := "external"
	testTag := "v2"
	boolT := true
	boolF := false
	tempDir := t.TempDir()
	err := copyfs.Copy(tempDir+"/"+testRepo, "./testdata/"+testRepo)
	if err != nil {
		t.Fatalf("failed to copy %s to tempDir: %v", testRepo, err)
	}
	err = copyfs.Copy(tempDir+"/"+externalRepo, "./testdata/"+externalRepo)
	if err != nil {
		t.Fatalf("failed to copy %s to tempDir: %v", externalRepo, err)
	}
	regRefHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
		API: oConfig.ConfigAPI{
			Referrer: oConfig.ConfigAPIReferrer{
				Enabled: &boolT,
			},
		},
	})
	regNoRefHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
		API: oConfig.ConfigAPI{
			Referrer: oConfig.ConfigAPIReferrer{
				Enabled: &boolF,
			},
		},
	})
	tsRef := httptest.NewServer(regRefHandler)
	tsRefURL, _ := url.Parse(tsRef.URL)
	tsRefHost := tsRefURL.Host
	tsNoRef := httptest.NewServer(regNoRefHandler)
	tsNoRefURL, _ := url.Parse(tsNoRef.URL)
	tsNoRefHost := tsNoRefURL.Host
	t.Cleanup(func() {
		tsRef.Close()
		tsNoRef.Close()
		_ = regRefHandler.Close()
		_ = regNoRefHandler.Close()
	})
	rcHosts := []config.Host{
		{
			Name:     tsRefHost,
			Hostname: tsRefHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsNoRefHost,
			Hostname: tsNoRefHost,
			TLS:      config.TLSDisabled,
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	rc := New(
		WithConfigHost(rcHosts...),
		WithSlog(log),
	)
	// top level table of three registry refs, also include a remote ref for each
	ttServers := []struct {
		name     string
		reg      string
		external string
	}{
		{
			name:     "ocidir",
			reg:      "ocidir://" + tempDir,
			external: tsRefHost,
		},
		{
			name:     "reg-with-referrer",
			reg:      tsRefHost,
			external: tsNoRefHost,
		},
		{
			name:     "reg-wo-referrer",
			reg:      tsNoRefHost,
			external: "ocidir://" + tempDir,
		},
	}
	for _, tcServer := range ttServers {
		tcServer := tcServer
		t.Run(tcServer.name, func(t *testing.T) {
			t.Parallel()
			refTag, err := ref.New(fmt.Sprintf("%s/%s:%s", tcServer.reg, testRepo, testTag))
			if err != nil {
				t.Fatalf("failed to generate refTag: %v", err)
			}
			refExt, err := ref.New(fmt.Sprintf("%s/%s", tcServer.external, externalRepo))
			if err != nil {
				t.Fatalf("failed to generate refExt: %v", err)
			}
			tt := []struct {
				name         string
				ref          ref.Ref
				opts         []scheme.ReferrerOpts
				count        int
				firstAT      string
				expectSource ref.Ref
				expectErr    error
			}{
				{
					name:  "resolve-tag",
					ref:   refTag,
					count: 2,
				},
				{
					name: "resolve-platform",
					ref:  refTag,
					opts: []scheme.ReferrerOpts{
						scheme.WithReferrerPlatform("linux/amd64"),
					},
					count:   1,
					firstAT: "application/example.arms",
				},
				{
					name: "filter",
					ref:  refTag,
					opts: []scheme.ReferrerOpts{
						scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: "application/example.signature"}),
					},
					count:   1,
					firstAT: "application/example.signature",
				},
				{
					name: "external-repo",
					ref:  refTag,
					opts: []scheme.ReferrerOpts{
						scheme.WithReferrerSource(refExt),
						scheme.WithReferrerMatchOpt(descriptor.MatchOpt{SortAnnotation: "preference"}),
					},
					count:        2,
					firstAT:      "application/example.sbom",
					expectSource: refExt,
				},
			}
			for _, tc := range tt {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					rl, err := rc.ReferrerList(ctx, tc.ref, tc.opts...)
					if tc.expectErr != nil {
						if err == nil {
							t.Fatalf("ReferrerList did not fail")
						}
						if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
							t.Fatalf("unexpected error, expected %v, received %v", tc.expectErr, err)
						}
						return
					}
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					if !ref.EqualRepository(rl.Subject, tc.ref) {
						t.Errorf("unexpected subject: expected %s, received %s", tc.ref.CommonName(), rl.Subject.CommonName())
					}
					if tc.expectSource.IsSet() && !ref.EqualRepository(rl.Source, tc.expectSource) {
						t.Errorf("unexpected source: expected %s, received %s", tc.expectSource.CommonName(), rl.Source.CommonName())
					}
					if tc.expectSource.IsZero() && !rl.Source.IsZero() {
						t.Errorf("source should not be set: received %s", rl.Source.CommonName())
					}
					if tc.count != len(rl.Descriptors) {
						t.Errorf("unexpected number of responses, expected %d, received response %v", tc.count, rl.Descriptors)
					}
					if tc.firstAT != "" && (len(rl.Descriptors) == 0 || rl.Descriptors[0].ArtifactType != tc.firstAT) {
						t.Errorf("unexpected first entry, expected %s, received response %v", tc.firstAT, rl.Descriptors)
					}
				})
			}
		})
	}
}
