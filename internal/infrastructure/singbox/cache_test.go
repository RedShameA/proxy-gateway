package singbox

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSingBoxOutboundCacheInvalidatesAndClosesReleasedEntries(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var builds int32
	var closes int32

	entry, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
		atomic.AddInt32(&builds, 1)
		return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
			atomic.AddInt32(&closes, 1)
			return nil
		}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.release(entry)

	cache.invalidateFingerprint("fingerprint-1")
	if got := atomic.LoadInt32(&closes); got != 1 {
		t.Fatalf("closes after invalidate = %d, want 1", got)
	}

	replacement, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
		atomic.AddInt32(&builds, 1)
		return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
			atomic.AddInt32(&closes, 1)
			return nil
		}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.release(replacement)
	if got := atomic.LoadInt32(&builds); got != 2 {
		t.Fatalf("builds = %d, want replacement build after invalidation", got)
	}
}

func TestSingBoxOutboundCacheClosesInvalidatedEntryOnRelease(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var closes int32
	entry, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
		return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
			atomic.AddInt32(&closes, 1)
			return nil
		}), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		cache.invalidateFingerprint("fingerprint-1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("invalidate blocked while entry was still referenced")
	}
	if got := atomic.LoadInt32(&closes); got != 0 {
		t.Fatalf("closes before release = %d, want 0", got)
	}

	cache.release(entry)
	if got := atomic.LoadInt32(&closes); got != 1 {
		t.Fatalf("closes after release = %d, want 1", got)
	}
}

func TestSingBoxOutboundCacheInvalidatesChainsByFingerprint(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var chainCloses int32
	chain, err := cache.acquireChain("front->exit", func() (*singBoxCachedOutbound, error) {
		return newSingBoxCachedOutbound("front->exit", []string{"front", "exit"}, nil, func() error {
			atomic.AddInt32(&chainCloses, 1)
			return nil
		}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.release(chain)

	cache.invalidateFingerprint("exit")
	if got := atomic.LoadInt32(&chainCloses); got != 1 {
		t.Fatalf("chain closes after exit invalidation = %d, want 1", got)
	}
}
