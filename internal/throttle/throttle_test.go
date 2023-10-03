package throttle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var tNil *Throttle
	err := tNil.Acquire(ctx)
	if err != nil {
		t.Errorf("acquire failed: %v", err)
	}
	err = tNil.Release(ctx)
	if err != nil {
		t.Errorf("release failed: %v", err)
	}
	a, err := tNil.TryAcquire(ctx)
	if err != nil {
		t.Errorf("try acquire failed: %v", err)
	}
	if !a {
		t.Errorf("try acquire did not succeed")
	}

}

func TestSingleThrottle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := sync.WaitGroup{}
	t1 := New(1)
	// simple acquire
	err := t1.Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire: %v", err)
		return
	}
	// try to acquire in a goroutine
	acquired := false
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := t1.Acquire(ctx)
		if err != nil {
			t.Errorf("failed to acquire: %v", err)
			return
		}
		acquired = true
	}()
	sleepMS(1)
	// verify goroutine did not succeed and cannot be acquired
	if acquired {
		t.Errorf("throttle acquired before previous released")
	}
	a, err := t1.TryAcquire(ctx)
	if err != nil {
		t.Errorf("try acquire errored: %v", err)
	}
	if a {
		t.Errorf("try acquire succeeded")
	}
	// release and verify goroutine acquires and returns
	err = t1.Release(ctx)
	if err != nil {
		t.Errorf("release failed: %v", err)
	}
	wg.Wait()
	if !acquired {
		t.Errorf("throttle was not acquired by thread")
	}
	// start a new goroutine to acquire
	acquired = false
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := t1.Acquire(ctx)
		if err == nil {
			acquired = true
			return
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("acquire on cancel returned: %v", err)
			return
		}
	}()
	sleepMS(1)
	// verify goroutine still waiting, cancel context, and verify the return
	if acquired {
		t.Errorf("throttle acquired before previous released")
	}
	cancel()
	wg.Wait()
	if acquired {
		t.Errorf("throttle was not acquired by thread")
	}
	ctx = context.Background()
	a, err = t1.TryAcquire(ctx)
	if err != nil {
		t.Errorf("try acquire errored: %v", err)
	}
	if a {
		t.Errorf("try acquire succeeded")
	}
	// release, twice, and verify try acquire can succeed
	err = t1.Release(ctx)
	if err != nil {
		t.Errorf("release failed: %v", err)
	}
	err = t1.Release(context.Background())
	if err == nil {
		t.Errorf("second release succeeded")
	}
	a, err = t1.TryAcquire(context.Background())
	if err != nil {
		t.Errorf("try acquire errored: %v", err)
	}
	if !a {
		t.Errorf("try acquire failed")
	}
}

func TestCapacity(t *testing.T) {
	t.Parallel()
	t3 := New(3)
	wg := sync.WaitGroup{}
	ctx := context.Background()

	// acquire all three with an intermediate release
	err := t3.Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire: %v", err)
	}
	err = t3.Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire: %v", err)
	}
	err = t3.Release(ctx)
	if err != nil {
		t.Errorf("failed to release: %v", err)
	}
	err = t3.Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire: %v", err)
	}
	err = t3.Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire: %v", err)
	}
	// verify try acquire fails on the 4th
	a, err := t3.TryAcquire(ctx)
	if err != nil {
		t.Errorf("failed to try acquire: %v", err)
	}
	if a {
		t.Errorf("try acquire succeeded on full throttle")
	}
	// launch acquire requests in background
	wg.Add(1)
	a = false
	go func() {
		defer wg.Done()
		err := t3.Acquire(ctx)
		if err != nil {
			t.Errorf("failed to acquire: %v", err)
		}
		a = true
		err = t3.Acquire(ctx)
		if err != nil {
			t.Errorf("failed to acquire: %v", err)
		}
	}()
	sleepMS(1)
	if a {
		t.Errorf("acquire ran in background")
	}
	// release two
	err = t3.Release(ctx)
	if err != nil {
		t.Errorf("failed to release: %v", err)
	}
	err = t3.Release(ctx)
	if err != nil {
		t.Errorf("failed to release: %v", err)
	}
	// wait for background job to finish and verify acquired
	wg.Wait()
	if !a {
		t.Errorf("acquire did not run in background")
	}
}

func TestMulti(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tList := make([]*Throttle, 4)
	for i := 0; i < 3; i++ {
		tList[i] = New(1)
	}
	wg := sync.WaitGroup{}

	// acquire duplicate throttles, one multiple times, another two in alternating sets `ababa`
	tListA := []*Throttle{tList[0], tList[0], tList[0]}
	tListB := []*Throttle{tList[1], tList[2], tList[1], tList[2], tList[1]}
	ctxA, err := AcquireMulti(ctx, tListA)
	if err != nil {
		t.Errorf("failed to acquire multiple entries to same throttle: %v", err)
	}
	ctxB, err := AcquireMulti(ctx, tListB)
	if err != nil {
		t.Errorf("failed to acquire multiple entries to pair of throttles: %v", err)
	}
	ok, err := tList[0].TryAcquire(ctx)
	if err != nil || ok {
		t.Errorf("try acquire on 0 did not fail, ok=%t, err=%v", ok, err)
	}
	ok, err = tList[1].TryAcquire(ctx)
	if err != nil || ok {
		t.Errorf("try acquire on 1 did not fail, ok=%t, err=%v", ok, err)
	}
	ok, err = tList[2].TryAcquire(ctx)
	if err != nil || ok {
		t.Errorf("try acquire on 2 did not fail, ok=%t, err=%v", ok, err)
	}
	err = ReleaseMulti(ctxA, tListA)
	if err != nil {
		t.Errorf("failed to release multiple locks on A: %v", err)
	}
	err = ReleaseMulti(ctxB, tListB)
	if err != nil {
		t.Errorf("failed to release multiple locks on B: %v", err)
	}
	// use acquire multi on first two
	_, err = AcquireMulti(ctx, tList[:0])
	if err != nil {
		t.Errorf("empty list acquire multi failed: %v", err)
	}
	ctxMulti, err := AcquireMulti(ctx, tList[:2])
	if err != nil {
		t.Errorf("failed to acquire multi: %v", err)
		return
	}
	_, err = AcquireMulti(ctxMulti, tList[2:])
	if err == nil {
		t.Errorf("nested acquire multi did not fail")
	}
	// try acquiring individually with ctxMulti, first two should succeed
	a, err := tList[0].TryAcquire(ctxMulti)
	if err != nil {
		t.Errorf("failed to try acquire on 0: %v", err)
	}
	if !a {
		t.Errorf("try acquire on 0 did not return true")
	}
	a, err = tList[1].TryAcquire(ctxMulti)
	if err != nil {
		t.Errorf("failed to try acquire on 1: %v", err)
	}
	if !a {
		t.Errorf("try acquire on 1 did not return true")
	}
	// actions on 2 should all fail because it's not in the multi list
	err = tList[2].Acquire(ctxMulti)
	if err == nil {
		t.Errorf("acquire on 2 did not error")
	}
	err = tList[2].Release(ctxMulti)
	if err == nil {
		t.Errorf("acquire on 2 did not error")
	}
	a, err = tList[2].TryAcquire(ctxMulti)
	if err == nil {
		t.Errorf("try acquire on 2 did not error")
	}
	if a {
		t.Errorf("try acquire on 2 returned true")
	}
	// try acquire with ctx, first two should be blocked, 3rd should succeed
	a, err = tList[0].TryAcquire(ctx)
	if err != nil {
		t.Errorf("failed to try acquire on 0: %v", err)
	}
	if a {
		t.Errorf("try acquire on 0 returned true")
	}
	a, err = tList[1].TryAcquire(ctx)
	if err != nil {
		t.Errorf("failed to try acquire on 1: %v", err)
	}
	if a {
		t.Errorf("try acquire on 1 returned true")
	}
	a, err = tList[2].TryAcquire(ctx)
	if err != nil {
		t.Errorf("failed to try acquire on 2: %v", err)
	}
	if !a {
		t.Errorf("try acquire on 2 returned false")
	}
	// run acquire in background on first two
	wg.Add(1)
	finished := false
	go func() {
		defer wg.Done()
		err := tList[0].Acquire(ctx)
		if err != nil {
			t.Errorf("failed to acquire 0: %v", err)
		}
		err = tList[1].Acquire(ctx)
		if err != nil {
			t.Errorf("failed to acquire 1: %v", err)
		}
		finished = true
	}()
	sleepMS(1)
	if finished {
		t.Errorf("background job finished before release")
	}
	// release multi
	err = ReleaseMulti(ctxMulti, tList[:2])
	if err != nil {
		t.Errorf("failed to release multi: %v", err)
	}
	// verify background job finished
	wg.Wait()
	if !finished {
		t.Errorf("background job did not finish")
	}
	// release all
	for i := 0; i < 3; i++ {
		err = tList[i].Release(ctx)
		if err != nil {
			t.Errorf("failed to release %d: %v", i, err)
		}
	}
	// verify acquire, try acquire, and release with old context succeeds
	err = tList[0].Acquire(ctxMulti)
	if err != nil {
		t.Errorf("acquire on stale context failed: %v", err)
	}
	a, err = tList[0].TryAcquire(ctxMulti)
	if err != nil {
		t.Errorf("try acquire on stale context failed: %v", err)
	}
	if a {
		t.Errorf("try acquire returned true")
	}
	err = tList[0].Release(ctxMulti)
	if err != nil {
		t.Errorf("release on stale context failed: %v", err)
	}
	// acquire 0 and verify acquire multi blocked
	err = tList[0].Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire 0: %v", err)
	}
	wg.Add(1)
	finished = false
	go func() {
		defer wg.Done()
		ctxNew, err := AcquireMulti(ctx, tList)
		if err != nil {
			t.Errorf("failed to acquire multi")
		}
		ctxMulti = ctxNew
		finished = true
	}()
	sleepMS(1)
	err = tList[1].Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire 1: %v", err)
	}
	err = tList[0].Release(ctx)
	if err != nil {
		t.Errorf("failed to release 0: %v", err)
	}
	sleepMS(1)
	err = tList[0].Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire 0: %v", err)
	}
	err = tList[1].Release(ctx)
	if err != nil {
		t.Errorf("failed to release 1: %v", err)
	}
	sleepMS(1)
	err = tList[2].Acquire(ctx)
	if err != nil {
		t.Errorf("failed to acquire 2: %v", err)
	}
	err = tList[0].Release(ctx)
	if err != nil {
		t.Errorf("failed to release 0: %v", err)
	}
	sleepMS(1)
	if finished {
		t.Errorf("acquire multi returned before release was run")
	}
	// release and verify acquire multi finishes
	err = tList[2].Release(ctx)
	if err != nil {
		t.Errorf("failed to release 2: %v", err)
	}
	wg.Wait()
	if !finished {
		t.Errorf("acquire multi did not finish")
	}
	// run acquire in background
	wg.Add(1)
	finished = false
	go func() {
		defer wg.Done()
		err := tList[1].Acquire(ctx)
		if err != nil {
			t.Errorf("failed to acquire after release multi: %v", err)
		}
		finished = true
	}()
	sleepMS(1)
	if finished {
		t.Errorf("acquire returned before release multi was run")
	}
	// release multi
	err = ReleaseMulti(ctx, tList)
	if err == nil {
		t.Errorf("release multi on wrong context succeeded")
	}
	err = ReleaseMulti(ctxMulti, tList)
	if err != nil {
		t.Errorf("release multi failed: %v", err)
	}
	err = ReleaseMulti(ctxMulti, tList)
	if err == nil {
		t.Errorf("release multi again succeeded")
	}
	// verify background acquire finished
	wg.Wait()
	if !finished {
		t.Errorf("acquire did not finish")
	}
	// verify acquire/release with multi context works after release multi
	err = tList[0].Acquire(ctxMulti)
	if err != nil {
		t.Errorf("failed to acquire after release multi on multi context: %v", err)
	}
	a, err = tList[2].TryAcquire(ctxMulti)
	if err != nil {
		t.Errorf("failed to try acquire after release multi on multi context: %v", err)
	}
	if !a {
		t.Errorf("try acquire after release multi with multi context returned false")
	}
	err = tList[0].Release(ctxMulti)
	if err != nil {
		t.Errorf("failed to release 0: %v", err)
	}
	err = tList[2].Release(ctxMulti)
	if err != nil {
		t.Errorf("failed to release 2: %v", err)
	}
	// run acquire multi in background with 1 still blocked and cancel context
	finished = false
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctxMulti, err := AcquireMulti(ctx, tList)
		if errors.Is(err, context.Canceled) {
			// expected failure from context
			return
		}
		t.Errorf("acquire multi did not fail on canceled context")
		_ = ReleaseMulti(ctxMulti, tList)
		finished = true
	}()
	sleepMS(1)
	if finished {
		t.Errorf("acquire multi finished unexpectedly")
	}
	// cancel the context, wait for acquire multi to fail
	cancel()
	wg.Wait()
	err = tList[1].Release(ctx)
	if err != nil {
		t.Errorf("failed to release 1: %v", err)
	}
}

func sleepMS(ms int64) {
	time.Sleep(time.Millisecond * time.Duration(ms))
}
