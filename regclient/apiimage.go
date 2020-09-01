package regclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/pkg/archive"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func (rc *regClient) ImageCopy(ctx context.Context, refSrc Ref, refTgt Ref) error {
	// get the manifest for the source
	m, err := rc.ManifestGet(ctx, refSrc)
	if err != nil {
		return err
	}

	// transfer the config
	cd, err := m.GetConfigDigest()
	if err != nil {
		return err
	}
	if err := rc.BlobCopy(ctx, refSrc, refTgt, cd.String()); err != nil {
		return err
	}

	// for each layer from the source
	l, err := m.GetLayers()
	if err != nil {
		return err
	}
	for _, layerSrc := range l {
		if err := rc.BlobCopy(ctx, refSrc, refTgt, layerSrc.Digest.String()); err != nil {
			return err
		}
	}

	// push manifest to target
	if err := rc.ManifestPut(ctx, refTgt, m); err != nil {
		return err
	}

	return nil
}

func (rc *regClient) ImageExport(ctx context.Context, ref Ref, outStream io.Writer) error {
	if ref.CommonName() == "" {
		return ErrNotFound
	}

	expManifest := dockerTarManifest{}
	expManifest.RepoTags = append(expManifest.RepoTags, ref.CommonName())

	m, err := rc.ManifestGet(ctx, ref)
	if err != nil {
		return err
	}

	// write to a temp directory
	tempDir, err := ioutil.TempDir("", "regcli-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	fmt.Fprintf(os.Stderr, "Debug: Using temp directory for export \"%s\"\n", tempDir)

	// retrieve the config blob
	cd, err := m.GetConfigDigest()
	if err != nil {
		return err
	}
	confio, _, err := rc.BlobGet(ctx, ref, cd.String(), []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return err
	}
	confstr, err := ioutil.ReadAll(confio)
	if err != nil {
		return err
	}
	confDigest := digest.FromBytes(confstr)
	if cd != confDigest {
		fmt.Fprintf(os.Stderr, "Warning: digest for image config does not match, pulled %s, calculated %s\n", cd.String(), confDigest.String())
	}
	conf := ociv1.Image{}
	err = json.Unmarshal(confstr, &conf)
	if err != nil {
		return err
	}
	// reset the rootfs DiffIDs and recalculate them as layers are downloaded from the manifest
	// layer digest will change when decompressed and docker load expects layers as tar files
	conf.RootFS.DiffIDs = []digest.Digest{}

	l, err := m.GetLayers()
	if err != nil {
		return err
	}
	for _, layerDesc := range l {
		// TODO: wrap layer download in a concurrency throttled goroutine
		// create tempdir for layer
		layerDir, err := ioutil.TempDir(tempDir, "layer-*")
		if err != nil {
			return err
		}
		// no need to defer remove of layerDir, it is inside of tempDir

		// request layer
		layerRComp, _, err := rc.BlobGet(ctx, ref, layerDesc.Digest.String(), []string{})
		if err != nil {
			return err
		}
		// handle any failures before reading to a file
		defer layerRComp.Close()
		// gather digest of compressed stream to verify downloaded blob
		digestComp := digest.Canonical.Digester()
		trComp := io.TeeReader(layerRComp, digestComp.Hash())
		// decompress layer
		layerTarStream, err := archive.DecompressStream(trComp)
		if err != nil {
			return err
		}
		// generate digest of decompressed layer
		digestTar := digest.Canonical.Digester()
		tr := io.TeeReader(layerTarStream, digestTar.Hash())

		// download to a temp location
		layerTarFile := filepath.Join(layerDir, "layer.tar")
		lf, err := os.OpenFile(layerTarFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		_, err = io.Copy(lf, tr)
		if err != nil {
			return err
		}
		lf.Close()

		// verify digests
		if layerDesc.Digest != digestComp.Digest() {
			fmt.Fprintf(os.Stderr, "Warning: digest for layer does not match, pulled %s, calculated %s\n", layerDesc.Digest.String(), digestComp.Digest().String())
		}

		// update references to uncompressed tar digest in the filesystem, manifest, and image config
		digestFull := digestTar.Digest()
		digestHex := digestFull.Encoded()
		digestDir := filepath.Join(tempDir, digestHex)
		digestFile := filepath.Join(digestHex, "layer.tar")
		digestFileFull := filepath.Join(tempDir, digestFile)
		if err := os.Rename(layerDir, digestDir); err != nil {
			return err
		}
		if err := os.Chtimes(digestFileFull, *conf.Created, *conf.Created); err != nil {
			return err
		}
		expManifest.Layers = append(expManifest.Layers, digestFile)
		conf.RootFS.DiffIDs = append(conf.RootFS.DiffIDs, digestFull)
	}
	// TODO: if using goroutines, wait for all layers to finish

	// calc config digest and write to file
	confstr, err = json.Marshal(conf)
	if err != nil {
		return err
	}
	confDigest = digest.Canonical.FromBytes(confstr)
	confFile := confDigest.Encoded() + ".json"
	confFileFull := filepath.Join(tempDir, confFile)
	if err := ioutil.WriteFile(confFileFull, confstr, 0644); err != nil {
		return err
	}
	if err := os.Chtimes(confFileFull, *conf.Created, *conf.Created); err != nil {
		return err
	}
	expManifest.Config = confFile

	// convert to list and write manifest
	ml := []dockerTarManifest{expManifest}
	mlj, err := json.Marshal(ml)
	if err != nil {
		return err
	}
	manifestFile := filepath.Join(tempDir, "manifest.json")
	if err := ioutil.WriteFile(manifestFile, mlj, 0644); err != nil {
		return err
	}
	if err := os.Chtimes(manifestFile, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
		return err
	}

	// package in tar file
	fs, err := archive.Tar(tempDir, archive.Uncompressed)
	if err != nil {
		return err
	}
	defer fs.Close()

	_, err = io.Copy(outStream, fs)

	return nil
}

func (rc *regClient) ImageInspect(ctx context.Context, ref Ref) (ociv1.Image, error) {
	img := ociv1.Image{}

	m, err := rc.ManifestGet(ctx, ref)
	if err != nil {
		return img, err
	}
	cd, err := m.GetConfigDigest()
	if err != nil {
		return img, err
	}
	imgIO, _, err := rc.BlobGet(ctx, ref, cd.String(), []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return img, err
	}

	imgBody, err := ioutil.ReadAll(imgIO)
	if err != nil {
		return img, err
	}
	// fmt.Printf("Body:\n%s\n", respBody)
	err = json.Unmarshal(imgBody, &img)
	if err != nil {
		return img, err
	}
	return img, nil
}
