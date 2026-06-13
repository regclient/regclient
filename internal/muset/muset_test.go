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

package muset

import (
	"sync"
	"testing"
	"time"
)

func TestMuset(t *testing.T) {
	t.Parallel()
	// test acquiring sets of locks, sleep between changes to force out race conditions
	var muA, muB, muC sync.Mutex
	// empty set
	Lock()
	// noting initially locked
	Lock(&muA, &muB, &muC)
	muA.Unlock()
	muB.Unlock()
	muC.Unlock()
	// repeating entries
	Lock(&muA, &muA, &muB, &muA, &muB)
	muB.Unlock()
	// A initially locked
	// rotating set of locks in different orders
	finished := false
	delay := time.Microsecond * 10
	wg := sync.WaitGroup{}
	wg.Go(func() {
		Lock(&muA, &muB, &muC)
		finished = true
	})
	time.Sleep(delay)
	if finished {
		t.Error("finished before unlock")
	}
	muB.Lock()
	muA.Unlock()
	time.Sleep(delay)
	if finished {
		t.Error("finished before unlock")
	}
	muC.Lock()
	muB.Unlock()
	time.Sleep(delay)
	if finished {
		t.Error("finished before unlock")
	}
	muA.Lock()
	muC.Unlock()
	time.Sleep(delay)
	if finished {
		t.Error("finished before unlock")
	}
	muA.Unlock()
	wg.Wait()
	if !finished {
		t.Error("did not finish")
	}
}
