package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/spf13/cobra"
)

var indexKnownTypes = []string{
	types.MediaTypeOCI1ManifestList,
	types.MediaTypeDocker2ManifestList,
}

var indexCmd = &cobra.Command{
	Use:   "index <cmd>",
	Short: "manage manifest lists and OCI index",
}

var indexAddCmd = &cobra.Command{
	Use:       "add <image_ref>",
	Aliases:   []string{"append", "insert"},
	Short:     "add an index entry",
	Long:      `Add an entry to a manifest list or OCI Index.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete digests
	RunE:      runIndexAdd,
}

var indexCreateCmd = &cobra.Command{
	Use:       "create <image_ref>",
	Aliases:   []string{"init", "new"},
	Short:     "create an index",
	Long:      `Create a manifest list or OCI Index.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete digests
	RunE:      runIndexCreate,
}

var indexDeleteCmd = &cobra.Command{
	Use:       "delete <image_ref>",
	Aliases:   []string{"del", "rm", "remove"},
	Short:     "delete an index entry",
	Long:      `Delete an entry from a manifest list or OCI Index.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{}, // do not auto complete digests
	RunE:      runIndexDelete,
}

var indexOpts struct {
	annotations     []string
	byDigest        bool
	descAnnotations []string
	descPlatform    string
	digests         []string
	format          string
	incDigestTags   bool
	incReferrers    bool
	mediaType       string
	platforms       []string
	refs            []string
}

func init() {
	indexAddCmd.Flags().StringArrayVarP(&indexOpts.descAnnotations, "desc-annotation", "", []string{}, "Annotation to add to descriptors of new entries")
	indexAddCmd.Flags().StringVarP(&indexOpts.descPlatform, "desc-platform", "", "", "Platform to set in descriptors of new entries")
	indexAddCmd.Flags().StringArrayVarP(&indexOpts.digests, "digest", "", []string{}, "Digest to add")
	indexAddCmd.Flags().BoolVar(&indexOpts.incDigestTags, "digest-tags", false, "Include digest tags")
	indexAddCmd.Flags().BoolVar(&indexOpts.incReferrers, "referrers", false, "Include referrers")
	indexAddCmd.Flags().StringArrayVarP(&indexOpts.refs, "ref", "", []string{}, "References to add")
	indexAddCmd.Flags().StringArrayVarP(&indexOpts.platforms, "platform", "", []string{}, "Platforms to include from ref")

	indexCreateCmd.Flags().StringArrayVarP(&indexOpts.annotations, "annotation", "", []string{}, "Annotation to set on manifest")
	indexCreateCmd.Flags().BoolVarP(&indexOpts.byDigest, "by-digest", "", false, "Push manifest by digest instead of tag")
	indexCreateCmd.Flags().StringArrayVarP(&indexOpts.descAnnotations, "desc-annotation", "", []string{}, "Annotation to add to descriptors of new entries")
	indexCreateCmd.Flags().StringVarP(&indexOpts.descPlatform, "desc-platform", "", "", "Platform to set in descriptors of new entries")
	indexCreateCmd.Flags().StringArrayVarP(&indexOpts.digests, "digest", "", []string{}, "Digest to include in new index")
	indexCreateCmd.Flags().StringVarP(&indexOpts.format, "format", "", "", "Format output with go template syntax")
	indexCreateCmd.Flags().BoolVar(&indexOpts.incDigestTags, "digest-tags", false, "Include digest tags")
	indexCreateCmd.Flags().BoolVar(&indexOpts.incReferrers, "referrers", false, "Include referrers")
	indexCreateCmd.Flags().StringVarP(&indexOpts.mediaType, "media-type", "m", types.MediaTypeOCI1ManifestList, "Media-type for manifest list or OCI Index")
	indexCreateCmd.Flags().StringArrayVarP(&indexOpts.refs, "ref", "", []string{}, "References to include in new index")
	indexCreateCmd.Flags().StringArrayVarP(&indexOpts.platforms, "platform", "", []string{}, "Platforms to include from ref")
	indexCreateCmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return indexKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})

	indexDeleteCmd.Flags().StringArrayVarP(&indexOpts.digests, "digest", "", []string{}, "Digest to delete")
	indexDeleteCmd.Flags().StringArrayVarP(&indexOpts.platforms, "platform", "", []string{}, "Platform to delete")

	indexCmd.AddCommand(indexAddCmd)
	indexCmd.AddCommand(indexCreateCmd)
	indexCmd.AddCommand(indexDeleteCmd)
	rootCmd.AddCommand(indexCmd)
}

func runIndexAdd(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// parse ref
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	// setup regclient
	rc := newRegClient()
	defer rc.Close(ctx, r)

	// pull existing index
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}
	mi, ok := m.(manifest.Indexer)
	if !ok {
		return fmt.Errorf("current manifest is not an index/manifest list, \"%s\": %w", m.GetDescriptor().MediaType, types.ErrUnsupportedMediaType)
	}
	curDesc, err := mi.GetManifestList()
	if err != nil {
		return err
	}

	// generate a list of descriptors from CLI args
	descList, err := indexBuildDescList(ctx, rc, r)
	if err != nil {
		return err
	}

	// append list
	curDesc = append(curDesc, descList...)
	curDesc = indexDescListRmDup(curDesc)
	err = mi.SetManifestList(curDesc)
	if err != nil {
		return err
	}

	// push the index
	if r.Tag == "" && r.Digest != "" {
		r.Digest = m.GetDescriptor().Digest.String()
	}
	err = rc.ManifestPut(ctx, r, m)
	if err != nil {
		return err
	}

	// format output
	result := struct {
		Manifest manifest.Manifest
	}{
		Manifest: m,
	}
	if r.Tag == "" && r.Digest != "" && indexOpts.format == "" {
		indexOpts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(os.Stdout, indexOpts.format, result)
}

func runIndexCreate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// validate media type
	if indexOpts.mediaType != types.MediaTypeOCI1ManifestList && indexOpts.mediaType != types.MediaTypeDocker2ManifestList {
		return fmt.Errorf("unsupported manifest media type: %s%.0w", indexOpts.mediaType, types.ErrUnsupportedMediaType)
	}

	// parse ref
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	// setup regclient
	rc := newRegClient()
	defer rc.Close(ctx, r)

	// parse annotations
	annotations := map[string]string{}
	for _, a := range indexOpts.annotations {
		aSplit := strings.SplitN(a, "=", 2)
		if len(aSplit) == 1 {
			annotations[aSplit[0]] = ""
		} else {
			annotations[aSplit[0]] = aSplit[1]
		}
	}

	// generate a list of descriptors from CLI args
	descList, err := indexBuildDescList(ctx, rc, r)
	if err != nil {
		return err
	}
	descList = indexDescListRmDup(descList)

	// build the index
	mOpts := []manifest.Opts{}
	switch indexOpts.mediaType {
	case types.MediaTypeOCI1ManifestList:
		m := v1.Index{
			Versioned: v1.IndexSchemaVersion,
			MediaType: types.MediaTypeOCI1ManifestList,
			Manifests: descList,
		}
		if len(annotations) > 0 {
			m.Annotations = annotations
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	case types.MediaTypeDocker2ManifestList:
		m := schema2.ManifestList{
			Versioned: schema2.ManifestListSchemaVersion,
			Manifests: descList,
		}
		if len(annotations) > 0 {
			m.Annotations = annotations
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	}
	mm, err := manifest.New(mOpts...)
	if err != nil {
		return err
	}

	// push the index
	if indexOpts.byDigest {
		r.Tag = ""
		r.Digest = mm.GetDescriptor().Digest.String()
	}
	err = rc.ManifestPut(ctx, r, mm)
	if err != nil {
		return err
	}

	// format output
	result := struct {
		Manifest manifest.Manifest
	}{
		Manifest: mm,
	}
	if indexOpts.byDigest && indexOpts.format == "" {
		indexOpts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(os.Stdout, indexOpts.format, result)
}

func runIndexDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// parse ref
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	// setup regclient
	rc := newRegClient()
	defer rc.Close(ctx, r)

	// pull existing index
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}
	mi, ok := m.(manifest.Indexer)
	if !ok {
		return fmt.Errorf("current manifest is not an index/manifest list, \"%s\": %w", m.GetDescriptor().MediaType, types.ErrUnsupportedMediaType)
	}
	curDesc, err := mi.GetManifestList()
	if err != nil {
		return err
	}

	// for each CLI arg, find and delete matching entries
	for _, dig := range indexOpts.digests {
		i := len(curDesc) - 1
		for i >= 0 {
			if curDesc[i].Digest.String() == dig {
				if i < len(curDesc)-1 {
					curDesc = append(curDesc[:i], curDesc[i+1:]...)
				} else {
					curDesc = curDesc[:i]
				}
			}
			i--
		}
	}
	for _, platStr := range indexOpts.platforms {
		plat, err := platform.Parse(platStr)
		if err != nil {
			return err
		}
		i := len(curDesc) - 1
		for i >= 0 {
			if curDesc[i].Platform != nil && platform.Match(plat, *curDesc[i].Platform) {
				if i < len(curDesc)-1 {
					curDesc = append(curDesc[:i], curDesc[i+1:]...)
				} else {
					curDesc = curDesc[:i]
				}
			}
			i--
		}
	}

	// update manifest
	err = mi.SetManifestList(curDesc)
	if err != nil {
		return err
	}

	// push the index
	if r.Tag == "" && r.Digest != "" {
		r.Digest = m.GetDescriptor().Digest.String()
	}
	err = rc.ManifestPut(ctx, r, m)
	if err != nil {
		return err
	}

	// format output
	result := struct {
		Manifest manifest.Manifest
	}{
		Manifest: m,
	}
	if r.Tag == "" && r.Digest != "" && indexOpts.format == "" {
		indexOpts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(os.Stdout, indexOpts.format, result)
}

func indexBuildDescList(ctx context.Context, rc *regclient.RegClient, r ref.Ref) ([]types.Descriptor, error) {
	imgCopyOpts := []regclient.ImageOpts{
		regclient.ImageWithChild(),
	}
	if indexOpts.incDigestTags {
		imgCopyOpts = append(imgCopyOpts, regclient.ImageWithDigestTags())
	}
	if indexOpts.incReferrers {
		imgCopyOpts = append(imgCopyOpts, regclient.ImageWithReferrers())
	}

	descAnnotations := map[string]string{}
	for _, a := range indexOpts.descAnnotations {
		aSplit := strings.SplitN(a, "=", 2)
		if len(aSplit) == 1 {
			descAnnotations[aSplit[0]] = ""
		} else {
			descAnnotations[aSplit[0]] = aSplit[1]
		}
	}
	platforms := []platform.Platform{}
	for _, pStr := range indexOpts.platforms {
		p, err := platform.Parse(pStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse platform %s: %w", pStr, err)
		}
		platforms = append(platforms, p)
	}

	// copy each ref by digest to the destination repository
	if indexOpts.digests == nil {
		indexOpts.digests = []string{}
	}
	for _, rStr := range indexOpts.refs {
		srcRef, err := ref.New(rStr)
		if err != nil {
			return nil, err
		}
		mCopy, err := rc.ManifestHead(ctx, srcRef, regclient.WithManifestRequireDigest())
		if err != nil {
			return nil, err
		}
		if !mCopy.IsList() || len(platforms) == 0 {
			// single manifest
			desc := mCopy.GetDescriptor()
			tgtRef := r
			tgtRef.Tag = ""
			tgtRef.Digest = desc.Digest.String()
			err = rc.ImageCopy(ctx, srcRef, tgtRef, imgCopyOpts...)
			if err != nil {
				return nil, err
			}
			indexOpts.digests = append(indexOpts.digests, desc.Digest.String())
		} else {
			// platform specific descriptors are being extracted from a manifest list
			mCopy, err = rc.ManifestGet(ctx, srcRef)
			if err != nil {
				return nil, err
			}
			mi, ok := mCopy.(manifest.Indexer)
			if !ok {
				return nil, fmt.Errorf("manifest list is not an Indexer")
			}
			dl, err := mi.GetManifestList()
			if err != nil {
				return nil, fmt.Errorf("failed to get descriptor list: %w", err)
			}
			for _, d := range dl {
				if d.Platform != nil && indexPlatformInList(*d.Platform, platforms) {
					dRef := srcRef
					dRef.Tag = ""
					dRef.Digest = d.Digest.String()
					tgtRef := r
					tgtRef.Tag = ""
					tgtRef.Digest = d.Digest.String()
					err = rc.ImageCopy(ctx, dRef, tgtRef, imgCopyOpts...)
					if err != nil {
						return nil, err
					}
					indexOpts.digests = append(indexOpts.digests, d.Digest.String())
				}
			}
		}
	}

	// parse each digest, pull manifest, get config, append to list of descriptors
	descList := []types.Descriptor{}
	for _, dig := range indexOpts.digests {
		rDig := r
		rDig.Tag = ""
		rDig.Digest = dig

		mDig, err := getManifest(ctx, rc, rDig)
		if err != nil {
			return nil, err
		}
		desc := mDig.GetDescriptor()
		plat := &platform.Platform{}
		if indexOpts.descPlatform != "" {
			*plat, err = platform.Parse(indexOpts.descPlatform)
		} else {
			plat, err = indexGetPlatform(ctx, rc, rDig, mDig)
		}
		if err == nil {
			desc.Platform = plat
		}
		if len(descAnnotations) > 0 && desc.Annotations == nil {
			desc.Annotations = map[string]string{}
		}
		for k, v := range descAnnotations {
			desc.Annotations[k] = v
		}
		descList = append(descList, desc)
	}
	return descList, nil
}

func indexGetPlatform(ctx context.Context, rc *regclient.RegClient, r ref.Ref, m manifest.Manifest) (*platform.Platform, error) {
	if mi, ok := m.(manifest.Imager); ok {
		cd, err := mi.GetConfig()
		if err != nil {
			return nil, err
		}
		blobConfig, err := rc.BlobGetOCIConfig(ctx, r, cd)
		if err != nil {
			return nil, err
		}
		ociConfig := blobConfig.GetConfig()
		if ociConfig.OS == "" {
			return nil, nil
		}
		plat := platform.Platform{
			OS:           ociConfig.OS,
			Architecture: ociConfig.Architecture,
			OSVersion:    ociConfig.OSVersion,
			OSFeatures:   ociConfig.OSFeatures,
			Variant:      ociConfig.Variant,
			Features:     ociConfig.OSFeatures,
		}
		return &plat, nil
	}
	return nil, nil
}

func indexDescListRmDup(dl []types.Descriptor) []types.Descriptor {
	i := 0
	for i < len(dl)-1 {
		j := len(dl) - 1
		for j > i {
			if dl[i].Equal(dl[j]) {
				if j < len(dl)-1 {
					dl = append(dl[:j], dl[j+1:]...)
				} else {
					dl = dl[:j]
				}
			}
			j--
		}
		i++
	}
	return dl
}

func indexPlatformInList(p platform.Platform, pl []platform.Platform) bool {
	for _, cur := range pl {
		if platform.Match(p, cur) {
			return true
		}
	}
	return false
}
