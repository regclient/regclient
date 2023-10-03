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
	wg.Add(1)
	go func() {
		Lock(&muA, &muB, &muC)
		finished = true
		wg.Done()
	}()
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
