package regclient

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/types/ref"
)

func TestTag(t *testing.T) {
	t.Parallel()
	existingRepo := "testrepo"
	existingTag := "v2"
	ctx := context.Background()
	boolT := true
	boolF := false
	regRWHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
		API: oConfig.ConfigAPI{
			DeleteEnabled: &boolT,
		},
	})
	regROHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
		API: oConfig.ConfigAPI{
			DeleteEnabled: &boolF,
		},
	})
	tsRW := httptest.NewServer(regRWHandler)
	tsRWURL, _ := url.Parse(tsRW.URL)
	tsRWHost := tsRWURL.Host
	tsRO := httptest.NewServer(regROHandler)
	tsROURL, _ := url.Parse(tsRO.URL)
	tsROHost := tsROURL.Host
	t.Cleanup(func() {
		tsRW.Close()
		tsRO.Close()
		_ = regRWHandler.Close()
		_ = regROHandler.Close()
	})
	rcHosts := []config.Host{
		{
			Name:     tsRWHost,
			Hostname: tsRWHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsROHost,
			Hostname: tsROHost,
			TLS:      config.TLSDisabled,
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := New(
		WithConfigHost(rcHosts...),
		WithSlog(log),
		WithRetryDelay(delayInit, delayMax),
	)
	tempDir := t.TempDir()
	err := copyfs.Copy(tempDir+"/"+existingRepo, "./testdata/"+existingRepo)
	if err != nil {
		t.Fatalf("failed to copy %s to tempDir: %v", existingRepo, err)
	}
	tt := []struct {
		name           string
		repo           string
		deleteDisabled bool
	}{
		{
			name: "reg RW",
			repo: tsRWHost + "/" + existingRepo,
		},
		{
			name:           "reg RO",
			repo:           tsROHost + "/" + existingRepo,
			deleteDisabled: true,
		},
		{
			name: "ocidir",
			repo: "ocidir://" + tempDir + "/" + existingRepo,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ref.New(tc.repo)
			if err != nil {
				t.Fatalf("failed to parse ref %s: %v", tc.repo, err)
			}
			tl, err := rc.TagList(ctx, r)
			if err != nil {
				t.Fatalf("failed to list tags: %v", err)
			}
			if len(tl.Tags) == 0 {
				t.Fatalf("failed to get tags: %v", tl)
			}
			rDel, err := ref.New(tc.repo + ":" + existingTag)
			if err != nil {
				t.Fatalf("failed to parse ref %s: %v", tc.repo+":"+existingTag, err)
			}
			err = rc.TagDelete(ctx, rDel)
			if tc.deleteDisabled {
				if err == nil {
					t.Errorf("delete succeeded on a read-only repo")
				}
			} else {
				if err != nil {
					t.Errorf("failed to delete tag: %v", err)
				}
			}
		})
	}
}
