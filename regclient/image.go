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

	// "github.com/docker/docker/pkg/archive"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/sirupsen/logrus"
)

func (rc *regClient) ImageCopy(ctx context.Context, refSrc Ref, refTgt Ref) error {
	// check if source and destination already match
	msh, errS := rc.ManifestHead(ctx, refSrc)
	mdh, errD := rc.ManifestHead(ctx, refTgt)
	if errS == nil && errD == nil && msh.GetDigest() == mdh.GetDigest() {
		rc.log.WithFields(logrus.Fields{
			"source": refSrc.Reference,
			"target": refTgt.Reference,
			"digest": msh.GetDigest().String(),
		}).Info("Copy not needed, target already up to date")
		return nil
	}

	// get the manifest for the source
	m, err := rc.ManifestGet(ctx, refSrc)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref": refSrc.Reference,
			"err": err,
		}).Warn("Failed to get source manifest")
		return err
	}

	switch m.GetMediaType() {
	case MediaTypeDocker2ManifestList:
		dml := m.GetDockerManifestList()
		for _, entry := range dml.Manifests {
			entrySrc := refSrc
			entryTgt := refTgt
			entrySrc.Tag = ""
			entryTgt.Tag = ""
			entrySrc.Digest = entry.Digest.String()
			entryTgt.Digest = entry.Digest.String()
			if err := rc.ImageCopy(ctx, entrySrc, entryTgt); err != nil {
				return err
			}
		}

	case MediaTypeOCI1ManifestList:
		oml := m.GetOCIManifestList()
		for _, entry := range oml.Manifests {
			entrySrc := refSrc
			entryTgt := refTgt
			entrySrc.Tag = ""
			entryTgt.Tag = ""
			entrySrc.Digest = entry.Digest.String()
			entryTgt.Digest = entry.Digest.String()
			if err := rc.ImageCopy(ctx, entrySrc, entryTgt); err != nil {
				return err
			}
		}

	case MediaTypeDocker2Manifest, MediaTypeOCI1Manifest:
		// transfer the config
		cd, err := m.GetConfigDigest()
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"ref": refSrc.Reference,
				"err": err,
			}).Warn("Failed to get config digest from manifest")
			return err
		}
		rc.log.WithFields(logrus.Fields{
			"source": refSrc.Reference,
			"target": refTgt.Reference,
			"digest": cd.String(),
		}).Info("Copy config")
		if err := rc.BlobCopy(ctx, refSrc, refTgt, cd.String()); err != nil {
			rc.log.WithFields(logrus.Fields{
				"source": refSrc.Reference,
				"target": refTgt.Reference,
				"digest": cd.String(),
				"err":    err,
			}).Warn("Failed to copy config")
			return err
		}

		// for each layer from the source
		l, err := m.GetLayers()
		if err != nil {
			return err
		}
		for _, layerSrc := range l {
			rc.log.WithFields(logrus.Fields{
				"source": refSrc.Reference,
				"target": refTgt.Reference,
				"layer":  layerSrc.Digest.String(),
			}).Info("Copy layer")
			if err := rc.BlobCopy(ctx, refSrc, refTgt, layerSrc.Digest.String()); err != nil {
				rc.log.WithFields(logrus.Fields{
					"source": refSrc.Reference,
					"target": refTgt.Reference,
					"layer":  layerSrc.Digest.String(),
					"err":    err,
				}).Warn("Failed to copy layer")
				return err
			}
		}

	default:
		return ErrUnsupportedMediaType
	}

	// push manifest to target
	if err := rc.ManifestPut(ctx, refTgt, m); err != nil {
		rc.log.WithFields(logrus.Fields{
			"target": refTgt.Reference,
			"err":    err,
		}).Warn("Failed to push manifest")
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
		rc.log.WithFields(logrus.Fields{
			"ref": ref.Reference,
			"err": err,
		}).Warn("Failed to get manifest")
		return err
	}

	// write to a temp directory
	tempDir, err := ioutil.TempDir("", "regcli-export-")
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"dir": tempDir,
			"err": err,
		}).Warn("Failed to create temp directory")
		return err
	}
	defer os.RemoveAll(tempDir)

	rc.log.WithFields(logrus.Fields{
		"dir": tempDir,
	}).Debug("Using temp directory for export")

	// retrieve the config blob
	cd, err := m.GetConfigDigest()
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Failed to get config digest from manifest")
		return err
	}
	confio, _, err := rc.BlobGet(ctx, ref, cd.String(), []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref":    ref.Reference,
			"digest": cd.String(),
			"err":    err,
		}).Warn("Failed to get config")
		return err
	}
	confstr, err := ioutil.ReadAll(confio)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref":    ref.Reference,
			"digest": cd.String(),
			"err":    err,
		}).Warn("Failed to download config")
		return err
	}
	confDigest := digest.FromBytes(confstr)
	if cd != confDigest {
		rc.log.WithFields(logrus.Fields{
			"ref":        ref.Reference,
			"expected":   cd.String(),
			"calculated": confDigest.String(),
		}).Warn("Config digest mismatch")

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
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Failed to get layers for manifest")
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
			rc.log.WithFields(logrus.Fields{
				"ref":   ref.Reference,
				"layer": layerDesc.Digest.String(),
				"err":   err,
			}).Warn("Failed to download layer")
			return err
		}
		defer layerRComp.Close()
		// decompress layer
		layerTarStream, err := archive.Decompress(layerRComp)
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"err":    err,
				"ref":    ref.Reference,
				"digest": layerDesc.Digest.String(),
			}).Warn("Failed to decompress layer")
			return err
		}
		// TODO: verify received layer is a tgz, check mediatype?
		// generate digest of decompressed layer
		digestTar := digest.Canonical.Digester()
		tr := io.TeeReader(layerTarStream, digestTar.Hash())

		// download to a temp location
		layerTarFile := filepath.Join(layerDir, "layer.tar")
		lf, err := os.OpenFile(layerTarFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"err":  err,
				"file": layerTarFile,
			}).Warn("Failed to create temp layer file")
			return err
		}
		_, err = io.Copy(lf, tr)
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"err":    err,
				"ref":    ref.Reference,
				"digest": layerDesc.Digest.String(),
				"file":   layerTarFile,
			}).Warn("Failed to download layer")
			return err
		}
		lf.Close()

		// update references to uncompressed tar digest in the filesystem, manifest, and image config
		digestFull := digestTar.Digest()
		digestHex := digestFull.Encoded()
		digestDir := filepath.Join(tempDir, digestHex)
		digestFile := filepath.Join(digestHex, "layer.tar")
		digestFileFull := filepath.Join(tempDir, digestFile)
		if err := os.Rename(layerDir, digestDir); err != nil {
			rc.log.WithFields(logrus.Fields{
				"err": err,
				"src": layerDir,
				"tgt": digestDir,
			}).Warn("Failed to rename layer temp dir")
			return err
		}
		if err := os.Chtimes(digestDir, *conf.Created, *conf.Created); err != nil {
			rc.log.WithFields(logrus.Fields{
				"err":  err,
				"file": digestDir,
			}).Warn("Failed to adjust creation time")
			return err
		}
		if err := os.Chtimes(digestFileFull, *conf.Created, *conf.Created); err != nil {
			rc.log.WithFields(logrus.Fields{
				"err":  err,
				"file": digestFileFull,
			}).Warn("Failed to adjust creation time")
			return err
		}
		expManifest.Layers = append(expManifest.Layers, digestFile)
		conf.RootFS.DiffIDs = append(conf.RootFS.DiffIDs, digestFull)
	}
	// TODO: if using goroutines, wait for all layers to finish

	// calc config digest and write to file
	confstr, err = json.Marshal(conf)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error marshaling conf json")
		return err
	}
	confDigest = digest.Canonical.FromBytes(confstr)
	confFile := confDigest.Encoded() + ".json"
	confFileFull := filepath.Join(tempDir, confFile)
	if err := ioutil.WriteFile(confFileFull, confstr, 0644); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error writing conf json")
		return err
	}
	if err := os.Chtimes(confFileFull, *conf.Created, *conf.Created); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error changing conf json timestamp")
		return err
	}
	expManifest.Config = confFile

	// convert to list and write manifest
	ml := []dockerTarManifest{expManifest}
	mlj, err := json.Marshal(ml)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error marshaling manifest")
		return err
	}
	manifestFile := filepath.Join(tempDir, "manifest.json")
	if err := ioutil.WriteFile(manifestFile, mlj, 0644); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error writing manifest")
		return err
	}
	if err := os.Chtimes(manifestFile, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error changing manifest timestamp")
		return err
	}

	// package in tar file
	err = archive.Tar(ctx, tempDir, outStream, archive.Uncompressed)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Error taring temp dir")
		return err
	}

	return nil
}

func (rc *regClient) ImageGetConfig(ctx context.Context, ref Ref, d string) (ociv1.Image, error) {
	img := ociv1.Image{}

	imgIO, _, err := rc.BlobGet(ctx, ref, d, []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return img, err
	}

	imgBody, err := ioutil.ReadAll(imgIO)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref":    ref.Reference,
			"digest": d,
			"err":    err,
		}).Warn("Error reading config blog")
		return img, err
	}

	err = json.Unmarshal(imgBody, &img)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref":  ref.Reference,
			"body": imgBody,
			"err":  err,
		}).Warn("Error unmarshaling conf json")
		return img, err
	}
	return img, nil
}
