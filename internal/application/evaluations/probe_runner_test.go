package evaluations

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunConcurrentProbesLimitsConcurrencyAndCollectsResults(t *testing.T) {
	var running int32
	var maxRunning int32

	results := RunConcurrentProbes([]int{1, 2, 3, 4}, 2, func(value int) int {
		current := atomic.AddInt32(&running, 1)
		for {
			max := atomic.LoadInt32(&maxRunning)
			if current <= max || atomic.CompareAndSwapInt32(&maxRunning, max, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return value * 10
	})

	if len(results) != 4 {
		t.Fatalf("results len = %d, want 4", len(results))
	}
	if atomic.LoadInt32(&maxRunning) > 2 {
		t.Fatalf("max running = %d, want <= 2", maxRunning)
	}
}

func TestRunConcurrentProbesReturnsNilForEmptyInput(t *testing.T) {
	results := RunConcurrentProbes[int, int](nil, 2, func(value int) int {
		t.Fatal("probe should not be called")
		return value
	})

	if results != nil {
		t.Fatalf("results = %#v, want nil", results)
	}
}
