package pqueue

import (
	"context"
	"testing"
	"time"
)

// create example data type
type testData struct {
	pref int
}

func TestNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var qNil *Queue[testData]
	e := testData{pref: 1}
	done, err := qNil.Acquire(ctx, e)
	if err != nil {
		t.Errorf("failed to Acquire a nil queue: %v", err)
	}
	done()
	done, err = qNil.TryAcquire(ctx, e)
	if err != nil {
		t.Errorf("failed to TryAcquire a nil queue: %v", err)
	}
	if done == nil {
		t.Errorf("TryAcquire returned a nil done function")
	} else {
		done()
	}
}

func TestQueue(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	q := New(Opts[testData]{
		Max: 2,
		Next: func(queued, active []*testData) int {
			i := 0
			for j := 1; j < len(queued); j++ {
				if queued[i].pref < queued[j].pref {
					i = j
				}
			}
			return i
		},
	})
	eList := make([]testData, 7)
	for i := range eList {
		eList[i] = testData{pref: i}
	}
	finished := make(chan int)
	// first acquire two
	done0, err := q.Acquire(ctx, eList[0])
	if err != nil {
		t.Fatalf("failed to acquire queue 2: %v", err)
	}
	done1, err := q.Acquire(ctx, eList[1])
	if err != nil {
		t.Fatalf("failed to acquire queue 3: %v", err)
	}
	// background acquire two more, which should block
	for _, i := range []int{2, 3} {
		go func(i int) {
			done, err := q.Acquire(ctx, eList[i])
			if err != nil {
				t.Errorf("failed to acquire queue %d: %v", i, err)
			}
			finished <- i
			done()
		}(i)
	}
	// verify background jobs blocked
	sleepMS(2)
	select {
	case i := <-finished:
		t.Fatalf("acquired from a full queue entry %d", i)
	default:
	}
	// release one, verify both background jobs acquire and release in correct priority order
	done0()
	i := <-finished
	if i != 3 {
		t.Errorf("released the wrong queue entry: expected 3, received %d", i)
	}
	i = <-finished
	if i != 2 {
		t.Errorf("released the wrong queue entry: expected 2, received %d", i)
	}
	// acquire another to fill the queue
	done4, err := q.Acquire(ctx, eList[4])
	if err != nil {
		t.Fatalf("failed to acquire queue 4: %v", err)
	}
	// test context cancel with another background acquire
	go func() {
		done, err := q.Acquire(ctx, eList[5])
		if err == nil {
			t.Errorf("did not fail acquiring queue entry when context canceled")
		}
		if done != nil {
			t.Errorf("done should be nil on failed acquire")
			done()
		}
		finished <- 5
	}()
	sleepMS(2)
	select {
	case i := <-finished:
		t.Fatalf("acquired from a full queue entry %d", i)
	default:
		// successfully blocked
	}
	cancel()
	i = <-finished
	if i != 5 {
		t.Errorf("unexpected finished entry, expected 5, received %d", i)
	}
	// make a new context, and test TryAcquire with a full queue
	ctx = context.Background()
	done6, err := q.TryAcquire(ctx, eList[6])
	if err != nil {
		t.Fatalf("failed TryAcquire on 6: %v", err)
	}
	if done6 != nil {
		t.Errorf("TryAcquire did not return a nil done")
		done6()
	}
	// test TryAcquire with an available space
	done1()
	done6, err = q.TryAcquire(ctx, eList[6])
	if err != nil {
		t.Fatalf("failed TryAcquire on 6: %v", err)
	}
	if done6 == nil {
		t.Errorf("TryAcquire returned a nil done")
	} else {
		done6()
	}
	done4()
}

func TestMulti(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := testData{pref: 1}
	qList := make([]*Queue[testData], 4)
	for i := range qList {
		qList[i] = New(Opts[testData]{})
	}
	// test acquiring nil multiple times
	_, done, err := AcquireMulti(ctx, e, nil, nil)
	if err != nil {
		t.Errorf("failed to acquire same queue multiple times: %v", err)
	}
	if done != nil {
		done()
	}
	// test acquiring same queue multiple times
	_, done, err = AcquireMulti(ctx, e, qList[0], qList[0])
	if err != nil {
		t.Errorf("failed to acquire same queue multiple times: %v", err)
	}
	if done != nil {
		done()
	}
	ctxMulti, doneAll, err := AcquireMulti(ctx, e, qList[0], qList[1], nil, qList[0], qList[1], nil, qList[0])
	if err != nil {
		t.Errorf("failed to acquire a couple queues multiple times: %v", err)
	}
	// verify requests to Acquire inside of the context work, but only for the included queues
	done, err = qList[0].TryAcquire(ctxMulti, e)
	if err != nil {
		t.Errorf("failed to acquire queue from AcquireMulti list: %v", err)
	}
	if done == nil {
		t.Errorf("TryAcquire did not return a done func")
	} else {
		done()
	}
	done, err = qList[1].TryAcquire(ctxMulti, e)
	if err != nil {
		t.Errorf("failed to acquire queue from AcquireMulti list: %v", err)
	}
	if done == nil {
		t.Errorf("TryAcquire did not return a done func")
	} else {
		done()
	}
	done, err = qList[2].TryAcquire(ctxMulti, e)
	if err == nil {
		t.Errorf("did not fail acquiring a queue missing from the AcquireMulti list")
	}
	if done != nil {
		t.Errorf("TryAcquire returned a done function on a queue not in the context")
		done()
	}
	done, err = qList[0].Acquire(ctxMulti, e)
	if err != nil {
		t.Errorf("failed to acquire queue from AcquireMulti list: %v", err)
	}
	if done == nil {
		t.Errorf("Acquire did not return a done func")
	} else {
		done()
	}
	done, err = qList[1].Acquire(ctxMulti, e)
	if err != nil {
		t.Errorf("failed to acquire queue from AcquireMulti list: %v", err)
	}
	if done == nil {
		t.Errorf("Acquire did not return a done func")
	} else {
		done()
	}
	done, err = qList[2].Acquire(ctxMulti, e)
	if err == nil {
		t.Errorf("did not fail acquiring a queue missing from the AcquireMulti list")
	}
	if done != nil {
		t.Errorf("Acquire returned a done function on a queue not in the context")
		done()
	}
	// try acquiring from outside of the context to verify request is blocked by the running MultiAcquire
	done, err = qList[0].TryAcquire(ctx, e)
	if err != nil {
		t.Errorf("TryAcquire returned an error: %v", err)
	}
	if done != nil {
		t.Errorf("TryAcquire succeeded before MultiAcquire released resource")
		done()
	}
	done, err = qList[1].TryAcquire(ctx, e)
	if err != nil {
		t.Errorf("TryAcquire returned an error: %v", err)
	}
	if done != nil {
		t.Errorf("TryAcquire succeeded before MultiAcquire released resource")
		done()
	}
	// release the MultiAcquire
	if doneAll != nil {
		doneAll()
	}
	// try acquiring from outside of the context to verify request is no longer blocked
	done, err = qList[0].TryAcquire(ctx, e)
	if err != nil {
		t.Errorf("TryAcquire returned an error: %v", err)
	}
	if done == nil {
		t.Errorf("TryAcquire failed after MultiAcquire released resource")
	} else {
		done()
	}
	done, err = qList[1].TryAcquire(ctx, e)
	if err != nil {
		t.Errorf("TryAcquire returned an error: %v", err)
	}
	if done == nil {
		t.Errorf("TryAcquire failed after MultiAcquire released resource")
	} else {
		done()
	}
	// try acquiring with the old context to verify it no longer blocks any request
	for _, i := range []int{0, 1, 2} {
		done, err = qList[i].TryAcquire(ctxMulti, e)
		if err != nil {
			t.Errorf("TryAcquire returned an error for %d: %v", i, err)
		}
		if done == nil {
			t.Errorf("TryAcquire failed after MultiAcquire released resource for %d", i)
		} else {
			done()
		}
	}
	// setup blocking requests to MultiAcquire, with tests for first, last, and middle entry in the queue list blocking
	doneList := make([]func(), 3)
	for _, i := range []int{0, 1, 2} {
		doneList[i], err = qList[i].TryAcquire(ctx, e)
		if err != nil {
			t.Errorf("failed acquiring %d: %v", i, err)
		}
		if doneList[i] == nil {
			t.Errorf("acquiring %d returned a nil", i)
		}
	}
	finished := make(chan int)
	for i, list := range [][]*Queue[testData]{
		{qList[0], qList[3]},                          // first entry blocking
		{qList[3], qList[1]},                          // last entry blocking
		{qList[0], qList[2], nil, qList[1], qList[3]}, // middle entry blocking (0 and 1 will be released first)
	} {
		go func(i int, list []*Queue[testData]) {
			_, done, err := AcquireMulti(ctx, e, list...)
			if err != nil {
				t.Errorf("failed to AcquireMulti for group %d: %v", i, err)
			}
			if done != nil {
				done()
			}
			finished <- i
		}(i, list)
	}
	sleepMS(5)
	select {
	case i := <-finished:
		t.Errorf("unexpected early release of AcquireMulti group %d", i)
	default:
		// expected, all entries blocked
	}
	// release 0, verify AcquireMulti group 0 runs
	doneList[0]()
	i := <-finished
	if i != 0 {
		t.Errorf("unexpected group released %d", i)
	}
	// release 1, verify group 1 runs
	doneList[1]()
	i = <-finished
	if i != 1 {
		t.Errorf("unexpected group released %d", i)
	}
	// release 2, verify group 2 runs
	doneList[2]()
	i = <-finished
	if i != 2 {
		t.Errorf("unexpected group released %d", i)
	}
	// cancel context while MultiAcquire is blocked, verify return, release blocked queue, and verify nothing is blocked
	cancelCtx, cancelFn := context.WithCancel(ctx)
	done, err = qList[0].TryAcquire(cancelCtx, e)
	if err != nil {
		t.Errorf("failed acquiring %d: %v", i, err)
	}
	if done == nil {
		t.Errorf("acquiring %d returned a nil", i)
	}
	go func() {
		sleepMS(5)
		cancelFn()
	}()
	_, doneAll, err = AcquireMulti(cancelCtx, e, qList[0], qList[1])
	if err == nil {
		t.Errorf("AcquireMulti did not error when context canceled")
	}
	if doneAll != nil {
		t.Errorf("AcquireMulti on a canceled context did not return a nil done function")
		doneAll()
	}
	if done != nil {
		done()
	}
	for i := range qList {
		done, err = qList[i].TryAcquire(ctxMulti, e)
		if err != nil {
			t.Errorf("TryAcquire returned an error for %d: %v", i, err)
		}
		if done == nil {
			t.Errorf("TryAcquire blocked for queue %d", i)
		} else {
			done()
		}
	}
}

func sleepMS(ms int64) {
	time.Sleep(time.Millisecond * time.Duration(ms))
}
