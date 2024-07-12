package ocidir

import (
	"context"
	"testing"

	"github.com/regclient/regclient/types/ref"
)

func TestPing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	o := New()
	rOkay, err := ref.NewHost("ocidir://../../testdata/testrepo")
	if err != nil {
		t.Fatalf("failed to create ref: %v", err)
	}
	result, err := o.Ping(ctx, rOkay)
	if err != nil {
		t.Fatalf("failed to ping: %v", err)
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

	rMissing, err := ref.NewHost("ocidir://../../testdata/missing")
	if err != nil {
		t.Fatalf("failed to create ref: %v", err)
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

	rFile, err := ref.NewHost("ocidir://../../testdata/testrepo/index.json")
	if err != nil {
		t.Fatalf("failed to create ref: %v", err)
	}
	result, err = o.Ping(ctx, rFile)
	if err == nil {
		t.Fatalf("ping to a file did not fail")
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
