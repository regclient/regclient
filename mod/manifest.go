package mod

import (
	"context"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

// WithAnnotation adds an annotation, or deletes it if the value is set to an empty string.
// If name is not prefixed with a platform selector, this only applies to the top level manifest.
// Name may be prefixed with a list of platforms "[p1,p2,...]name", e.g. "[linux/amd64]com.example.field".
// The platform selector may also be "[*]" to apply to all manifests, including the top level manifest list.
func WithAnnotation(name, value string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		// extract the list for platforms to update from the name
		name = strings.TrimSpace(name)
		platforms := []platform.Platform{}
		allPlatforms := false
		if name[0] == '[' && strings.Index(name, "]") > 0 {
			end := strings.Index(name, "]")
			list := strings.Split(name[1:end], ",")
			for _, entry := range list {
				entry = strings.TrimSpace(entry)
				if entry == "*" {
					allPlatforms = true
					continue
				}
				p, err := platform.Parse(entry)
				if err != nil {
					return fmt.Errorf("failed to parse annotation platform %s: %w", entry, err)
				}
				platforms = append(platforms, p)
			}
			name = name[end+1:]
		}
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			// skip deleted manifest, those not in the platform list, or the non-top manifest if no platform list provided
			if dm.mod == deleted {
				return nil
			}
			if len(platforms) > 0 && !allPlatforms {
				if dm.m.IsList() || dm.config == nil || dm.config.oc == nil {
					return nil
				}
				p := dm.config.oc.GetConfig().Platform
				found := false
				for _, pe := range platforms {
					if platform.Match(p, pe) {
						found = true
						break
					}
				}
				if !found {
					return nil
				}
			}
			if len(platforms) == 0 && !allPlatforms && !dm.top {
				return nil
			}
			// check if annotation is already set to the correct value
			ma := dm.m.(manifest.Annotator)
			annotations, err := ma.GetAnnotations()
			if err != nil {
				return err
			}
			if annotations == nil {
				annotations = map[string]string{}
			}
			cur, ok := annotations[name]
			if (value == "" && !ok) || (value != "" && value == cur) {
				return nil
			}
			// update annotation
			err = ma.SetAnnotation(name, value)
			if err != nil {
				return err
			}
			if dm.mod == unchanged {
				dm.mod = replaced
			}
			dm.newDesc = dm.m.GetDescriptor()
			return nil
		})
		return nil
	}
}

// WithAnnotationOCIBase adds annotations for the base image.
func WithAnnotationOCIBase(rBase ref.Ref, dBase digest.Digest) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			annoBaseDig := "org.opencontainers.image.base.digest"
			annoBaseName := "org.opencontainers.image.base.name"
			changed := false
			om := dm.m.GetOrig()
			if dm.m.IsList() {
				ociI, err := manifest.OCIIndexFromAny(om)
				if err != nil {
					return err
				}
				if ociI.Annotations == nil {
					ociI.Annotations = map[string]string{}
				}
				if ociI.Annotations[annoBaseName] != rBase.CommonName() {
					ociI.Annotations[annoBaseName] = rBase.CommonName()
					changed = true
				}
				if ociI.Annotations[annoBaseDig] != dBase.String() {
					ociI.Annotations[annoBaseDig] = dBase.String()
					changed = true
				}
				err = manifest.OCIIndexToAny(ociI, &om)
				if err != nil {
					return err
				}
			} else {
				ociM, err := manifest.OCIManifestFromAny(om)
				if err != nil {
					return err
				}
				if ociM.Annotations == nil {
					ociM.Annotations = map[string]string{}
				}
				if ociM.Annotations[annoBaseName] != rBase.CommonName() {
					ociM.Annotations[annoBaseName] = rBase.CommonName()
					changed = true
				}
				if ociM.Annotations[annoBaseDig] != dBase.String() {
					ociM.Annotations[annoBaseDig] = dBase.String()
					changed = true
				}
				err = manifest.OCIManifestToAny(ociM, &om)
				if err != nil {
					return err
				}
			}
			if changed {
				if dm.mod == unchanged {
					dm.mod = replaced
				}
				err := dm.m.SetOrig(om)
				if err != nil {
					return err
				}
				dm.newDesc = dm.m.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}

// WithAnnotationPromoteCommon pulls up common annotations from child images to the index.
func WithAnnotationPromoteCommon() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			changed := false
			if !dm.m.IsList() {
				return nil
			}
			ociI, err := manifest.OCIIndexFromAny(dm.m.GetOrig())
			if err != nil {
				return err
			}
			var common map[string]string
			for _, child := range dm.manifests {
				if child.mod == deleted {
					continue
				}
				mAnnot, ok := child.m.(manifest.Annotator)
				if !ok {
					return fmt.Errorf("manifest does not support annotations: %s%.0w", child.m.GetDescriptor().Digest.String(), errs.ErrUnsupportedMediaType)
				}
				cur, err := mAnnot.GetAnnotations()
				if err != nil {
					return err
				}
				if common == nil {
					common = cur
				} else {
					for k, v := range common {
						if curV, ok := cur[k]; !ok || v != curV {
							delete(common, k)
						}
					}
				}
				if len(common) == 0 {
					return nil
				}
			}
			if len(common) == 0 {
				return nil
			}
			if ociI.Annotations == nil {
				ociI.Annotations = common
				changed = true
			} else {
				for k, v := range common {
					if curV, ok := ociI.Annotations[k]; !ok || v != curV {
						ociI.Annotations[k] = v
						changed = true
					}
				}
			}
			if changed {
				if dm.mod == unchanged {
					dm.mod = replaced
				}
				err = dm.m.SetOrig(ociI)
				if err != nil {
					return err
				}
				dm.newDesc = dm.m.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}

// WithLabelToAnnotation copies image config labels to manifest annotations.
func WithLabelToAnnotation() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			changed := false
			if dm.m.IsList() {
				return nil
			}
			om := dm.m.GetOrig()
			ociOM, err := manifest.OCIManifestFromAny(om)
			if err != nil {
				return err
			}
			if ociOM.Annotations == nil {
				ociOM.Annotations = map[string]string{}
			}
			if dm.config == nil || dm.config.oc == nil {
				return nil
			}
			oc := dm.config.oc.GetConfig()
			if oc.Config.Labels == nil {
				return nil
			}
			for name, value := range oc.Config.Labels {
				cur, ok := ociOM.Annotations[name]
				if !ok || cur != value {
					ociOM.Annotations[name] = value
					changed = true
				}
			}
			if !changed {
				return nil
			}
			err = manifest.OCIManifestToAny(ociOM, &om)
			if err != nil {
				return err
			}
			err = dm.m.SetOrig(om)
			if err != nil {
				return err
			}
			dm.newDesc = dm.m.GetDescriptor()
			if dm.mod == unchanged {
				dm.mod = replaced
			}
			return nil
		})
		return nil
	}
}

// WithManifestDigestAlgo changes the digester algorithm.
func WithManifestDigestAlgo(algo digest.Algorithm) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		if !algo.Available() {
			return fmt.Errorf("digest algorithm is not available: %s", string(algo))
		}
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			origDig := dm.m.GetDescriptor().Digest
			if dm.newDesc.Digest != "" {
				origDig = dm.newDesc.Digest
			}
			if origDig.Validate() == nil && origDig.Algorithm() == algo {
				return nil
			}
			desc := dm.m.GetDescriptor()
			desc.Digest = ""
			err := desc.DigestAlgoPrefer(algo)
			if err != nil {
				return err
			}
			om := dm.m.GetOrig()
			dm.m, err = manifest.New(
				manifest.WithDesc(desc),
				manifest.WithOrig(om),
			)
			if err != nil {
				return err
			}
			dm.newDesc = dm.m.GetDescriptor()
			if dm.mod == unchanged {
				dm.mod = replaced
			}
			return nil
		})
		return nil
	}
}

// WithManifestToDocker converts the manifest to Docker schema2 media types.
func WithManifestToDocker() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			changed := false
			om := dm.m.GetOrig()
			if dm.m.IsList() {
				if dm.m.GetDescriptor().MediaType != mediatype.Docker2ManifestList {
					ociM, err := manifest.OCIIndexFromAny(om)
					if err != nil {
						return err
					}
					dml := schema2.ManifestList{}
					err = manifest.OCIIndexToAny(ociM, &dml)
					if err != nil {
						return err
					}
					changed = true
					om = dml
				}
			} else {
				ociM, err := manifest.OCIManifestFromAny(om)
				if err != nil {
					return err
				}
				if dm.m.GetDescriptor().MediaType != mediatype.Docker2Manifest {
					changed = true
				}
				if ociM.ArtifactType != "" {
					return fmt.Errorf("unable to convert artifactType to docker manifest, ref %s%.0w", rSrc.CommonName(), errs.ErrUnsupportedMediaType)
				}
				if ociM.Config.MediaType == mediatype.OCI1ImageConfig {
					ociM.Config.MediaType = mediatype.Docker2ImageConfig
					changed = true
				}
				for i, l := range ociM.Layers {
					switch l.MediaType {
					case mediatype.OCI1Layer:
						ociM.Layers[i].MediaType = mediatype.Docker2Layer
					case mediatype.OCI1LayerGzip:
						ociM.Layers[i].MediaType = mediatype.Docker2LayerGzip
					case mediatype.OCI1LayerZstd:
						ociM.Layers[i].MediaType = mediatype.Docker2LayerZstd
					case mediatype.OCI1ForeignLayerGzip:
						ociM.Layers[i].MediaType = mediatype.Docker2ForeignLayer
					default:
						continue
					}
					changed = true
				}
				if changed {
					dm := schema2.Manifest{}
					err = manifest.OCIManifestToAny(ociM, &dm)
					if err != nil {
						return err
					}
					om = dm
				}
			}
			if !changed {
				return nil
			}
			newM, err := manifest.New(manifest.WithOrig(om))
			if err != nil {
				return err
			}
			dm.m = newM
			dm.newDesc = dm.m.GetDescriptor()
			if dm.mod == unchanged {
				dm.mod = replaced
			}
			return nil
		})
		return nil
	}
}

// WithManifestToOCI converts the manifest to OCI media types.
func WithManifestToOCI() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			changed := false
			om := dm.m.GetOrig()
			if dm.m.IsList() {
				ociM, err := manifest.OCIIndexFromAny(om)
				if err != nil {
					return err
				}
				if dm.m.GetDescriptor().MediaType != mediatype.OCI1ManifestList {
					changed = true
					om = ociM
				}
			} else {
				ociM, err := manifest.OCIManifestFromAny(om)
				if err != nil {
					return err
				}
				if dm.m.GetDescriptor().MediaType != mediatype.OCI1Manifest {
					changed = true
				}
				if ociM.Config.MediaType == mediatype.Docker2ImageConfig {
					ociM.Config.MediaType = mediatype.OCI1ImageConfig
					changed = true
				}
				for i, l := range ociM.Layers {
					switch l.MediaType {
					case mediatype.Docker2Layer:
						ociM.Layers[i].MediaType = mediatype.OCI1Layer
					case mediatype.Docker2LayerGzip:
						ociM.Layers[i].MediaType = mediatype.OCI1LayerGzip
					case mediatype.Docker2LayerZstd:
						ociM.Layers[i].MediaType = mediatype.OCI1LayerZstd
					case mediatype.Docker2ForeignLayer:
						ociM.Layers[i].MediaType = mediatype.OCI1ForeignLayerGzip
					default:
						continue
					}
					changed = true
				}
				if changed {
					om = ociM
				}
			}
			if !changed {
				return nil
			}
			newM, err := manifest.New(manifest.WithOrig(om))
			if err != nil {
				return err
			}
			dm.m = newM
			dm.newDesc = dm.m.GetDescriptor()
			if dm.mod == unchanged {
				dm.mod = replaced
			}
			return nil
		})
		return nil
	}
}

const (
	dockerReferenceType   = "vnd.docker.reference.type"
	dockerReferenceDigest = "vnd.docker.reference.digest"
)

// WithManifestToOCIReferrers converts other referrer types to OCI subject/referrers.
func WithManifestToOCIReferrers() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			mi, ok := dm.m.(manifest.Indexer)
			if ok {
				changed := false
				// create a map of digests for each child DM we may need to reference later
				dmLU := map[string]*dagManifest{
					dm.origDesc.Digest.String(): dm,
				}
				for _, childDM := range dm.manifests {
					dmLU[childDM.origDesc.Digest.String()] = childDM
				}
				ml, err := mi.GetManifestList()
				if err != nil {
					return fmt.Errorf("failed to get manifest list: %w", err)
				}
				mlI := 0
				for _, childDM := range dm.manifests {
					if childDM.mod == added {
						continue
					}
					if childDM.mod == deleted {
						mlI++
						continue
					}
					// get descriptor, skip descriptors that are not docker referrers
					if mlI >= len(ml) {
						return fmt.Errorf("could not find descriptor, index=%d, digest=%s", mlI, dm.origDesc.Digest.String())
					}
					desc := ml[mlI]
					mlI++
					if len(desc.Annotations) == 0 || desc.Annotations[dockerReferenceType] == "" || desc.Annotations[dockerReferenceDigest] == "" {
						continue
					}
					// find the subjectDM
					subjectDM, ok := dmLU[desc.Annotations[dockerReferenceDigest]]
					if !ok || subjectDM == nil {
						return fmt.Errorf("could not find digest, convert referrers before other mod actions, digest=%s", desc.Annotations[dockerReferenceDigest])
					}
					// validate the manifest being converted
					_, ok = childDM.m.(manifest.Subjecter)
					if !ok {
						return fmt.Errorf("docker reference type does not support subject, mt=%s", childDM.m.GetDescriptor().MediaType)
					}
					am, ok := childDM.m.(manifest.Annotator)
					if !ok {
						return fmt.Errorf("docker reference type does not support annotations, mt=%s", childDM.m.GetDescriptor().MediaType)
					}
					err := am.SetAnnotation(dockerReferenceType, desc.Annotations[dockerReferenceType])
					if err != nil {
						return fmt.Errorf("failed to set annotations: %w", err)
					}
					// copy childDM to add a referrer entry to the targetDM
					referrerDM := *childDM
					referrerDM.mod = added
					subjectDM.referrers = append(subjectDM.referrers, &referrerDM)
					// mark this childDM for deletion
					changed = true
					childDM.mod = deleted
				}
				if changed {
					if dm.mod == unchanged {
						dm.mod = replaced
					}
				}
			}
			return nil
		})
		return nil
	}
}

// WithExternalURLsRm strips external URLs from descriptors and adjusts media type to match.
func WithExternalURLsRm() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted {
				return nil
			}
			changed := false
			if dm.m.IsList() {
				return nil
			}
			om := dm.m.GetOrig()
			ociOM, err := manifest.OCIManifestFromAny(om)
			if err != nil {
				return err
			}
			// strip layers from image
			for i := range ociOM.Layers {
				if len(ociOM.Layers[i].URLs) > 0 {
					ociOM.Layers[i].URLs = []string{}
					mt := ociOM.Layers[i].MediaType
					switch mt {
					case mediatype.Docker2ForeignLayer:
						mt = mediatype.Docker2LayerGzip
					case mediatype.OCI1ForeignLayer:
						mt = mediatype.OCI1Layer
					case mediatype.OCI1ForeignLayerGzip:
						mt = mediatype.OCI1LayerGzip
					case mediatype.OCI1ForeignLayerZstd:
						mt = mediatype.OCI1LayerZstd
					}
					ociOM.Layers[i].MediaType = mt
					changed = true
				}
			}
			// also strip from dag so other steps don't skip the external layer
			for i, dl := range dm.layers {
				if dl.mod == deleted {
					continue
				}
				if dl.newDesc.Digest == "" && len(dl.desc.URLs) > 0 {
					dl.newDesc = dl.desc
				}
				if len(dl.newDesc.URLs) > 0 {
					dl.newDesc.URLs = []string{}
					dm.layers[i] = dl
				}
			}
			if !changed {
				return nil
			}
			err = manifest.OCIManifestToAny(ociOM, &om)
			if err != nil {
				return err
			}
			err = dm.m.SetOrig(om)
			if err != nil {
				return err
			}
			dm.newDesc = dm.m.GetDescriptor()
			if dm.mod == unchanged {
				dm.mod = replaced
			}
			return nil
		})
		return nil
	}
}

// WithRebase attempts to rebase the image using OCI annotations identifying the base image.
func WithRebase() Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		ma, ok := dm.m.(manifest.Annotator)
		if !ok {
			return fmt.Errorf("rebased failed, manifest does not support annotations")
		}
		annot, err := ma.GetAnnotations()
		if err != nil {
			return fmt.Errorf("failed getting annotations: %w", err)
		}
		baseName, ok := annot[types.AnnotationBaseImageName]
		if !ok {
			return fmt.Errorf("annotation for base image is missing (%s or %s)%.0w", types.AnnotationBaseImageName, types.AnnotationBaseImageDigest, errs.ErrMissingAnnotation)
		}
		baseDigest, ok := annot[types.AnnotationBaseImageDigest]
		if !ok {
			return fmt.Errorf("annotation for base image is missing (%s or %s)%.0w", types.AnnotationBaseImageName, types.AnnotationBaseImageDigest, errs.ErrMissingAnnotation)
		}
		rNew, err := ref.New(baseName)
		if err != nil {
			return fmt.Errorf("failed to parse base name: %w", err)
		}
		dig, err := digest.Parse(baseDigest)
		if err != nil {
			return fmt.Errorf("failed to parse base digest: %w", err)
		}
		rOld := rNew
		rOld.Digest = dig.String()

		return rebaseAddStep(dc, rOld, rNew)
	}
}

// WithRebaseRefs swaps the base image layers from the old to the new reference.
func WithRebaseRefs(rOld, rNew ref.Ref) Opts {
	// cache old and new manifests, variable is nil until first pulled
	return func(dc *dagConfig, dm *dagManifest) error {
		return rebaseAddStep(dc, rOld, rNew)
	}
}

func rebaseAddStep(dc *dagConfig, rBaseOld, rBaseNew ref.Ref) error {
	var mbOldCache, mbNewCache manifest.Manifest
	dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, dm *dagManifest) error {
		// skip if manifest list or deleted
		if dm.m.IsList() || dm.mod == deleted || dm.config == nil {
			return nil
		}
		// get and cache base manifests
		var err error
		if mbOldCache == nil {
			mbOldCache, err = rc.ManifestGet(ctx, rBaseOld)
			if err != nil {
				return err
			}
		}
		if mbNewCache == nil {
			mbNewCache, err = rc.ManifestGet(ctx, rBaseNew)
			if err != nil {
				return err
			}
		}
		// if old and new are the same, skip rebase
		if mbOldCache.GetDescriptor().Equal(mbNewCache.GetDescriptor()) {
			return nil
		}
		// get the platform from the config, pull the relevant manifest
		oc := dm.config.oc.GetConfig()
		p := platform.Platform{
			OS:           oc.OS,
			Architecture: oc.Architecture,
			Variant:      oc.Variant,
			OSVersion:    oc.OSVersion,
			OSFeatures:   oc.OSFeatures,
			Features:     oc.OSFeatures,
		}
		mbOld := mbOldCache
		if mbOld.IsList() {
			d, err := manifest.GetPlatformDesc(mbOld, &p)
			if err != nil {
				return err
			}
			rp := rBaseOld
			rp.Digest = d.Digest.String()
			mbOld, err = rc.ManifestGet(ctx, rp)
			if err != nil {
				return err
			}
		}
		mbNew := mbNewCache
		if mbNew.IsList() {
			d, err := manifest.GetPlatformDesc(mbNew, &p)
			if err != nil {
				return err
			}
			rp := rBaseNew
			rp.Digest = d.Digest.String()
			mbNew, err = rc.ManifestGet(ctx, rp)
			if err != nil {
				return err
			}
		}
		// load layers and config from image and old/new base images
		imageOld, ok := mbOld.(manifest.Imager)
		if !ok {
			return fmt.Errorf("base original image is not an image")
		}
		layersOld, err := imageOld.GetLayers()
		if err != nil {
			return err
		}
		cdOld, err := imageOld.GetConfig()
		if err != nil {
			return err
		}
		confOld, err := rc.BlobGetOCIConfig(ctx, rBaseOld, cdOld)
		if err != nil {
			return err
		}
		confOCIOld := confOld.GetConfig()

		imageNew, ok := mbNew.(manifest.Imager)
		if !ok {
			return fmt.Errorf("base new image is not an image")
		}
		layersNew, err := imageNew.GetLayers()
		if err != nil {
			return err
		}
		cdNew, err := imageNew.GetConfig()
		if err != nil {
			return err
		}
		confNew, err := rc.BlobGetOCIConfig(ctx, rBaseNew, cdNew)
		if err != nil {
			return err
		}
		confOCINew := confNew.GetConfig()

		mi, ok := dm.m.(manifest.Imager)
		if !ok {
			return fmt.Errorf("manifest is not an image")
		}
		layers, err := mi.GetLayers()
		if err != nil {
			return err
		}
		conf := dm.config.oc
		confOCI := conf.GetConfig()

		// validate current base
		if len(layersOld) > len(layers) {
			return fmt.Errorf("base image has more layers than modified image%.0w", errs.ErrMismatch)
		}
		for i := range layersOld {
			if !layers[i].Same(layersOld[i]) {
				return fmt.Errorf("old base image does not match image layers, layer %d, base %v, image %v%.0w", i, layersOld[i], layers[i], errs.ErrMismatch)
			}
		}
		if len(confOCIOld.History) > len(confOCI.History) {
			return fmt.Errorf("base image has more history entries than modified image%.0w", errs.ErrMismatch)
		}
		historyLayers := 0
		for i := range confOCIOld.History {
			if confOCI.History[i].Author != confOCIOld.History[i].Author ||
				confOCI.History[i].Comment != confOCIOld.History[i].Comment ||
				!confOCI.History[i].Created.Equal(*confOCIOld.History[i].Created) ||
				confOCI.History[i].CreatedBy != confOCIOld.History[i].CreatedBy ||
				confOCI.History[i].EmptyLayer != confOCIOld.History[i].EmptyLayer {
				return fmt.Errorf("old base image does not match image history, entry %d, base %v, image %v%.0w", i, confOCIOld.History[i], confOCI.History[i], errs.ErrMismatch)
			}
			if !confOCIOld.History[i].EmptyLayer {
				historyLayers++
			}
		}
		if len(layersOld) != historyLayers || len(layersOld) != len(confOCIOld.RootFS.DiffIDs) {
			return fmt.Errorf("old base image layer count doesn't match history%.0w", errs.ErrMismatch)
		}
		if len(confOCIOld.RootFS.DiffIDs) > len(confOCI.RootFS.DiffIDs) {
			return fmt.Errorf("base image has more rootfs entries than modified image%.0w", errs.ErrMismatch)
		}
		for i := range confOCIOld.RootFS.DiffIDs {
			if confOCI.RootFS.DiffIDs[i] != confOCIOld.RootFS.DiffIDs[i] {
				return fmt.Errorf("old base image does not match image rootfs, entry %d, base %s, image %s%.0w", i, confOCIOld.RootFS.DiffIDs[i].String(), confOCI.RootFS.DiffIDs[i].String(), errs.ErrMismatch)
			}
		}
		// validate new base
		historyLayers = 0
		for i := range confOCINew.History {
			if !confOCINew.History[i].EmptyLayer {
				historyLayers++
			}
		}
		if len(layersNew) != historyLayers || len(confOCINew.RootFS.DiffIDs) != historyLayers {
			return fmt.Errorf("new base image config history doesn't match layer count")
		}

		// delete the old layers and config entries from vars and dag
		pruneNum := len(layersOld)
		i := 0
		for pruneNum > 0 {
			if dm.layers[i].mod == added {
				i++
				continue
			}
			if i == 0 {
				dm.layers = dm.layers[1:]
			} else if i >= len(dm.layers)-1 {
				dm.layers = dm.layers[:i]
			} else {
				dm.layers = append(dm.layers[:i], dm.layers[i+1:]...)
			}
			pruneNum--
		}
		layers = layers[len(layersOld):]
		confOCI.History = confOCI.History[len(confOCIOld.History):]
		confOCI.RootFS.DiffIDs = confOCI.RootFS.DiffIDs[len(confOCIOld.RootFS.DiffIDs):]

		// insert new layers and config entries in vars and dag, mark as modified
		dagAdd := []*dagLayer{}
		for i, l := range layersNew {
			dagAdd = append(dagAdd, &dagLayer{
				mod:      unchanged,
				ucDigest: confOCINew.RootFS.DiffIDs[i],
				desc:     l,
				rSrc:     rBaseNew,
			})
		}
		dm.layers = append(dagAdd, dm.layers...)
		layers = append(layersNew, layers...)
		err = mi.SetLayers(layers)
		if err != nil {
			return err
		}
		confOCI.History = append(confOCINew.History, confOCI.History...)
		confOCI.RootFS.DiffIDs = append(confOCINew.RootFS.DiffIDs, confOCI.RootFS.DiffIDs...)
		dm.config.oc.SetConfig(confOCI)
		dm.config.newDesc = dm.config.oc.GetDescriptor()

		// set modified flags on config and manifest
		dm.config.modified = true
		if dm.mod == unchanged {
			dm.mod = replaced
		}
		dc.forceLayerWalk = true

		return nil
	})
	return nil
}
