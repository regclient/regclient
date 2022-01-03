package ocidir

import (
	"context"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
)

func TestTag(t *testing.T) {
	ctx := context.Background()
	exTags := []string{"latest", "v0.3", "v0.3.10"}
	f := rwfs.OSNew("")
	o := New(WithFS(f))
	tRef := "ocidir://testdata/regctl"
	r, err := ref.New(tRef)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", tRef, err)
	}
	tl, err := o.TagList(ctx, r)
	if err != nil {
		t.Errorf("failed to retrieve tag list: %v", err)
		return
	}
	tlTags, err := tl.GetTags()
	if err != nil {
		t.Errorf("failed to get tags: %v", err)
	}
	if !cmpSliceString(exTags, tlTags) {
		t.Errorf("unexpected tag list, expected %v, received %v", exTags, tlTags)
	}
}
