package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
	"golang.org/x/sync/semaphore"
)

func TestRegbot(t *testing.T) {
	ctx := context.Background()
	// setup sample source with an in-memory ocidir directory
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	// setup various globals normally done by loadConf
	sem = semaphore.NewWeighted(1)
	rc = regclient.New(regclient.WithFS(fsMem))
	var confBytes = `
  version: 1
  defaults:
    parallel: 1
    timeout: 60s
  `
	confRdr := bytes.NewReader([]byte(confBytes))
	conf, err = ConfigLoadReader(confRdr)
	if err != nil {
		t.Errorf("failed parsing config: %v", err)
		return
	}
	shortTime, err := time.ParseDuration("10ms")
	if err != nil {
		t.Errorf("failed to setup shortTime: %v", err)
		return
	}
	tests := []struct {
		name      string
		script    ConfigScript
		dryrun    bool
		exists    []string
		missing   []string
		desired   []string
		undesired []string
		expErr    error
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
				Script: `tag.ls "ocidir://testrepo"`,
			},
			expErr: nil,
		},
		{
			name: "GetConfig",
			script: ConfigScript{
				Name: "GetConfig",
				Script: `
				m = manifest.get("ocidir://testrepo:v1", "linux/amd64")
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
				image.copy("ocidir://testrepo:v1", "ocidir://testcopy:latest")
				`,
			},
			exists: []string{"ocidir://testcopy:latest"},
			desired: []string{
				"testcopy/index.json",
				"testcopy/oci-layout",
				"testcopy/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
			},
			expErr: nil,
		},
		{
			name: "DeleteCopy",
			script: ConfigScript{
				Name: "DeleteCopy",
				Script: `
				image.copy("ocidir://testrepo:v1", "ocidir://testdel:old")
				tag.delete("ocidir://testdel:old")
				`,
			},
			// exists: []string{"ocidir://testcopy:latest"},
			missing: []string{"ocidir://testdel:old"},
			desired: []string{
				"testcopy/index.json",
				"testcopy/oci-layout",
			},
			undesired: []string{
				"testdel/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
			},
			expErr: nil,
		},
		{
			name:   "DryRun",
			dryrun: true,
			script: ConfigScript{
				Name: "DryRun",
				Script: `
				image.copy("ocidir://testrepo:v1", "ocidir://testdryrun:latest")
				`,
			},
			// exists: []string{"ocidir://testcopy:latest"},
			missing: []string{"ocidir://testdryrun:latest"},
			undesired: []string{
				"testdryrun/index.json",
				"testdryrun/oci-layout",
				"testdryrun/blobs/sha256/94ec59b4c55eb2341b63ea9a0abab63590a923e7cb5cd682217ca209ef362694", // v1
			},
			expErr: nil,
		},
		{
			name: "Timeout",
			script: ConfigScript{
				Name: "Timeout",
				Script: `
				while true do
					tag.ls "ocidir://testrepo"
				end
				`,
				Timeout: shortTime,
			},
			expErr: ErrScriptFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootOpts.dryRun = tt.dryrun
			err = tt.script.process(ctx)
			if tt.expErr != nil {
				if err == nil {
					t.Errorf("process did not fail")
				} else if !errors.Is(err, tt.expErr) && err.Error() != tt.expErr.Error() {
					t.Errorf("unexpected error on process: %v, expected %v", err, tt.expErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error on process: %v", err)
				return
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
					t.Errorf("cannot parse ref %s: %v", missing, err)
					continue
				}
				_, err = rc.ManifestHead(ctx, r)
				if err == nil {
					t.Errorf("ref exists: %s", missing)
				}
			}
			for _, file := range tt.desired {
				_, err = rwfs.Stat(fsMem, file)
				if err != nil {
					t.Errorf("missing file in sync: %s", file)
				}
			}
			for _, file := range tt.undesired {
				_, err = rwfs.Stat(fsMem, file)
				if err == nil {
					t.Errorf("undesired file after sync: %s", file)
				}
			}
		})
	}
}
