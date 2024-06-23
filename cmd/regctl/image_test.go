package main

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"
)

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
			name:        "format config",
			cmd:         []string{"image", "inspect", srcRef, "--format", `{{ index .Config.Labels "version" }}`},
			expectOut:   "3",
			outContains: false,
		},
		{
			name:        "format getconfig",
			cmd:         []string{"image", "inspect", srcRef, "--format", `{{ .GetConfig.OS}}`},
			expectOut:   "linux",
			outContains: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cobraTest(t, nil, tc.cmd...)
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
