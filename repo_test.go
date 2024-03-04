package regclient

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/types/errs"
)

func TestRepoList(t *testing.T) {
	ctx := context.Background()
	log := &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}
	delayInit, _ := time.ParseDuration("0.05s")
	delayMax, _ := time.ParseDuration("0.10s")
	rc := New(
		WithLog(log),
		WithRetryDelay(delayInit, delayMax),
	)
	_, err := rc.RepoList(ctx, "registry.example.com/path")
	if !errors.Is(err, errs.ErrParsingFailed) {
		t.Errorf("RepoList unexpected error on hostname with a path: expected %v, received %v", errs.ErrParsingFailed, err)
	}
}
