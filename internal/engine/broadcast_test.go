package engine

import (
	"sync"
	"testing"
)

func TestBroadcastControlToggle(t *testing.T) {
	c := NewBroadcastControl()
	if c.Paused() {
		t.Fatalf("initial Paused() = true, want false")
	}
	if got := c.Toggle(); !got || !c.Paused() {
		t.Fatalf("first Toggle() = %v, Paused() = %v, want true/true", got, c.Paused())
	}
	if got := c.Toggle(); got || c.Paused() {
		t.Fatalf("second Toggle() = %v, Paused() = %v, want false/false", got, c.Paused())
	}
}

// TestBroadcastControlConcurrentToggle exercises the CAS loop under contention
// (run with -race): every Toggle() call must observe a distinct prior state, so
// exactly N of 2N concurrent toggles should land on true.
func TestBroadcastControlConcurrentToggle(t *testing.T) {
	c := NewBroadcastControl()
	const n = 50
	results := make([]bool, 2*n)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = c.Toggle()
		}(i)
	}
	wg.Wait()

	trueCount := 0
	for _, r := range results {
		if r {
			trueCount++
		}
	}
	if trueCount != n {
		t.Fatalf("2N toggles from a false start should land true exactly N=%d times, got %d", n, trueCount)
	}
}
