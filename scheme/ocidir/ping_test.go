package ocidir

import (
	"context"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types/ref"
)

func TestPing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := rwfs.OSNew("")
	o := New(WithFS(f))
	rOkay, err := ref.NewHost("ocidir://testdata/regctl")
	if err != nil {
		t.Errorf("failed to create ref: %v", err)
		return
	}
	result, err := o.Ping(ctx, rOkay)
	if err != nil {
		t.Errorf("failed to ping: %v", err)
		return
	}
	if result.Header != nil {
		t.Errorf("header is not nil")
	}
	if result.Stat == nil {
		t.Errorf("stat is nil")
	} else {
		if !result.Stat.IsDir() {
			t.Errorf("stat is not a directory")
		}
	}

	rMissing, err := ref.NewHost("ocidir://testdata/missing")
	if err != nil {
		t.Errorf("failed to create ref: %v", err)
		return
	}
	result, err = o.Ping(ctx, rMissing)
	if err == nil {
		t.Errorf("ping to missing directory succeeded")
	}
	if result.Header != nil {
		t.Errorf("header is not nil")
	}
	if result.Stat != nil {
		t.Errorf("stat on missing is not nil")
	}

	rFile, err := ref.NewHost("ocidir://testdata/regctl/index.json")
	if err != nil {
		t.Errorf("failed to create ref: %v", err)
		return
	}
	result, err = o.Ping(ctx, rFile)
	if err == nil {
		t.Errorf("ping to a file did not fail")
		return
	}
	if result.Header != nil {
		t.Errorf("header is not nil")
	}
	if result.Stat == nil {
		t.Errorf("stat to a file is nil")
	} else {
		if result.Stat.IsDir() {
			t.Errorf("stat of a file is a directory")
		}
	}
}
