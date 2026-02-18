package regclient

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/olareg/olareg"
	oConfig "github.com/olareg/olareg/config"

	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

func TestImageCheckBase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
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
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := New(
		WithConfigHost(rcHosts...),
		WithSlog(log),
		WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)
	rb1, err := ref.New(tsHost + "/testrepo:b1")
	if err != nil {
		t.Fatalf("failed to setup ref: %v", err)
	}
	rb2, err := ref.New(tsHost + "/testrepo:b2")
	if err != nil {
		t.Fatalf("failed to setup ref: %v", err)
	}
	rb3, err := ref.New(tsHost + "/testrepo:b3")
	if err != nil {
		t.Fatalf("failed to setup ref: %v", err)
	}
	m3, err := rc.ManifestHead(ctx, rb3)
	if err != nil {
		t.Fatalf("failed to get digest for base3: %v", err)
	}
	dig3 := m3.GetDescriptor().Digest
	r1, err := ref.New(tsHost + "/testrepo:v1")
	if err != nil {
		t.Fatalf("failed to setup ref: %v", err)
	}
	r2, err := ref.New(tsHost + "/testrepo:v2")
	if err != nil {
		t.Fatalf("failed to setup ref: %v", err)
	}
	r3, err := ref.New(tsHost + "/testrepo:v3")
	if err != nil {
		t.Fatalf("failed to setup ref: %v", err)
	}

	tt := []struct {
		name      string
		opts      []ImageOpts
		r         ref.Ref
		expectErr error
	}{
		{
			name:      "missing annotation",
			r:         r1,
			expectErr: errs.ErrMissingAnnotation,
		},
		{
			name:      "annotation v2",
			r:         r2,
			expectErr: errs.ErrMismatch,
		},
		{
			name:      "annotation v3",
			r:         r3,
			expectErr: errs.ErrMismatch,
		},
		{
			name: "manual v2, b1",
			r:    r2,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb1.CommonName())},
		},
		{
			name:      "manual v2, b2",
			r:         r2,
			opts:      []ImageOpts{ImageWithCheckBaseRef(rb2.CommonName())},
			expectErr: errs.ErrMismatch,
		},
		{
			name:      "manual v2, b3",
			r:         r2,
			opts:      []ImageOpts{ImageWithCheckBaseRef(rb3.CommonName())},
			expectErr: errs.ErrMismatch,
		},
		{
			name: "manual v3, b1",
			r:    r3,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb1.CommonName())},
		},
		{
			name: "manual v3, b3 with digest",
			r:    r3,
			opts: []ImageOpts{ImageWithCheckBaseRef(rb3.CommonName()), ImageWithCheckBaseDigest(dig3.String())},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := rc.ImageCheckBase(ctx, tc.r, tc.opts...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("check base did not fail")
				} else if err.Error() != tc.expectErr.Error() && !errors.Is(err, tc.expectErr) {
					t.Errorf("error mismatch, expected %v, received %v", tc.expectErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("check base failed")
				}
			}
		})
	}
}

func TestImageConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
	})
	ts := httptest.NewServer(regHandler)
	t.Cleanup(func() {
		ts.Close()
		_ = regHandler.Close()
	})
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	rcHosts := []config.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := New(
		WithConfigHost(rcHosts...),
		WithSlog(log),
		WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)
	tt := []struct {
		name       string
		r          string
		opts       []ImageOpts
		expectErr  error
		expectArch string
		expectOS   string
	}{
		{
			name:       "ocidir-v1-amd64",
			r:          "ocidir://testdata/testrepo:v1",
			opts:       []ImageOpts{ImageWithPlatform("linux/amd64")},
			expectArch: "amd64",
			expectOS:   "linux",
		},
		{
			name:      "ocidir-not-found",
			r:         "ocidir://testdata/testrepo:missing",
			expectErr: errs.ErrNotFound,
		},
		{
			name:      "ocidir-a1",
			r:         "ocidir://testdata/testrepo:a1",
			opts:      []ImageOpts{},
			expectErr: errs.ErrUnsupportedMediaType,
		},
		{
			name:       "reg-v2-arm64",
			r:          tsHost + "/testrepo:v2",
			opts:       []ImageOpts{ImageWithPlatform("linux/arm64")},
			expectArch: "arm64",
			expectOS:   "linux",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ref.New(tc.r)
			if err != nil {
				t.Fatalf("failed to parse ref: %v", err)
			}
			bConf, err := rc.ImageConfig(ctx, r, tc.opts...)
			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("method did not fail")
				}
				if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("method failed: %v", err)
			}
			c := bConf.GetConfig()
			if (tc.expectOS != "" && tc.expectOS != c.OS) || (tc.expectArch != "" && tc.expectArch != c.Architecture) {
				t.Errorf("unexpected config, expected %s/%s, received %s/%s", tc.expectOS, tc.expectArch, c.OS, c.Architecture)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boolT := true
	copyHookErr := errors.New("copy hook failure")
	regHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
		},
	})
	regROHandler := olareg.New(oConfig.Config{
		Storage: oConfig.ConfigStorage{
			StoreType: oConfig.StoreMem,
			RootDir:   "./testdata",
			ReadOnly:  &boolT,
		},
	})
	ts := httptest.NewServer(regHandler)
	tsRO := httptest.NewServer(regROHandler)
	t.Cleanup(func() {
		ts.Close()
		_ = regHandler.Close()
		tsRO.Close()
		_ = regROHandler.Close()
	})
	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Host
	tsROURL, _ := url.Parse(tsRO.URL)
	tsROHost := tsROURL.Host
	rcHosts := []config.Host{
		{
			Name:     tsHost,
			Hostname: tsHost,
			TLS:      config.TLSDisabled,
		},
		{
			Name:     tsROHost,
			Hostname: tsROHost,
			TLS:      config.TLSDisabled,
		},
	}
	rReferrerSrc, err := ref.New("ocidir://./testdata/external")
	if err != nil {
		t.Fatalf("failed to parse referrer src repo: %v", err)
	}
	rReferrerTgt, err := ref.New(tsHost + "/dest-external")
	if err != nil {
		t.Fatalf("failed to parse referrer tgt repo: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := New(
		WithConfigHost(rcHosts...),
		WithSlog(log),
		WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)
	tempDir := t.TempDir()
	tt := []struct {
		name      string
		src, tgt  string
		opts      []ImageOpts
		expectErr error
	}{
		{
			name: "ocidir to registry",
			src:  "ocidir://./testdata/testrepo:v1",
			tgt:  tsHost + "/dest-ocidir:v1",
		},
		{
			name:      "ocidir to read-only registry",
			src:       "ocidir://./testdata/testrepo:v1",
			tgt:       tsROHost + "/dest-ocidir:v1",
			expectErr: errs.ErrHTTPStatus,
		},
		{
			name: "ocidir to ocidir",
			src:  "ocidir://./testdata/testrepo:v1",
			tgt:  "ocidir://" + tempDir + "/testrepo:v1",
		},
		{
			name: "registry to registry",
			src:  tsHost + "/testrepo:v1",
			tgt:  tsHost + "/dest-reg:v1",
		},
		{
			name: "registry to registry same repo",
			src:  tsHost + "/testrepo:v1",
			tgt:  tsHost + "/testrepo:v1-copy",
		},
		{
			name: "ocidir to registry with referrers and digest tags",
			src:  "ocidir://./testdata/testrepo:v2",
			tgt:  tsHost + "/dest-ocidir:v2",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags()},
		},
		{
			name: "ocidir to ocidir with referrers and digest tags",
			src:  "ocidir://./testdata/testrepo:v2",
			tgt:  "ocidir://" + tempDir + "/testrepo:v2",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags()},
		},
		{
			name: "registry to registry with referrers and digest tags",
			src:  tsHost + "/testrepo:v2",
			tgt:  tsHost + "/dest-reg:v2",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags()},
		},
		{
			name: "ocidir to registry with external referrers and digest tags",
			src:  "ocidir://./testdata/testrepo:v2",
			tgt:  tsHost + "/dest-ocidir:v2",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags(), ImageWithReferrerSrc(rReferrerSrc), ImageWithReferrerTgt(rReferrerTgt)},
		},
		{
			name: "ocidir to registry with fast check",
			src:  "ocidir://./testdata/testrepo:v3",
			tgt:  tsHost + "/testrepo:v3-copy",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags(), ImageWithFastCheck()},
		},
		{
			name: "ocidir to registry child/loop",
			src:  "ocidir://./testdata/testrepo:child",
			tgt:  tsHost + "/dest-ocidir:child",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags()},
		},
		{
			name: "ocidir to ocidir child/loop",
			src:  "ocidir://./testdata/testrepo:child",
			tgt:  "ocidir://" + tempDir + "/testrepo:child",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags()},
		},
		{
			name: "registry to registry child/loop",
			src:  tsHost + "/testrepo:child",
			tgt:  tsHost + "/dest-reg:child",
			opts: []ImageOpts{ImageWithReferrers(), ImageWithDigestTags()},
		},
		{
			name: "ocidir to registry mirror digest tag",
			src:  "ocidir://./testdata/testrepo:mirror",
			tgt:  tsHost + "/dest-ocidir:mirror",
			opts: []ImageOpts{ImageWithDigestTags()},
		},
		{
			name: "ocidir to ocidir mirror digest tag",
			src:  "ocidir://./testdata/testrepo:mirror",
			tgt:  "ocidir://" + tempDir + "/testrepo:mirror",
			opts: []ImageOpts{ImageWithDigestTags()},
		},
		{
			name: "ocidir to ocidir with blob reader hook",
			src:  "ocidir://./testdata/testrepo:v1",
			tgt:  "ocidir://" + tempDir + "/testrepo:v4",
			opts: []ImageOpts{ImageWithBlobReaderHook(func(b *blob.BReader) (*blob.BReader, error) {
				desc := b.GetDescriptor()
				blobBody, err := io.ReadAll(b)
				if err != nil {
					return nil, err
				}
				calcDig := desc.Digest.Algorithm().FromBytes(blobBody)
				if desc.Digest != calcDig {
					return nil, fmt.Errorf("unexpected digest, expected %s, calculated %s", desc.Digest, calcDig)
				}
				return blob.NewReader(
					blob.WithHeader(b.RawHeaders()),
					blob.WithDesc(desc),
					blob.WithReader(bytes.NewReader(blobBody)),
				), nil
			})},
		},
		{
			name: "ocidir to ocidir with blob reader hook error",
			src:  "ocidir://./testdata/testrepo:v2",
			tgt:  "ocidir://" + tempDir + "/testrepo:v50",
			opts: []ImageOpts{ImageWithBlobReaderHook(func(b *blob.BReader) (*blob.BReader, error) {
				return nil, copyHookErr
			})},
			expectErr: copyHookErr,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rSrc, err := ref.New(tc.src)
			if err != nil {
				t.Fatalf("failed to parse ref %s: %v", tc.src, err)
			}
			rTgt, err := ref.New(tc.tgt)
			if err != nil {
				t.Fatalf("failed to parse ref %s: %v", tc.tgt, err)
			}
			err = rc.ImageCopy(ctx, rSrc, rTgt, tc.opts...)
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("copy did not fail, expected %v", tc.expectErr)
				} else if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("copy failed: %v", err)
			}
		})
	}
}

func TestExportImport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// copy testdata images into tempdir
	tempDir := t.TempDir()
	err := copyfs.Copy(tempDir+"/testrepo", "testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to copyfs to tempdir: %v", err)
	}
	// create regclient
	rc := New()
	rIn1, err := ref.New("ocidir://" + tempDir + "/testrepo:v1")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rOut1, err := ref.New("ocidir://" + tempDir + "/testout:v1")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rIn3, err := ref.New("ocidir://" + tempDir + "/testrepo:v3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}
	rOut3, err := ref.New("ocidir://" + tempDir + "/testout:v3")
	if err != nil {
		t.Fatalf("failed to parse ref: %v", err)
	}

	// export repo to tar
	fileOut1, err := os.Create(filepath.Join(tempDir, "test1.tar"))
	if err != nil {
		t.Fatalf("failed to create output tar: %v", err)
	}
	err = rc.ImageExport(ctx, rIn1, fileOut1)
	fileOut1.Close()
	if err != nil {
		t.Errorf("failed to export: %v", err)
	}
	fileOut3, err := os.Create(filepath.Join(tempDir, "test3.tar.gz"))
	if err != nil {
		t.Fatalf("failed to create output tar: %v", err)
	}
	err = rc.ImageExport(ctx, rIn3, fileOut3, ImageWithExportCompress())
	fileOut3.Close()
	if err != nil {
		t.Errorf("failed to export: %v", err)
	}

	// modify tar for tests
	fileR, err := os.Open(filepath.Join(tempDir, "test1.tar"))
	if err != nil {
		t.Fatalf("failed to open tar: %v", err)
	}
	fileW, err := os.Create(filepath.Join(tempDir, "test2.tar"))
	if err != nil {
		t.Errorf("failed to create tar: %v", err)
	}
	tr := tar.NewReader(fileR)
	tw := tar.NewWriter(fileW)
	for {
		th, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Errorf("failed to read tar header: %v", err)
		}
		th.Name = "./" + th.Name
		err = tw.WriteHeader(th)
		if err != nil {
			t.Errorf("failed to write tar header: %v", err)
		}
		if th.Size > 0 {
			_, err = io.Copy(tw, tr)
			if err != nil {
				t.Errorf("failed to copy tar file contents %s: %v", th.Name, err)
			}
		}
	}
	fileR.Close()
	fileW.Close()

	// import tar to repo
	fileIn2, err := os.Open(filepath.Join(tempDir, "test2.tar"))
	if err != nil {
		t.Fatalf("failed to open tar: %v", err)
	}
	defer fileIn2.Close()
	err = rc.ImageImport(ctx, rOut1, fileIn2)
	if err != nil {
		t.Errorf("failed to import: %v", err)
	}

	fileIn3, err := os.Open(filepath.Join(tempDir, "test3.tar.gz"))
	if err != nil {
		t.Fatalf("failed to open tar: %v", err)
	}
	defer fileIn3.Close()
	err = rc.ImageImport(ctx, rOut3, fileIn3)
	if err != nil {
		t.Errorf("failed to import: %v", err)
	}
}
