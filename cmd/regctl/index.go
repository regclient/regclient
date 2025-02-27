package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/docker/schema2"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/warning"
)

var indexKnownTypes = []string{
	mediatype.OCI1ManifestList,
	mediatype.Docker2ManifestList,
}

type indexOpts struct {
	rootOpts        *rootOpts
	annotations     []string
	artifactType    string
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
	subject         string
}

func NewIndexCmd(rOpts *rootOpts) *cobra.Command {
	var indexCmd = &cobra.Command{
		Use:   "index <cmd>",
		Short: "manage manifest lists and OCI index",
	}
	indexCmd.AddCommand(newIndexAddCmd(rOpts))
	indexCmd.AddCommand(newIndexCreateCmd(rOpts))
	indexCmd.AddCommand(newIndexDeleteCmd(rOpts))
	return indexCmd
}

func newIndexAddCmd(rOpts *rootOpts) *cobra.Command {
	opts := indexOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "add <image_ref>",
		Aliases: []string{"append", "insert"},
		Short:   "add an index entry",
		Long:    `Add an entry to a manifest list or OCI Index.`,
		Example: `
# add arm64 to the v1 image
regctl index add registry.example.org/repo:v1 --ref registry.example.org/repo:arm64`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete digests
		RunE:      opts.runIndexAdd,
	}
	cmd.Flags().StringArrayVar(&opts.descAnnotations, "desc-annotation", []string{}, "Annotation to add to descriptors of new entries")
	cmd.Flags().StringVar(&opts.descPlatform, "desc-platform", "", "Platform to set in descriptors of new entries")
	cmd.Flags().StringArrayVar(&opts.digests, "digest", []string{}, "Digest to add")
	cmd.Flags().BoolVar(&opts.incDigestTags, "digest-tags", false, "Include digest tags")
	cmd.Flags().BoolVar(&opts.incReferrers, "referrers", false, "Include referrers")
	cmd.Flags().StringArrayVar(&opts.refs, "ref", []string{}, "References to add")
	cmd.Flags().StringArrayVar(&opts.platforms, "platform", []string{}, "Platforms to include from ref")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	return cmd
}

func newIndexCreateCmd(rOpts *rootOpts) *cobra.Command {
	opts := indexOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "create <image_ref>",
		Aliases: []string{"init", "new", "put"},
		Short:   "create an index",
		Long:    `Create a manifest list or OCI Index.`,
		Example: `
# create an empty index
regctl index create registry.example.org/repo:v1

# create an index from the amd64 and arm64 platforms
regctl index create registry.example.org/alpine:latest \
  --ref alpine:latest --platform linux/amd64 --platform linux/arm64

# create a docker manifest list
regctl index create registry.example.org/busybox:1.34 \
  --media-type application/vnd.docker.distribution.manifest.list.v2+json \
  --ref busybox:1.34 --platform linux/amd64 --platform linux/arm64

# create an index of windows images
regctl index create registry.example.org/library/golang:windows \
  --ref golang:latest \
	--platform windows/amd64,osver=10.0.20348.2322 \
	--platform windows/amd64,osver=10.0.17763.5458`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete digests
		RunE:      opts.runIndexCreate,
	}
	cmd.Flags().StringArrayVar(&opts.annotations, "annotation", []string{}, "Annotation to set on manifest")
	cmd.Flags().StringVar(&opts.artifactType, "artifact-type", "", "Include an artifactType value")
	cmd.Flags().BoolVar(&opts.byDigest, "by-digest", false, "Push manifest by digest instead of tag")
	cmd.Flags().StringArrayVar(&opts.descAnnotations, "desc-annotation", []string{}, "Annotation to add to descriptors of new entries")
	cmd.Flags().StringVar(&opts.descPlatform, "desc-platform", "", "Platform to set in descriptors of new entries")
	cmd.Flags().StringArrayVar(&opts.digests, "digest", []string{}, "Digest to include in new index")
	cmd.Flags().BoolVar(&opts.incDigestTags, "digest-tags", false, "Include digest tags")
	cmd.Flags().StringVar(&opts.format, "format", "", "Format output with go template syntax")
	_ = cmd.RegisterFlagCompletionFunc("format", completeArgNone)
	cmd.Flags().StringVarP(&opts.mediaType, "media-type", "m", mediatype.OCI1ManifestList, "Media-type for manifest list or OCI Index")
	_ = cmd.RegisterFlagCompletionFunc("media-type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return indexKnownTypes, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringArrayVar(&opts.platforms, "platform", []string{}, "Platforms to include from ref")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	cmd.Flags().StringArrayVar(&opts.refs, "ref", []string{}, "References to include in new index")
	cmd.Flags().BoolVar(&opts.incReferrers, "referrers", false, "Include referrers")
	cmd.Flags().StringVar(&opts.subject, "subject", "", "Specify a subject tag or digest (this manifest must already exist in the repo)")
	return cmd
}

func newIndexDeleteCmd(rOpts *rootOpts) *cobra.Command {
	opts := indexOpts{
		rootOpts: rOpts,
	}
	cmd := &cobra.Command{
		Use:     "delete <image_ref>",
		Aliases: []string{"del", "rm", "remove"},
		Short:   "delete an index entry",
		Long:    `Delete an entry from a manifest list or OCI Index.`,
		Example: `
# remove the several platforms from an image
regctl index delete registry.example.org/repo:v1 \
  --platform unknown/unknown --platform linux/s390x \
  --platform linux/ppc64le --platform linux/mips64le`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{}, // do not auto complete digests
		RunE:      opts.runIndexDelete,
	}
	cmd.Flags().StringArrayVar(&opts.digests, "digest", []string{}, "Digest to delete")
	cmd.Flags().StringArrayVar(&opts.platforms, "platform", []string{}, "Platform to delete")
	_ = cmd.RegisterFlagCompletionFunc("platform", completeArgPlatform)
	return cmd
}

func (opts *indexOpts) runIndexAdd(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	// parse ref
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	// setup regclient
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	// pull existing index
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}
	mi, ok := m.(manifest.Indexer)
	if !ok {
		return fmt.Errorf("current manifest is not an index/manifest list, \"%s\": %w", m.GetDescriptor().MediaType, errs.ErrUnsupportedMediaType)
	}
	curDesc, err := mi.GetManifestList()
	if err != nil {
		return err
	}

	// generate a list of descriptors from CLI args
	descList, err := opts.indexBuildDescList(ctx, rc, r)
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
	if r.Digest != "" {
		r = r.AddDigest(m.GetDescriptor().Digest.String())
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
	if r.Tag == "" && r.Digest != "" && opts.format == "" {
		opts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, result)
}

func (opts *indexOpts) runIndexCreate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	// validate media type
	if opts.mediaType != mediatype.OCI1ManifestList && opts.mediaType != mediatype.Docker2ManifestList {
		return fmt.Errorf("unsupported manifest media type: %s%.0w", opts.mediaType, errs.ErrUnsupportedMediaType)
	}

	// parse ref
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	// setup regclient
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	// parse annotations
	annotations := map[string]string{}
	for _, a := range opts.annotations {
		aSplit := strings.SplitN(a, "=", 2)
		if len(aSplit) == 1 {
			annotations[aSplit[0]] = ""
		} else {
			annotations[aSplit[0]] = aSplit[1]
		}
	}

	// generate a list of descriptors from CLI args
	descList, err := opts.indexBuildDescList(ctx, rc, r)
	if err != nil {
		return err
	}
	descList = indexDescListRmDup(descList)

	var subj *descriptor.Descriptor
	if opts.subject != "" && opts.mediaType == mediatype.OCI1ManifestList {
		var rSubj ref.Ref
		dig, err := digest.Parse(opts.subject)
		if err == nil {
			rSubj = r.SetDigest(dig.String())
		} else {
			rSubj = r.SetTag(opts.subject)
		}
		mSubj, err := rc.ManifestHead(ctx, rSubj, regclient.WithManifestRequireDigest())
		if err != nil {
			return fmt.Errorf("failed to lookup subject %s: %w", rSubj.CommonName(), err)
		}
		desc := mSubj.GetDescriptor()
		desc.Annotations = nil
		subj = &desc
	}

	// build the index
	mOpts := []manifest.Opts{}
	switch opts.mediaType {
	case mediatype.OCI1ManifestList:
		m := v1.Index{
			Versioned:    v1.IndexSchemaVersion,
			MediaType:    mediatype.OCI1ManifestList,
			ArtifactType: opts.artifactType,
			Manifests:    descList,
			Subject:      subj,
		}
		if len(annotations) > 0 {
			m.Annotations = annotations
		}
		mOpts = append(mOpts, manifest.WithOrig(m))
	case mediatype.Docker2ManifestList:
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
	if opts.byDigest {
		r = r.SetDigest(mm.GetDescriptor().Digest.String())
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
	if opts.byDigest && opts.format == "" {
		opts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, result)
}

func (opts *indexOpts) runIndexDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// dedup warnings
	if w := warning.FromContext(ctx); w == nil {
		ctx = warning.NewContext(ctx, &warning.Warning{Hook: warning.DefaultHook()})
	}

	// parse ref
	r, err := ref.New(args[0])
	if err != nil {
		return err
	}

	// setup regclient
	rc := opts.rootOpts.newRegClient()
	defer rc.Close(ctx, r)

	// pull existing index
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		return err
	}
	mi, ok := m.(manifest.Indexer)
	if !ok {
		return fmt.Errorf("current manifest is not an index/manifest list, \"%s\": %w", m.GetDescriptor().MediaType, errs.ErrUnsupportedMediaType)
	}
	curDesc, err := mi.GetManifestList()
	if err != nil {
		return err
	}

	// for each CLI arg, find and delete matching entries
	for _, dig := range opts.digests {
		i := len(curDesc) - 1
		for i >= 0 {
			if curDesc[i].Digest.String() == dig {
				curDesc = slices.Delete(curDesc, i, i+1)
			}
			i--
		}
	}
	for _, platStr := range opts.platforms {
		plat, err := platform.Parse(platStr)
		if err != nil {
			return err
		}
		i := len(curDesc) - 1
		for i >= 0 {
			if curDesc[i].Platform != nil && platform.Match(plat, *curDesc[i].Platform) {
				curDesc = slices.Delete(curDesc, i, i+1)
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
	if r.Digest != "" {
		r = r.AddDigest(m.GetDescriptor().Digest.String())
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
	if r.Tag == "" && r.Digest != "" && opts.format == "" {
		opts.format = "{{ printf \"%s\\n\" .Manifest.GetDescriptor.Digest }}"
	}
	return template.Writer(cmd.OutOrStdout(), opts.format, result)
}

func (opts *indexOpts) indexBuildDescList(ctx context.Context, rc *regclient.RegClient, r ref.Ref) ([]descriptor.Descriptor, error) {
	imgCopyOpts := []regclient.ImageOpts{
		regclient.ImageWithChild(),
	}
	if opts.incDigestTags {
		imgCopyOpts = append(imgCopyOpts, regclient.ImageWithDigestTags())
	}
	if opts.incReferrers {
		imgCopyOpts = append(imgCopyOpts, regclient.ImageWithReferrers())
	}

	descAnnotations := map[string]string{}
	for _, a := range opts.descAnnotations {
		aSplit := strings.SplitN(a, "=", 2)
		if len(aSplit) == 1 {
			descAnnotations[aSplit[0]] = ""
		} else {
			descAnnotations[aSplit[0]] = aSplit[1]
		}
	}
	platforms := []platform.Platform{}
	for _, pStr := range opts.platforms {
		p, err := platform.Parse(pStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse platform %s: %w", pStr, err)
		}
		platforms = append(platforms, p)
	}

	// copy each ref by digest to the destination repository
	if opts.digests == nil {
		opts.digests = []string{}
	}
	for _, rStr := range opts.refs {
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
			tgtRef := r.SetDigest(desc.Digest.String())
			err = rc.ImageCopy(ctx, srcRef, tgtRef, imgCopyOpts...)
			if err != nil {
				return nil, err
			}
			opts.digests = append(opts.digests, desc.Digest.String())
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
					dRef := srcRef.SetDigest(d.Digest.String())
					tgtRef := r.SetDigest(d.Digest.String())
					err = rc.ImageCopy(ctx, dRef, tgtRef, imgCopyOpts...)
					if err != nil {
						return nil, err
					}
					opts.digests = append(opts.digests, d.Digest.String())
				}
			}
		}
	}

	// parse each digest, pull manifest, get config, append to list of descriptors
	descList := []descriptor.Descriptor{}
	for _, dig := range opts.digests {
		rDig := r.SetDigest(dig)
		mDig, err := rc.ManifestHead(ctx, rDig, regclient.WithManifestRequireDigest())
		if err != nil {
			return nil, err
		}
		desc := mDig.GetDescriptor()
		plat := &platform.Platform{}
		if opts.descPlatform != "" {
			*plat, err = platform.Parse(opts.descPlatform)
		} else {
			plat, err = indexGetPlatform(ctx, rc, rDig, mDig)
		}
		if err == nil {
			desc.Platform = plat
		}
		if len(descAnnotations) > 0 {
			desc.Annotations = map[string]string{}
		} else {
			desc.Annotations = nil
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
		if !m.IsSet() {
			// fetch the manifest if it wasn't already pulled
			m, err := rc.ManifestGet(ctx, r)
			if err != nil {
				return nil, err
			}
			if mi, ok = m.(manifest.Imager); !ok {
				return nil, nil
			}
		}
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
		return &ociConfig.Platform, nil
	}
	return nil, nil
}

func indexDescListRmDup(dl []descriptor.Descriptor) []descriptor.Descriptor {
	i := 0
	for i < len(dl)-1 {
		j := len(dl) - 1
		for j > i {
			if dl[i].Equal(dl[j]) {
				dl = slices.Delete(dl, j, j+1)
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
