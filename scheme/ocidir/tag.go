package ocidir

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"github.com/regclient/regclient/types/tag"
)

// TagDelete removes a tag from the repository
func (o *OCIDir) TagDelete(ctx context.Context, r ref.Ref) error {

	return types.ErrNotImplemented
}

// TagList returns a list of tags from the repository
func (o *OCIDir) TagList(ctx context.Context, r ref.Ref, opts ...scheme.TagOpts) (*tag.TagList, error) {
	// get index
	index, err := o.readIndex(r)
	if err != nil {
		return nil, err
	}
	tl := []string{}
	for _, desc := range index.Manifests {
		if t, ok := desc.Annotations[aRefName]; ok && !strings.Contains(t, ":") {
			tl = append(tl, t)
		}
	}
	sort.Strings(tl)
	ib, err := json.Marshal(index)
	if err != nil {
		return nil, err
	}
	// return listing from index
	t, err := tag.New(
		tag.WithRaw(ib),
		tag.WithRef(r),
		tag.WithMT(types.MediaTypeOCI1ManifestList),
		tag.WithTags(tl),
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}
