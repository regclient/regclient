// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
