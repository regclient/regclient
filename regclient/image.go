package regclient

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	// "github.com/docker/docker/pkg/archive"
	"github.com/docker/distribution"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/archive"
	"github.com/regclient/regclient/regclient/manifest"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

const (
	dockerManifestFilename = "manifest.json"
	ociLayoutVersion       = "1.0.0"
	ociIndexFilename       = "index.json"
	annotationRefName      = "org.opencontainers.image.ref.name"
	annotationImageName    = "io.containerd.image.name"
)

// ImageClient provides registry client requests to images
type ImageClient interface {
	ImageCopy(ctx context.Context, refSrc types.Ref, refTgt types.Ref) error
	ImageExport(ctx context.Context, ref types.Ref, outStream io.Writer) error
	ImageImport(ctx context.Context, ref types.Ref, tarFile string) error
}

// used by import/export to match docker tar expected format
type dockerTarManifest struct {
	Config       string
	RepoTags     []string
	Layers       []string
	Parent       digest.Digest                      `json:",omitempty"`
	LayerSources map[digest.Digest]ociv1.Descriptor `json:",omitempty"`
}

type tarFileHandler func(header *tar.Header, trd *tarReadData) error
type tarReadData struct {
	tr          *tar.Reader
	handleAdded bool
	handlers    map[string]tarFileHandler
	processed   map[string]bool
	finish      []func() error
	// data processed from various handlers
	manifests           map[digest.Digest]manifest.Manifest
	ociIndex            ociv1.Index
	ociManifest         manifest.Manifest
	dockerManifestFound bool
	dockerManifestList  []dockerTarManifest
	dockerManifest      dockerSchema2.Manifest
}
type tarWriteData struct {
	tw        *tar.Writer
	dirs      map[string]bool
	files     map[string]bool
	uid, gid  int
	mode      int64
	timestamp time.Time
}

func (rc *regClient) ImageCopy(ctx context.Context, refSrc types.Ref, refTgt types.Ref) error {
	// check if source and destination already match
	mdh, errD := rc.ManifestHead(ctx, refTgt)
	if errD == nil && refTgt.Digest != "" && digest.Digest(refTgt.Digest) == mdh.GetDigest() {
		rc.log.WithFields(logrus.Fields{
			"target": refTgt.Reference,
			"digest": mdh.GetDigest().String(),
		}).Info("Copy not needed, target already up to date")
		return nil
	} else if errD == nil && refTgt.Digest == "" {
		msh, errS := rc.ManifestHead(ctx, refSrc)
		if errS == nil && msh.GetDigest() == mdh.GetDigest() {
			rc.log.WithFields(logrus.Fields{
				"source": refSrc.Reference,
				"target": refTgt.Reference,
				"digest": mdh.GetDigest().String(),
			}).Info("Copy not needed, target already up to date")
			return nil
		}
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

	if refSrc.Registry != refTgt.Registry || refSrc.Repository != refTgt.Repository {
		// copy components of the image if the repository is different
		if m.IsList() {
			// manifest lists need to recursively copy nested images by digest
			pd, err := m.GetDescriptorList()
			if err != nil {
				return err
			}
			for _, entry := range pd {
				entrySrc := refSrc
				entryTgt := refTgt
				entrySrc.Tag = ""
				entryTgt.Tag = ""
				entrySrc.Digest = entry.Digest.String()
				entryTgt.Digest = entry.Digest.String()
				switch entry.MediaType {
				case MediaTypeDocker1Manifest, MediaTypeDocker1ManifestSigned,
					MediaTypeDocker2Manifest, MediaTypeDocker2ManifestList,
					MediaTypeOCI1Manifest, MediaTypeOCI1ManifestList:
					// known manifest media type
					err = rc.ImageCopy(ctx, entrySrc, entryTgt)
				case MediaTypeDocker2ImageConfig, MediaTypeOCI1ImageConfig,
					MediaTypeDocker2Layer, MediaTypeOCI1Layer, MediaTypeOCI1LayerGzip,
					MediaTypeBuildkitCacheConfig:
					// known blob media type
					err = rc.BlobCopy(ctx, entrySrc, entryTgt, entry.Digest)
				default:
					// unknown media type, first try an image copy
					err = rc.ImageCopy(ctx, entrySrc, entryTgt)
					if err != nil {
						// fall back to trying to copy a blob
						err = rc.BlobCopy(ctx, entrySrc, entryTgt, entry.Digest)
					}
				}
				if err != nil {
					return err
				}
			}
		} else {
			// copy components of an image
			// transfer the config
			cd, err := m.GetConfigDigest()
			if err != nil {
				// docker schema v1 does not have a config object, ignore if it's missing
				if !errors.Is(err, ErrUnsupportedMediaType) {
					rc.log.WithFields(logrus.Fields{
						"ref": refSrc.Reference,
						"err": err,
					}).Warn("Failed to get config digest from manifest")
					return fmt.Errorf("Failed to get config digest for %s: %w", refSrc.CommonName(), err)
				}
			} else {
				rc.log.WithFields(logrus.Fields{
					"source": refSrc.Reference,
					"target": refTgt.Reference,
					"digest": cd.String(),
				}).Info("Copy config")
				if err := rc.BlobCopy(ctx, refSrc, refTgt, cd); err != nil {
					rc.log.WithFields(logrus.Fields{
						"source": refSrc.Reference,
						"target": refTgt.Reference,
						"digest": cd.String(),
						"err":    err,
					}).Warn("Failed to copy config")
					return err
				}
			}

			// copy filesystem layers
			l, err := m.GetLayers()
			if err != nil {
				return err
			}
			for _, layerSrc := range l {
				if len(layerSrc.URLs) > 0 {
					// skip blobs where the URLs are defined, these aren't hosted and won't be pulled from the source
					rc.log.WithFields(logrus.Fields{
						"source":        refSrc.Reference,
						"target":        refTgt.Reference,
						"layer":         layerSrc.Digest.String(),
						"external-urls": layerSrc.URLs,
					}).Debug("Skipping external layer")
					continue
				}
				rc.log.WithFields(logrus.Fields{
					"source": refSrc.Reference,
					"target": refTgt.Reference,
					"layer":  layerSrc.Digest.String(),
				}).Info("Copy layer")
				if err := rc.BlobCopy(ctx, refSrc, refTgt, layerSrc.Digest); err != nil {
					rc.log.WithFields(logrus.Fields{
						"source": refSrc.Reference,
						"target": refTgt.Reference,
						"layer":  layerSrc.Digest.String(),
						"err":    err,
					}).Warn("Failed to copy layer")
					return err
				}
			}
		}
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

// ImageExport exports an image to an output stream.
// The format is compatible with "docker load" if a single image is selected and not a manifest list.
// The ref must include a tag for exporting to docker (defaults to latest), and may also include a digest.
// The export is also formatted according to OCI layout which supports multi-platform images.
// <https://github.com/opencontainers/image-spec/blob/master/image-layout.md>
// A tar file will be sent to outStream.
//
// Resulting filesystem:
// oci-layout: created at top level, can be done at the start
// index.json: created at top level, single descriptor with org.opencontainers.image.ref.name annotation pointing to the tag
// manifest.json: created at top level, based on every layer added, only works for a single arch image
// blobs/$algo/$hash: each content addressable object (manifest, config, or layer), created recursively
func (rc *regClient) ImageExport(ctx context.Context, ref types.Ref, outStream io.Writer) error {
	var ociIndex ociv1.Index

	// create tar writer object
	tw := tar.NewWriter(outStream)
	defer tw.Close()
	twd := &tarWriteData{
		tw:    tw,
		dirs:  map[string]bool{},
		files: map[string]bool{},
		mode:  0644,
	}

	// retrieve image manifest
	m, err := rc.ManifestGet(ctx, ref)
	if err != nil {
		rc.log.WithFields(logrus.Fields{
			"ref": ref.CommonName(),
			"err": err,
		}).Warn("Failed to get manifest")
		return err
	}

	// build/write oci-layout
	ociLayout := ociv1.ImageLayout{Version: ociLayoutVersion}
	err = twd.tarWriteFileJSON(ociv1.ImageLayoutFile, ociLayout)
	if err != nil {
		return err
	}

	// create a manifest descriptor
	mBody, err := m.RawBody()
	if err != nil {
		return err
	}
	mDesc := ociv1.Descriptor{
		MediaType: m.GetMediaType(),
		Digest:    m.GetDigest(),
		Size:      int64(len(mBody)),
		Annotations: map[string]string{
			annotationImageName: ref.CommonName(),
			annotationRefName:   ref.Tag,
		},
	}

	// generate/write an OCI index
	ociIndex.SchemaVersion = 2
	ociIndex.Manifests = append(ociIndex.Manifests, mDesc) // add the descriptor to the manifest list
	err = twd.tarWriteFileJSON(ociIndexFilename, ociIndex)
	if err != nil {
		return err
	}

	// append to docker manifest with tag, config filename, each layer filename, and layer descriptors
	if !m.IsList() {
		conf, err := m.GetConfigDescriptor()
		if err != nil {
			return err
		}
		refTag := ref
		if refTag.Digest != "" {
			refTag.Digest = ""
		}
		dockerManifest := dockerTarManifest{
			RepoTags:     []string{refTag.CommonName()},
			Config:       tarOCILayoutDescPath(conf),
			Layers:       []string{},
			LayerSources: map[digest.Digest]ociv1.Descriptor{},
		}
		dl, err := m.GetLayers()
		if err != nil {
			return err
		}
		for _, d := range dl {
			dockerManifest.Layers = append(dockerManifest.Layers, tarOCILayoutDescPath(d))
			dockerManifest.LayerSources[d.Digest] = d
		}

		// marshal manifest and write manifest.json
		err = twd.tarWriteFileJSON(dockerManifestFilename, []dockerTarManifest{dockerManifest})
		if err != nil {
			return err
		}
	}

	// recursively include manifests and nested blobs
	err = rc.imageExportDescriptor(ctx, ref, mDesc, twd)
	if err != nil {
		return err
	}

	return nil
}

// imageExportDescriptor pulls a manifest or blob, outputs to a tar file, and recursively processes any nested manifests or blobs
func (rc *regClient) imageExportDescriptor(ctx context.Context, ref types.Ref, desc ociv1.Descriptor, twd *tarWriteData) error {
	tarFilename := tarOCILayoutDescPath(desc)
	if twd.files[tarFilename] {
		// blob has already been imported into tar, skip
		return nil
	}
	switch desc.MediaType {
	case MediaTypeDocker1Manifest, MediaTypeDocker1ManifestSigned, MediaTypeDocker2Manifest, MediaTypeOCI1Manifest:
		// Handle single platform manifests
		// retrieve manifest
		mRef := ref
		mRef.Digest = desc.Digest.String()
		m, err := rc.ManifestGet(ctx, mRef)
		if err != nil {
			return err
		}
		// write manifest body by digest
		mBody, err := m.RawBody()
		if err != nil {
			return err
		}
		err = twd.tarWriteHeader(tarFilename, int64(len(mBody)))
		if err != nil {
			return err
		}
		_, err = twd.tw.Write(mBody)
		if err != nil {
			return err
		}

		// add config
		confD, err := m.GetConfigDescriptor()
		// ignore unsupported media type errors
		if err != nil && !errors.Is(err, manifest.ErrUnsupportedMediaType) {
			return err
		}
		if err == nil {
			err = rc.imageExportDescriptor(ctx, ref, confD, twd)
			if err != nil {
				return err
			}
		}

		// loop over layers
		layerDL, err := m.GetLayers()
		// ignore unsupported media type errors
		if err != nil && !errors.Is(err, manifest.ErrUnsupportedMediaType) {
			return err
		}
		if err == nil {
			for _, layerD := range layerDL {
				err = rc.imageExportDescriptor(ctx, ref, layerD, twd)
				if err != nil {
					return err
				}
			}
		}

	case MediaTypeDocker2ManifestList, MediaTypeOCI1ManifestList:
		// handle OCI index and Docker manifest list
		// retrieve manifest
		mRef := ref
		mRef.Digest = desc.Digest.String()
		m, err := rc.ManifestGet(ctx, mRef)
		if err != nil {
			return err
		}
		// write manifest body by digest
		mBody, err := m.RawBody()
		if err != nil {
			return err
		}
		err = twd.tarWriteHeader(tarFilename, int64(len(mBody)))
		if err != nil {
			return err
		}
		_, err = twd.tw.Write(mBody)
		if err != nil {
			return err
		}
		// recurse over entries in the list/index
		mdl, err := m.GetDescriptorList()
		if err != nil {
			return err
		}
		for _, md := range mdl {
			err = rc.imageExportDescriptor(ctx, ref, md, twd)
			if err != nil {
				return err
			}
		}

	default:
		// get blob
		blobR, err := rc.BlobGet(ctx, ref, desc.Digest, []string{})
		if err != nil {
			return err
		}
		defer blobR.Close()
		// write blob by digest
		err = twd.tarWriteHeader(tarFilename, int64(desc.Size))
		if err != nil {
			return err
		}
		size, err := io.Copy(twd.tw, blobR)
		if err != nil {
			return fmt.Errorf("Failed to export blob %s: %w", desc.Digest.String(), err)
		}
		if size != desc.Size {
			return fmt.Errorf("Blob size mismatch, descriptor %d, received %d", desc.Size, size)
		}
	}

	return nil
}

// ImageImport pushes an image from a tar file to a registry
func (rc *regClient) ImageImport(ctx context.Context, ref types.Ref, tarFile string) error {
	trd := &tarReadData{
		handlers:  map[string]tarFileHandler{},
		processed: map[string]bool{},
		finish:    []func() error{},
		manifests: map[digest.Digest]manifest.Manifest{},
	}
	// open tarFile
	fh, err := os.Open(tarFile)
	if err != nil {
		return err
	}

	// add handler for oci-layout, index.json, and manifest.json
	rc.imageImportOCIAddHandler(ctx, ref, trd)
	rc.imageImportDockerAddHandler(trd)

	// process tar file looking for oci-layout and index.json, load manifests/blobs on success
	err = trd.tarReadAll(fh)

	if err != nil && errors.Is(err, ErrNotFound) && trd.dockerManifestFound {
		// import failed but manifest.json found, fall back to manifest.json processing
		// add handlers for the docker manifest layers
		rc.imageImportDockerAddLayerHandlers(ctx, ref, trd)
		// reprocess the tar looking for manifest.json files
		err = trd.tarReadAll(fh)
		if err != nil {
			return fmt.Errorf("Failed to import layers from docker tar: %w", err)
		}
		// push docker manifest
		m, err := manifest.FromOrig(trd.dockerManifest)
		if err != nil {
			return err
		}
		err = rc.ManifestPut(ctx, ref, m)
		if err != nil {
			return err
		}
	} else if err != nil {
		// unhandled error from tar read
		return err
	} else {
		// successful load of OCI blobs, now push manifest and tag
		err = rc.imageImportOCIPushManifests(ctx, ref, trd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rc *regClient) imageImportBlob(ctx context.Context, ref types.Ref, desc ociv1.Descriptor, trd *tarReadData) error {
	// skip if blob already exists
	_, err := rc.BlobHead(ctx, ref, desc.Digest)
	if err == nil {
		return nil
	}
	// upload blob
	_, _, err = rc.BlobPut(ctx, ref, desc.Digest, trd.tr, "", desc.Size)
	if err != nil {
		return err
	}
	return nil
}

// imageImportDockerAddHandler processes tar files generated by docker
func (rc *regClient) imageImportDockerAddHandler(trd *tarReadData) {
	trd.handlers[dockerManifestFilename] = func(header *tar.Header, trd *tarReadData) error {
		err := trd.tarReadFileJSON(&trd.dockerManifestList)
		if err != nil {
			return err
		}
		trd.dockerManifestFound = true
		return nil
	}
}

// imageImportDockerAddLayerHandlers imports the docker layers when OCI import fails and docker manifest found
func (rc *regClient) imageImportDockerAddLayerHandlers(ctx context.Context, ref types.Ref, trd *tarReadData) {
	// remove handlers for OCI
	delete(trd.handlers, ociv1.ImageLayoutFile)
	delete(trd.handlers, ociIndexFilename)

	// make a docker v2 manifest from first json array entry (can only tag one image)
	trd.dockerManifest.SchemaVersion = 2
	trd.dockerManifest.MediaType = MediaTypeDocker2Manifest
	trd.dockerManifest.Layers = make([]distribution.Descriptor, len(trd.dockerManifestList[0].Layers))

	// add handler for config
	trd.handlers[trd.dockerManifestList[0].Config] = func(header *tar.Header, trd *tarReadData) error {
		// upload blob, digest is unknown
		d, size, err := rc.BlobPut(ctx, ref, "", trd.tr, "", header.Size)
		if err != nil {
			return err
		}
		// save the resulting descriptor to the manifest
		if od, ok := trd.dockerManifestList[0].LayerSources[d]; ok {
			trd.dockerManifest.Config = oci2DDesc(od)
		} else {
			trd.dockerManifest.Config = distribution.Descriptor{
				Digest:    d,
				Size:      size,
				MediaType: MediaTypeDocker2ImageConfig,
			}
		}
		return nil
	}
	// add handlers for each layer
	for i, layerFile := range trd.dockerManifestList[0].Layers {
		func(i int) {
			trd.handlers[layerFile] = func(header *tar.Header, trd *tarReadData) error {
				// ensure blob is compressed with gzip to match media type
				gzipR, err := archive.Compress(trd.tr, archive.CompressGzip)
				if err != nil {
					return err
				}
				// upload blob, digest and size is unknown
				d, size, err := rc.BlobPut(ctx, ref, "", gzipR, "", 0)
				if err != nil {
					return err
				}
				// save the resulting descriptor in the appropriate layer
				if od, ok := trd.dockerManifestList[0].LayerSources[d]; ok {
					trd.dockerManifest.Layers[i] = oci2DDesc(od)
				} else {
					trd.dockerManifest.Layers[i] = distribution.Descriptor{
						MediaType: MediaTypeDocker2Layer,
						Size:      size,
						Digest:    d,
					}
				}
				return nil
			}
		}(i)
	}
	trd.handleAdded = true
}

// imageImportOCIAddHandler adds handlers for oci-layout and index.json found in OCI layout tar files
func (rc *regClient) imageImportOCIAddHandler(ctx context.Context, ref types.Ref, trd *tarReadData) {
	// add handler for oci-layout, index.json, and manifest.json
	var err error
	var foundLayout, foundIndex bool

	// common handler code when both oci-layout and index.json have been processed
	ociHandler := func(trd *tarReadData) error {
		// no need to process docker manifest.json when OCI layout is available
		delete(trd.handlers, dockerManifestFilename)
		// create a manifest from the index
		trd.ociManifest, err = manifest.FromOrig(trd.ociIndex)
		if err != nil {
			return err
		}
		// start recursively processing manifests starting with the index
		// there's no need to push the index.json by digest, it will be pushed by tag if needed
		err = rc.imageImportOCIHandleManifest(ctx, ref, trd.ociManifest, trd, false)
		if err != nil {
			return err
		}
		return nil
	}
	trd.handlers[ociv1.ImageLayoutFile] = func(header *tar.Header, trd *tarReadData) error {
		var ociLayout ociv1.ImageLayout
		err := trd.tarReadFileJSON(&ociLayout)
		if err != nil {
			return err
		}
		if ociLayout.Version != ociLayoutVersion {
			// unknown version, ignore
			rc.log.WithFields(logrus.Fields{
				"version": ociLayout.Version,
			}).Warn("Unsupported oci-layout version")
			return nil
		}
		foundLayout = true
		if foundIndex {
			err = ociHandler(trd)
			if err != nil {
				return err
			}
		}
		return nil
	}
	trd.handlers[ociIndexFilename] = func(header *tar.Header, trd *tarReadData) error {
		err := trd.tarReadFileJSON(&trd.ociIndex)
		if err != nil {
			return err
		}
		foundIndex = true
		if foundLayout {
			err = ociHandler(trd)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// imageImportOCIHandleManifest recursively processes index and manifest entries from an OCI layout tar
func (rc *regClient) imageImportOCIHandleManifest(ctx context.Context, ref types.Ref, m manifest.Manifest, trd *tarReadData, push bool) error {
	// cache the manifest to avoid needing to pull again later, this is used if index.json is a wrapper around some other manifest
	trd.manifests[m.GetDigest()] = m

	handleManifest := func(d ociv1.Descriptor) {
		filename := tarOCILayoutDescPath(d)
		if !trd.processed[filename] && trd.handlers[filename] == nil {
			trd.handlers[filename] = func(header *tar.Header, trd *tarReadData) error {
				b, err := ioutil.ReadAll(trd.tr)
				if err != nil {
					return err
				}
				switch d.MediaType {
				case MediaTypeDocker1Manifest, MediaTypeDocker1ManifestSigned,
					MediaTypeDocker2Manifest, MediaTypeDocker2ManifestList,
					MediaTypeOCI1Manifest, MediaTypeOCI1ManifestList:
					// known manifest media types
					md, err := manifest.FromDescriptor(d, b)
					if err != nil {
						return err
					}
					return rc.imageImportOCIHandleManifest(ctx, ref, md, trd, true)
				case MediaTypeDocker2ImageConfig, MediaTypeOCI1ImageConfig,
					MediaTypeDocker2Layer, MediaTypeOCI1Layer, MediaTypeOCI1LayerGzip,
					MediaTypeBuildkitCacheConfig:
					// known blob media types
					return rc.imageImportBlob(ctx, ref, d, trd)
				default:
					// attempt manifest import, fall back to blob import
					md, err := manifest.FromDescriptor(d, b)
					if err == nil {
						return rc.imageImportOCIHandleManifest(ctx, ref, md, trd, true)
					}
					return rc.imageImportBlob(ctx, ref, d, trd)
				}
			}
		}
	}

	if !push {
		// for root index, add handler for matching reference (or only reference)
		dl, err := m.GetDescriptorList()
		if err != nil {
			return err
		}
		// locate the digest in the index
		var d ociv1.Descriptor
		if len(dl) == 1 {
			d = dl[0]
		} else {
			// if more than one digest is in the index, use the first matching tag
			for _, cur := range dl {
				if cur.Annotations[annotationRefName] == ref.Tag {
					d = cur
					break
				}
			}
		}
		if d.Digest.String() == "" {
			return fmt.Errorf("could not find requested tag in index.json, %s", ref.Tag)
		}
		handleManifest(d)
		// add a finish step to tag the selected digest
		trd.finish = append(trd.finish, func() error {
			mRef, ok := trd.manifests[d.Digest]
			if !ok {
				return fmt.Errorf("could not find manifest to tag, ref: %s, digest: %s", ref.CommonName(), d.Digest)
			}
			return rc.ManifestPut(ctx, ref, mRef)
		})
	} else if m.IsList() {
		// for index/manifest lists, add handlers for each embedded manifest
		dl, err := m.GetDescriptorList()
		if err != nil {
			return err
		}
		for _, d := range dl {
			handleManifest(d)
		}
	} else {
		// else if a single image/manifest
		// add handler for the config descriptor if it's defined
		cd, err := m.GetConfigDescriptor()
		if err == nil {
			filename := tarOCILayoutDescPath(cd)
			if !trd.processed[filename] && trd.handlers[filename] == nil {
				func(cd ociv1.Descriptor) {
					trd.handlers[filename] = func(header *tar.Header, trd *tarReadData) error {
						return rc.imageImportBlob(ctx, ref, cd, trd)
					}
				}(cd)
			}
		}
		// add handlers for each layer
		layers, err := m.GetLayers()
		if err != nil {
			return err
		}
		for _, d := range layers {
			filename := tarOCILayoutDescPath(d)
			if !trd.processed[filename] && trd.handlers[filename] == nil {
				func(d ociv1.Descriptor) {
					trd.handlers[filename] = func(header *tar.Header, trd *tarReadData) error {
						return rc.imageImportBlob(ctx, ref, d, trd)
					}
				}(d)
			}
		}
	}
	// add a finish func to push the manifest, this gets skipped for the index.json
	if push {
		trd.finish = append(trd.finish, func() error {
			mRef := ref
			mRef.Digest = string(m.GetDigest())
			_, err := rc.ManifestHead(ctx, mRef)
			if err == nil {
				return nil
			}
			return rc.ManifestPut(ctx, mRef, m)
		})
	}
	trd.handleAdded = true
	return nil
}

// imageImportOCIPushManifests uploads manifests after OCI blobs were successfully loaded
func (rc *regClient) imageImportOCIPushManifests(ctx context.Context, ref types.Ref, trd *tarReadData) error {
	// run finish handlers in reverse order to upload nested manifests
	for i := len(trd.finish) - 1; i >= 0; i-- {
		err := trd.finish[i]()
		if err != nil {
			return err
		}
	}
	return nil
}

func oci2DDesc(od ociv1.Descriptor) distribution.Descriptor {
	return distribution.Descriptor{
		MediaType:   od.MediaType,
		Digest:      od.Digest,
		Size:        od.Size,
		URLs:        od.URLs,
		Annotations: od.Annotations,
		Platform:    od.Platform,
	}
}

// tarReadAll processes the tar file in a loop looking for matching filenames in the list of handlers
// handlers for filenames are added at the top level, and by manifest imports
func (trd *tarReadData) tarReadAll(fh *os.File) error {
	// return immediately if nothing to do
	if len(trd.handlers) == 0 {
		return nil
	}
	for {
		// reset back to beginning of tar file
		_, err := fh.Seek(0, 0)
		if err != nil {
			return err
		}
		trd.tr = tar.NewReader(fh)
		trd.handleAdded = false
		// loop over each entry of the tar file
		for {
			header, err := trd.tr.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			// if a handler exists, run it, remove handler, and check if we are done
			if trd.handlers[header.Name] != nil {
				err = trd.handlers[header.Name](header, trd)
				if err != nil {
					return err
				}
				delete(trd.handlers, header.Name)
				trd.processed[header.Name] = true
				// return if last handler processed
				if len(trd.handlers) == 0 {
					return nil
				}
			}
		}
		// if entire file read without adding a new handler, fail
		if !trd.handleAdded {
			files := []string{}
			for file := range trd.handlers {
				files = append(files, file)
			}
			return fmt.Errorf("Unable to export all files from tar: %w", ErrNotFound)
		}
	}
}

// tarReadFileJSON reads the current tar entry and unmarshals json into provided interface
func (trd *tarReadData) tarReadFileJSON(data interface{}) error {
	b, err := ioutil.ReadAll(trd.tr)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, data)
	if err != nil {
		return err
	}
	return nil
}

var errTarFileExists = errors.New("Tar file already exists")

func (td *tarWriteData) tarWriteHeader(filename string, size int64) error {
	dirname := filepath.Dir(filename)
	if !td.dirs[dirname] && dirname != "." {
		header := tar.Header{
			Format:     tar.FormatPAX,
			Typeflag:   tar.TypeDir,
			Name:       dirname,
			Size:       0,
			Mode:       td.mode | 0511,
			ModTime:    td.timestamp,
			AccessTime: td.timestamp,
			ChangeTime: td.timestamp,
		}
		err := td.tw.WriteHeader(&header)
		if err != nil {
			return err
		}
		td.dirs[dirname] = true
	}
	if td.files[filename] {
		return fmt.Errorf("%w: %s", errTarFileExists, filename)
	}
	td.files[filename] = true
	header := tar.Header{
		Format:     tar.FormatPAX,
		Typeflag:   tar.TypeReg,
		Name:       filename,
		Size:       size,
		Mode:       td.mode | 0400,
		ModTime:    td.timestamp,
		AccessTime: td.timestamp,
		ChangeTime: td.timestamp,
	}
	return td.tw.WriteHeader(&header)
}

func (td *tarWriteData) tarWriteFileJSON(filename string, data interface{}) error {
	dataJson, err := json.Marshal(data)
	if err != nil {
		return err
	}
	err = td.tarWriteHeader(filename, int64(len(dataJson)))
	if err != nil {
		return err
	}
	_, err = td.tw.Write(dataJson)
	if err != nil {
		return err
	}
	return nil
}

func tarOCILayoutDescPath(d ociv1.Descriptor) string {
	return fmt.Sprintf("blobs/%s/%s", d.Digest.Algorithm(), d.Digest.Encoded())
}
