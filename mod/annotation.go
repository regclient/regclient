package mod

import (
	"context"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

// WithAnnotation adds an annotation, or deletes it if the value is set to an empty string
func WithAnnotation(name, value string) Opts {
	return func(dc *dagConfig) {
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, dm *dagManifest) error {
			var err error
			changed := false
			// only annotate top manifest
			if dm.mod == deleted || !dm.top {
				return nil
			}
			om := dm.m.GetOrig()
			if dm.m.IsList() {
				ociI, err := manifest.OCIIndexFromAny(om)
				if err != nil {
					return err
				}
				if ociI.Annotations == nil {
					ociI.Annotations = map[string]string{}
				}
				cur, ok := ociI.Annotations[name]
				if value == "" && ok {
					delete(ociI.Annotations, name)
					changed = true
				} else if value != "" && value != cur {
					ociI.Annotations[name] = value
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
				cur, ok := ociM.Annotations[name]
				if value == "" && ok {
					delete(ociM.Annotations, name)
					changed = true
				} else if value != "" && value != cur {
					ociM.Annotations[name] = value
					changed = true
				}
				err = manifest.OCIManifestToAny(ociM, &om)
				if err != nil {
					return err
				}
			}
			if changed {
				dm.mod = replaced
				err = dm.m.SetOrig(om)
				if err != nil {
					return err
				}
				dm.newDesc = dm.m.GetDescriptor()
			}
			return nil
		})
	}
}

// WithAnnotationOCIBase adds annotations for the base image
func WithAnnotationOCIBase(rBase ref.Ref, dBase digest.Digest) Opts {
	return func(dc *dagConfig) {
		dc.stepsManifest = append(dc.stepsManifest, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, dm *dagManifest) error {
			if dm.mod == deleted || !dm.top {
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
				dm.mod = replaced
				err := dm.m.SetOrig(om)
				if err != nil {
					return err
				}
				dm.newDesc = dm.m.GetDescriptor()
			}
			return nil
		})
	}
}

// WithLabelToAnnotation copies image config labels to manifest annotations
func WithLabelToAnnotation() Opts {
	return func(dc *dagConfig) {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, r ref.Ref, dm *dagManifest) error {
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
			dm.mod = replaced
			return nil
		})
	}
}
