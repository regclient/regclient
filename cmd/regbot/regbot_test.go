package main

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/pqueue"
	"github.com/regclient/regclient/types/ref"
)

func TestRegbot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boolT := true
	var err error
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../testdata",
		},
		API: oConfig.ConfigAPI{
			DeleteEnabled: &boolT,
		},
	})
	ts := httptest.NewServer(regHandler)
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	t.Cleanup(func() {
		ts.Close()
		_ = regHandler.Close()
	})
	rcHosts := []config.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     "registry.example.org",
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
	}
	// replace regclient with one configured for test hosts
	rc := regclient.New(
		regclient.WithConfigHost(rcHosts...),
	)
	// setup various globals normally done by loadConf
	pq := pqueue.New(pqueue.Opts[struct{}]{Max: 1})
	var confBytes = `
version: 1
defaults:
  parallel: 1
  timeout: 60s
`
	confRdr := bytes.NewReader([]byte(confBytes))
	conf, err := ConfigLoadReader(confRdr)
	if err != nil {
		t.Fatalf("failed parsing config: %v", err)
	}
	shortTime, err := time.ParseDuration("10ms")
	if err != nil {
		t.Fatalf("failed to setup shortTime: %v", err)
	}
	tests := []struct {
		name    string
		script  ConfigScript
		dryrun  bool
		exists  []string
		missing []string
		expErr  error
	}{
		{
			name: "Noop",
			script: ConfigScript{
				Name:   "Noop",
				Script: ``,
			},
			expErr: nil,
		},
		{
			name: "List",
			script: ConfigScript{
				Name:   "List",
				Script: `tag.ls "registry.example.org/testrepo"`,
			},
			expErr: nil,
		},
		{
			name: "GetConfig",
			script: ConfigScript{
				Name: "GetConfig",
				Script: `
				m = manifest.get("registry.example.org/testrepo:v1", "linux/amd64")
				ic = image.config(m)
				if ic.Config.Labels["version"] ~= "1" then
				  log("Config version: " .. ic.Config.Labels["version"])
					error "version label missing/invalid"
				end
				`,
			},
			expErr: nil,
		},
		{
			name: "CopyLatest",
			script: ConfigScript{
				Name: "CopyLatest",
				Script: `
				image.copy("registry.example.org/testrepo:v1", "registry.example.org/testcopy:latest")
				`,
			},
			exists: []string{"registry.example.org/testcopy:latest"},
		},
		{
			name: "DeleteCopy",
			script: ConfigScript{
				Name: "DeleteCopy",
				Script: `
				image.copy("registry.example.org/testrepo:v1", "registry.example.org/testdel:old")
				tag.delete("registry.example.org/testdel:old")
				`,
			},
			missing: []string{"registry.example.org/testdel:old"},
			expErr:  nil,
		},
		{
			name:   "DryRun",
			dryrun: true,
			script: ConfigScript{
				Name: "DryRun",
				Script: `
				image.copy("registry.example.org/testrepo:v1", "registry.example.org/testdryrun:latest")
				`,
			},
			missing: []string{"registry.example.org/testdryrun:latest"},
			expErr:  nil,
		},
		{
			name: "Timeout",
			script: ConfigScript{
				Name: "Timeout",
				Script: `
				while true do
					tag.ls "registry.example.org/testrepo"
				end
				`,
				Timeout: shortTime,
			},
			expErr: ErrScriptFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootOpts := rootCmd{
				dryRun: tt.dryrun,
				conf:   conf,
				log: &logrus.Logger{
					Out:       os.Stderr,
					Formatter: new(logrus.TextFormatter),
					Hooks:     make(logrus.LevelHooks),
					Level:     logrus.InfoLevel,
				},
				rc:       rc,
				throttle: pq,
			}
			err = rootOpts.process(ctx, tt.script)
			if tt.expErr != nil {
				if err == nil {
					t.Errorf("process did not fail")
				} else if !errors.Is(err, tt.expErr) && err.Error() != tt.expErr.Error() {
					t.Errorf("unexpected error on process: %v, expected %v", err, tt.expErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error on process: %v", err)
			}
			for _, exist := range tt.exists {
				r, err := ref.New(exist)
				if err != nil {
					t.Errorf("cannot parse ref %s: %v", exist, err)
					continue
				}
				_, err = rc.ManifestHead(ctx, r)
				if err != nil {
					t.Errorf("ref does not exist: %s", exist)
				}
			}
			for _, missing := range tt.missing {
				r, err := ref.New(missing)
				if err != nil {
					t.Fatalf("cannot parse ref %s: %v", missing, err)
				}
				_, err = rc.ManifestHead(ctx, r)
				if err == nil {
					t.Errorf("ref exists: %s", missing)
				}
			}
		})
	}
}
