package main

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

func TestImageCheckBase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "../../testdata",
		},
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
				Name:     "registry.example.org",
				Hostname: tsHost,
				TLS:      config.TLSDisabled,
			},
		),
	}

	rb, err := ref.New("ocidir://../../testdata/testrepo:b3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rc := regclient.New(rcOpts...)
	mb, err := rc.ManifestHead(ctx, rb, regclient.WithManifestRequireDigest())
	if err != nil {
		t.Fatalf("failed to head ref %s: %v", rb.CommonName(), err)
	}
	dig := mb.GetDescriptor().Digest

	tt := []struct {
		name      string
		args      []string
		expectErr error
		expectOut string
	}{
		{
			name:      "missing annotation",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v1"},
			expectErr: errs.ErrMissingAnnotation,
		},
		{
			name:      "annotation v2",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v2"},
			expectErr: errs.ErrMismatch,
			expectOut: "base image has changed",
		},
		{
			name:      "annotation v3",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v3"},
			expectErr: errs.ErrMismatch,
			expectOut: "base image has changed",
		},
		{
			name:      "manual v2 b1",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v2", "--base", tsHost + "/testrepo:b1"},
			expectOut: "base image matches",
		},
		{
			name:      "manual v2 b2",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v2", "--base", tsHost + "/testrepo:b2"},
			expectErr: errs.ErrMismatch,
			expectOut: "base image has changed",
		},
		{
			name:      "manual v3 b1",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v3", "--base", tsHost + "/testrepo:b1"},
			expectOut: "base image matches",
		},
		{
			name:      "manual v3 b3",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v3", "--base", tsHost + "/testrepo:b3"},
			expectErr: errs.ErrMismatch,
			expectOut: "base image has changed",
		},
		{
			name:      "manual v3 b3 with digest",
			args:      []string{"image", "check-base", tsHost + "/testrepo:v3", "--base", tsHost + "/testrepo:b3", "--digest", dig.String()},
			expectOut: "base image matches",
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
			if out != tc.expectOut {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}

func TestImageCopy(t *testing.T) {
	tempDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v2"
	boolT := true
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
	t.Setenv(ConfigEnv, filepath.Join(tempDir, "config.json"))
	_, err := cobraTest(t, nil, "registry", "set", tsHost, "--tls", "disabled")
	if err != nil {
		t.Fatalf("failed to disable TLS for internal registry")
	}
	tt := []struct {
		name        string
		args        []string
		expectErr   error
		expectOut   string
		outContains bool
	}{
		{
			name:      "ocidir-to-ocidir",
			args:      []string{"image", "copy", srcRef, "ocidir://" + tempDir + "testrepo:v2"},
			expectOut: "ocidir://" + tempDir + "testrepo:v2",
		},
		{
			name:      "ocidir-to-reg",
			args:      []string{"image", "copy", srcRef, tsHost + "/newrepo:v2"},
			expectOut: tsHost + "/newrepo:v2",
		},
		{
			name:      "reg-to-reg-platform",
			args:      []string{"image", "copy", "--platform", "linux/amd64", tsHost + "/testrepo:v3", tsHost + "/newrepo:v3"},
			expectOut: tsHost + "/newrepo:v3",
		},
		{
			name:      "ocidir-to-reg-external-referrers",
			args:      []string{"image", "copy", srcRef, tsHost + "/newrepo:v4", "--referrers", "--referrers-src", "ocidir://../../testdata/external", "--referrers-tgt", tsHost + "/external"},
			expectOut: tsHost + "/newrepo:v4",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.args...)
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
		})
	}
}

func TestImageCreate(t *testing.T) {
	tmpDir := t.TempDir()
	imageRef := fmt.Sprintf("ocidir://%s/repo:scratch", tmpDir)

	out, err := cobraTest(t, nil, "image", "create", imageRef)
	if err != nil {
		t.Fatalf("failed to run image create: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}
}

func TestImageExportImport(t *testing.T) {
	tmpDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v2"
	exportFile := tmpDir + "/export.tar"
	exportName := "registry.example.com/repo:v2"
	importRefA := fmt.Sprintf("ocidir://%s/repo:v2", tmpDir)

	out, err := cobraTest(t, nil, "image", "export", "--name", exportName, srcRef, exportFile)
	if err != nil {
		t.Fatalf("failed to run image export: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}

	out, err = cobraTest(t, nil, "image", "import", importRefA, exportFile)
	if err != nil {
		t.Fatalf("failed to run image import: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}

	out, err = cobraTest(t, nil, "image", "export", "--name", exportName, "--platform", "linux/amd64", srcRef, exportFile)
	if err != nil {
		t.Fatalf("failed to run image export: %v", err)
	}
	if out != "" {
		t.Errorf("unexpected output: %v", out)
	}
}

func TestImageInspect(t *testing.T) {
	srcRef := "ocidir://../../testdata/testrepo:v3"
	tt := []struct {
		name        string
		cmd         []string
		expectOut   string
		expectErr   error
		outContains bool
	}{
		{
			name:        "default",
			cmd:         []string{"image", "inspect", srcRef},
			expectOut:   "created",
			outContains: true,
		},
		{
			name:        "format body",
			cmd:         []string{"image", "inspect", srcRef, "--format", `body`},
			expectOut:   "created",
			outContains: true,
		},
		{
			name:        "format raw",
			cmd:         []string{"image", "inspect", srcRef, "--format", `raw`},
			expectOut:   "created",
			outContains: true,
		},
		{
			name:        "format headers",
			cmd:         []string{"image", "inspect", srcRef, "--format", `headers`},
			expectOut:   "",
			outContains: false,
		},
		{
			name:        "format config",
			cmd:         []string{"image", "inspect", srcRef, "--platform", "linux/amd64", "--format", `{{ index .Config.Labels "version" }}`},
			expectOut:   "3",
			outContains: false,
		},
		{
			name:        "format getconfig",
			cmd:         []string{"image", "inspect", srcRef, "--platform", "linux/arm64", "--format", `{{ .GetConfig.OS}}`},
			expectOut:   "linux",
			outContains: false,
		},
		{
			name:      "invalid ref",
			cmd:       []string{"image", "inspect", "invalid://ref*format"},
			expectErr: errs.ErrInvalidReference,
		},
		{
			name:      "unsupported artifact",
			cmd:       []string{"image", "inspect", "ocidir://../../testdata/testrepo:a1"},
			expectErr: errs.ErrUnsupportedMediaType,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.cmd...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("command did not fail")
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}

func TestImageMod(t *testing.T) {
	tmpDir := t.TempDir()
	srcRef := "ocidir://../../testdata/testrepo:v3"
	baseRef := "ocidir://../../testdata/testrepo:b1"
	modRef := fmt.Sprintf("ocidir://%s/repo:mod", tmpDir)
	tt := []struct {
		name        string
		cmd         []string
		expectOut   string
		outContains bool
		expectErr   error
	}{
		{
			name:      "layer-add-tar",
			cmd:       []string{"image", "mod", srcRef, "--create", modRef, "--layer-add", "tar=../../testdata/layer.tar,platform=linux/amd64"},
			expectOut: modRef,
		},
		{
			name:      "layer-add-dir",
			cmd:       []string{"image", "mod", srcRef, "--create", modRef, "--layer-add", "dir=../../cmd"},
			expectOut: modRef,
		},
		{
			name:      "layer-add-dir-workdir",
			cmd:       []string{"image", "mod", srcRef, "--create", modRef, "--layer-add", "dir=../../cmd,workdir=/tmp"},
			expectOut: modRef,
		},
		{
			name:      "layer-add-both",
			cmd:       []string{"image", "mod", srcRef, "--create", modRef, "--layer-add", "tar=../../testdata/layer.tar,dir=../../cmd,platform=linux/amd64"},
			expectErr: fmt.Errorf(`invalid argument "tar=../../testdata/layer.tar,dir=../../cmd,platform=linux/amd64" for "--layer-add" flag: cannot use dir and tar options together in layer-add`),
		},
		{
			name:      "timestamps",
			cmd:       []string{"image", "mod", srcRef, "--create", modRef, "--time", "set=2000-01-01T00:00:00Z,base-ref=" + baseRef},
			expectOut: modRef,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.cmd...)
			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("command did not fail with expected error: %v", tc.expectErr)
				}
				if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Fatalf("command failed with unexpected error, expected %v, received %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("command failed with error: %v", err)
			}
			if (!tc.outContains && out != tc.expectOut) || (tc.outContains && !strings.Contains(out, tc.expectOut)) {
				t.Errorf("unexpected output, expected %s, received %s", tc.expectOut, out)
			}
		})
	}
}
