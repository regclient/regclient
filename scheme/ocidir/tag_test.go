package ocidir

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/regclient/regclient/internal/copyfs"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

func TestTag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tempDir := t.TempDir()
	err := copyfs.Copy(filepath.Join(tempDir, "testrepo"), "../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to setup tempDir: %v", err)
	}
	o := New()
	tRef := "ocidir://" + tempDir + "/testrepo"
	r, err := ref.New(tRef)
	if err != nil {
		t.Fatalf("failed to parse ref %s: %v", tRef, err)
	}

	t.Run("TagList", func(t *testing.T) {
		exTags := []string{"a1", "a2", "ai", "b1", "b2", "b3", "child", "loop", "mirror", "v1", "v2", "v3"}
		tl, err := o.TagList(ctx, r)
		if err != nil {
			t.Fatalf("failed to retrieve tag list: %v", err)
		}
		tlTags, err := tl.GetTags()
		if err != nil {
			t.Fatalf("failed to get tags: %v", err)
		}
		for _, exTag := range exTags {
			if !slices.Contains(tlTags, exTag) {
				t.Errorf("missing tag: %s", exTag)
			}
		}
	})

	t.Run("TagDelete", func(t *testing.T) {
		keepTags := []string{"a2", "ai", "b1", "b2", "b3", "child", "loop", "v2", "v3"}
		rmTags := []string{"mirror", "a1", "v1"}
		rCp := r.SetTag("missing")
		err := o.TagDelete(ctx, rCp)
		if err == nil || !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("deleting missing tag %s: %v", rCp.CommonName(), err)
		}
		for _, rmTag := range rmTags {
			r := r.SetTag(rmTag)
			err = o.TagDelete(ctx, r)
			if err != nil {
				t.Errorf("failed to delete tag %s: %v", r.CommonName(), err)
			}
		}
		tl, err := o.TagList(ctx, r)
		if err != nil {
			t.Fatalf("failed to retrieve tag list: %v", err)
		}
		tlTags, err := tl.GetTags()
		if err != nil {
			t.Errorf("failed to get tags: %v", err)
		}
		for _, keep := range keepTags {
			if !slices.Contains(tlTags, keep) {
				t.Errorf("missing tag: %s", keep)
			}
		}
		for _, rm := range rmTags {
			if slices.Contains(tlTags, rm) {
				t.Errorf("tag not removed: %s", rm)
			}
		}
	})
}
