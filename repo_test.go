package regclient

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/errs"
)

func TestRepoList(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := New(
		WithSlog(log),
		WithRegOpts(reg.WithDelay(delayInit, delayMax)),
	)
	_, err := rc.RepoList(ctx, "registry.example.com/path")
	if !errors.Is(err, errs.ErrParsingFailed) {
		t.Errorf("RepoList unexpected error on hostname with a path: expected %v, received %v", errs.ErrParsingFailed, err)
	}
}
