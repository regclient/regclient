//go:build go1.18
// +build go1.18

package cache

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	t.Parallel()
	testData := []string{
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j",
		"k", "l", "m", "n", "o", "p", "q", "r", "s", "t",
	}
	count := 10
	pruneCount := int(float64(count) * 1.1)
	timeout := time.Millisecond * 250
	timeoutHalf := timeout / 2
	timePrune := timeout + (timeout / 10)
	c := New[int, string](WithAge(timeout), WithCount(count))
	// add entries beyond limit
	for i, v := range testData {
		c.Set(i, v)
	}
	// delete last 2 entries
	for i := len(testData) - 2; i < len(testData); i++ {
		c.Delete(i)
	}
	// get entries, verify some deleted
	pruneCutoff := len(testData) - pruneCount
	saveCutoff := len(testData) - count
	checkCutoff := len(testData) - (count / 2)
	for i, v := range testData {
		getVal, err := c.Get(i)
		if i < pruneCutoff || i >= len(testData)-2 {
			if err == nil {
				t.Errorf("value found that should have been pruned: %d", i)
			}
		} else if i >= saveCutoff && i < len(testData)-2 {
			if err != nil {
				t.Errorf("value not found: %d", i)
			} else if getVal != v {
				t.Errorf("value mismatch: %d, expect %s, received %s", i, v, getVal)
			}
		}
	}
	// track used time
	start := time.Now()
	// delay to before minAge and check some entries
	time.Sleep(timeoutHalf)
	for i, v := range testData {
		if i >= saveCutoff && i < checkCutoff {
			getVal, err := c.Get(i)
			if err != nil {
				t.Errorf("value not found: %d", i)
			} else if getVal != v {
				t.Errorf("value mismatch: %d, expect %s, received %s", i, v, getVal)
			}
		}
	}
	// delay to after maxAge from 1st used time
	time.Sleep(timePrune - time.Since(start))
	for i, v := range testData {
		getVal, err := c.Get(i)
		if i >= saveCutoff && i < checkCutoff {
			if err != nil {
				t.Errorf("value not found: %d", i)
			} else if getVal != v {
				t.Errorf("value mismatch: %d, expect %s, received %s", i, v, getVal)
			}
		} else {
			if err == nil {
				t.Errorf("value not pruned: %d", i)
			}
		}
	}
	// delay to after maxAge for all entries
	time.Sleep(timePrune)
	for i := range testData {
		_, err := c.Get(i)
		if err == nil {
			t.Errorf("value not pruned: %d", i)
		}
	}
	// set and delete a key
	c.Set(42, "x")
	c.Delete(42)
	// delete non-existent key
	c.Delete(42)
}
