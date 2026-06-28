package singbox

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingBoxOutboundCacheInvalidatesAndClosesReleasedEntries(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var builds int32
	var closes int32

	entry, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
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

	replacement, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
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
	entry, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
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
	chain, _, err := cache.acquireChain("front->exit", []string{"front", "exit"}, func() (*singBoxCachedOutbound, error) {
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

func TestSingBoxOutboundCacheBuildsDifferentKeysConcurrently(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	started := make(chan string, 2)
	releaseBuilds := make(chan struct{})
	errs := make(chan error, 2)

	for _, key := range []string{"fingerprint-1", "fingerprint-2"} {
		key := key
		go func() {
			entry, _, err := cache.acquireSingle(key, func() (*singBoxCachedOutbound, error) {
				started <- key
				<-releaseBuilds
				return newSingBoxCachedOutbound(key, []string{key}, nil, nil), nil
			})
			if err == nil {
				cache.release(entry)
			}
			errs <- err
		}()
	}

	waitForCacheTestSignals(t, started, 2, "parallel builds")
	close(releaseBuilds)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSingBoxOutboundCacheCoalescesSameKeyBuilds(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	const callers = 5
	var builds int32
	started := make(chan struct{}, 1)
	releaseBuild := make(chan struct{})
	entries := make(chan *singBoxCachedOutbound, callers)
	errs := make(chan error, callers)

	for i := 0; i < callers; i++ {
		go func() {
			entry, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
				atomic.AddInt32(&builds, 1)
				started <- struct{}{}
				<-releaseBuild
				return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, nil), nil
			})
			entries <- entry
			errs <- err
		}()
	}

	waitForCacheTestSignals(t, started, 1, "first build")
	close(releaseBuild)
	var first *singBoxCachedOutbound
	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		entry := <-entries
		if first == nil {
			first = entry
			continue
		}
		if entry != first {
			t.Fatal("same-key acquire returned different cached entries")
		}
	}
	if got := atomic.LoadInt32(&builds); got != 1 {
		t.Fatalf("builds = %d, want 1", got)
	}
	cache.mu.Lock()
	refs := first.refs
	cache.mu.Unlock()
	if refs != callers {
		t.Fatalf("refs = %d, want %d", refs, callers)
	}
	for i := 0; i < callers; i++ {
		cache.release(first)
	}
}

func TestSingBoxOutboundCacheSeparatesSingleAndChainBuildKeys(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var singleBuilds int32
	var chainBuilds int32
	single, _, err := cache.acquireSingle("same-fingerprint", func() (*singBoxCachedOutbound, error) {
		atomic.AddInt32(&singleBuilds, 1)
		return newSingBoxCachedOutbound("same-fingerprint", []string{"same-fingerprint"}, nil, nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.release(single)

	chain, _, err := cache.acquireChain("same-fingerprint", []string{"same-fingerprint", "exit-fingerprint"}, func() (*singBoxCachedOutbound, error) {
		atomic.AddInt32(&chainBuilds, 1)
		return newSingBoxCachedOutbound("same-fingerprint", []string{"same-fingerprint", "exit-fingerprint"}, nil, nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.release(chain)
	if single == chain {
		t.Fatal("single and chain cache entries unexpectedly share one entry")
	}
	if got := atomic.LoadInt32(&singleBuilds); got != 1 {
		t.Fatalf("single builds = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&chainBuilds); got != 1 {
		t.Fatalf("chain builds = %d, want 1", got)
	}
}

func TestSingBoxOutboundCacheReturnsBuildFailureToWaiterWithoutImmediateRebuild(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	buildErr := errors.New("build failed")
	var builds int32
	done := make(chan struct{})
	state := &singBoxOutboundBuildState{
		fingerprints: []string{"fingerprint-1"},
		done:         done,
		err:          buildErr,
	}
	close(done)
	cache.mu.Lock()
	cache.building[singBoxOutboundBuildKey{namespace: singBoxOutboundCacheSingle, key: "fingerprint-1"}] = state
	cache.mu.Unlock()

	if _, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
		atomic.AddInt32(&builds, 1)
		return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, nil), nil
	}); !errors.Is(err, buildErr) {
		t.Fatalf("waiter error = %v, want build error", err)
	}
	if got := atomic.LoadInt32(&builds); got != 0 {
		t.Fatalf("builds = %d, want no immediate rebuild", got)
	}
}

func TestSingBoxOutboundCacheInvalidateDuringBuildDropsAndClosesBuiltEntry(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var builds int32
	var closes int32
	started := make(chan struct{}, 1)
	releaseBuild := make(chan struct{})
	errs := make(chan error, 1)

	go func() {
		_, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
			atomic.AddInt32(&builds, 1)
			started <- struct{}{}
			<-releaseBuild
			return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
				atomic.AddInt32(&closes, 1)
				return nil
			}), nil
		})
		errs <- err
	}()

	waitForCacheTestSignals(t, started, 1, "first build")
	cache.invalidateFingerprint("fingerprint-1")
	close(releaseBuild)
	if err := <-errs; !errors.Is(err, errSingBoxOutboundBuildInvalidated) {
		t.Fatalf("build error = %v, want invalidated", err)
	}
	if got := atomic.LoadInt32(&closes); got != 1 {
		t.Fatalf("closes = %d, want invalidated build entry closed", got)
	}

	replacement, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
		atomic.AddInt32(&builds, 1)
		return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.release(replacement)
	if got := atomic.LoadInt32(&builds); got != 2 {
		t.Fatalf("builds = %d, want rebuild after invalidation", got)
	}
}

func TestSingBoxOutboundCacheCloseAllDuringBuildDropsEntryAndClosesCache(t *testing.T) {
	t.Parallel()

	cache := newSingBoxOutboundCache()
	var closes int32
	started := make(chan struct{}, 1)
	releaseBuild := make(chan struct{})
	acquireErrs := make(chan error, 1)

	go func() {
		_, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
			started <- struct{}{}
			<-releaseBuild
			return newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
				atomic.AddInt32(&closes, 1)
				return nil
			}), nil
		})
		acquireErrs <- err
	}()

	waitForCacheTestSignals(t, started, 1, "first build")
	closeErrs := make(chan error, 1)
	go func() {
		closeErrs <- cache.closeAll()
	}()
	waitForCacheClosed(t, cache)
	close(releaseBuild)

	if err := <-acquireErrs; !errors.Is(err, errSingBoxOutboundCacheClosed) {
		t.Fatalf("build error = %v, want cache closed", err)
	}
	if err := <-closeErrs; err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&closes); got != 1 {
		t.Fatalf("closes = %d, want built entry closed", got)
	}
	if _, _, err := cache.acquireSingle("fingerprint-1", func() (*singBoxCachedOutbound, error) {
		t.Fatal("build should not run after closeAll")
		return nil, nil
	}); !errors.Is(err, errSingBoxOutboundCacheClosed) {
		t.Fatalf("acquire after closeAll error = %v, want cache closed", err)
	}
}

func waitForCacheTestSignals[T any](t *testing.T, ch <-chan T, count int, label string) {
	t.Helper()
	deadline := time.After(time.Second)
	for i := 0; i < count; i++ {
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("timed out waiting for %s signal %d/%d", label, i+1, count)
		}
	}
}

func waitForCacheClosed(t *testing.T, cache *singBoxOutboundCache) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		cache.mu.Lock()
		closed := cache.closed
		cache.mu.Unlock()
		if closed {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cache closed flag")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
