package singbox

import (
	"errors"
	"fmt"
	"sync"
	"time"

	appproxy "proxygateway/internal/application/proxy"

	"github.com/sagernet/sing-box/adapter"
)

type singBoxOutboundCache struct {
	mu       sync.Mutex
	single   map[string]*singBoxCachedOutbound
	chain    map[string]*singBoxCachedOutbound
	building map[singBoxOutboundBuildKey]*singBoxOutboundBuildState
	closed   bool
}

type singBoxOutboundCacheNamespace string

const (
	singBoxOutboundCacheSingle singBoxOutboundCacheNamespace = "single"
	singBoxOutboundCacheChain  singBoxOutboundCacheNamespace = "chain"
)

var (
	errSingBoxOutboundCacheClosed      = errors.New("sing-box outbound cache is closed")
	errSingBoxOutboundBuildInvalidated = errors.New("sing-box outbound build invalidated")
)

type singBoxOutboundBuildKey struct {
	namespace singBoxOutboundCacheNamespace
	key       string
}

type singBoxOutboundBuildState struct {
	fingerprints []string
	done         chan struct{}
	err          error
	invalidated  bool
}

type singBoxCachedOutbound struct {
	key          string
	fingerprints []string
	outbound     adapter.Outbound
	close        func() error
	createdAt    time.Time
	lastUsedAt   time.Time
	refs         int
	closing      bool
	closed       bool
}

func newSingBoxOutboundCache() *singBoxOutboundCache {
	return &singBoxOutboundCache{
		single:   make(map[string]*singBoxCachedOutbound),
		chain:    make(map[string]*singBoxCachedOutbound),
		building: make(map[singBoxOutboundBuildKey]*singBoxOutboundBuildState),
	}
}

func newSingBoxCachedOutbound(key string, fingerprints []string, outbound adapter.Outbound, closeFn func() error) *singBoxCachedOutbound {
	now := time.Now()
	return &singBoxCachedOutbound{
		key:          key,
		fingerprints: append([]string(nil), fingerprints...),
		outbound:     outbound,
		close:        closeFn,
		createdAt:    now,
		lastUsedAt:   now,
	}
}

func (c *singBoxOutboundCache) acquireSingle(key string, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, appproxy.DialMetrics, error) {
	return c.acquire(c.single, singBoxOutboundCacheSingle, key, []string{key}, build)
}

func (c *singBoxOutboundCache) acquireChain(key string, fingerprints []string, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, appproxy.DialMetrics, error) {
	return c.acquire(c.chain, singBoxOutboundCacheChain, key, fingerprints, build)
}

func (c *singBoxOutboundCache) acquire(entries map[string]*singBoxCachedOutbound, namespace singBoxOutboundCacheNamespace, key string, fingerprints []string, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, appproxy.DialMetrics, error) {
	buildKey := singBoxOutboundBuildKey{namespace: namespace, key: key}
	var metrics appproxy.DialMetrics
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, metrics, errSingBoxOutboundCacheClosed
		}
		if entry, ok := entries[key]; ok && !entry.closing {
			entry.refs++
			entry.lastUsedAt = time.Now()
			c.mu.Unlock()
			return entry, metrics, nil
		}
		if state, ok := c.building[buildKey]; ok {
			c.mu.Unlock()
			waitStart := time.Now()
			<-state.done
			metrics.CacheWaitMS += durationMillis(waitStart)
			if state.err != nil {
				return nil, metrics, state.err
			}
			continue
		}
		state := &singBoxOutboundBuildState{
			fingerprints: append([]string(nil), fingerprints...),
			done:         make(chan struct{}),
		}
		c.building[buildKey] = state
		c.mu.Unlock()
		return c.buildAndStore(entries, buildKey, state, metrics, build)
	}
}

func (c *singBoxOutboundCache) buildAndStore(entries map[string]*singBoxCachedOutbound, buildKey singBoxOutboundBuildKey, state *singBoxOutboundBuildState, metrics appproxy.DialMetrics, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, appproxy.DialMetrics, error) {
	buildStart := time.Now()
	entry, err := build()
	metrics.CacheBuildMS += durationMillis(buildStart)
	var closeEntry *singBoxCachedOutbound
	c.mu.Lock()
	defer func() {
		delete(c.building, buildKey)
		state.err = err
		close(state.done)
		c.mu.Unlock()
		if closeEntry != nil {
			_ = c.closeEntries([]*singBoxCachedOutbound{closeEntry})
		}
	}()
	if err != nil {
		return nil, metrics, err
	}
	if c.closed {
		err = errSingBoxOutboundCacheClosed
		closeEntry = entry
		return nil, metrics, err
	}
	if state.invalidated {
		err = fmt.Errorf("%w: %s:%s", errSingBoxOutboundBuildInvalidated, buildKey.namespace, buildKey.key)
		closeEntry = entry
		return nil, metrics, err
	}
	if existing, ok := entries[buildKey.key]; ok && !existing.closing {
		existing.refs++
		existing.lastUsedAt = time.Now()
		closeEntry = entry
		return existing, metrics, nil
	}
	entry.refs = 1
	entries[buildKey.key] = entry
	return entry, metrics, nil
}

func (c *singBoxOutboundCache) release(entry *singBoxCachedOutbound) {
	if entry == nil {
		return
	}
	var closeFn func() error
	c.mu.Lock()
	if entry.refs > 0 {
		entry.refs--
	}
	if entry.refs == 0 && entry.closing && !entry.closed {
		entry.closed = true
		closeFn = entry.close
	}
	c.mu.Unlock()
	if closeFn != nil {
		_ = closeFn()
	}
}

func (c *singBoxOutboundCache) invalidateFingerprint(fingerprint string) {
	if fingerprint == "" {
		return
	}
	c.mu.Lock()
	var entries []*singBoxCachedOutbound
	if entry, ok := c.single[fingerprint]; ok {
		delete(c.single, fingerprint)
		entries = append(entries, entry)
	}
	for key, entry := range c.chain {
		if entry.hasFingerprint(fingerprint) {
			delete(c.chain, key)
			entries = append(entries, entry)
		}
	}
	for _, state := range c.building {
		if fingerprintsContain(state.fingerprints, fingerprint) {
			state.invalidated = true
		}
	}
	ready := c.markClosing(entries)
	c.mu.Unlock()
	_ = c.closeEntries(ready)
}

func (c *singBoxOutboundCache) closeAll() error {
	c.mu.Lock()
	c.closed = true
	entries := make([]*singBoxCachedOutbound, 0, len(c.single)+len(c.chain))
	for key, entry := range c.single {
		delete(c.single, key)
		entries = append(entries, entry)
	}
	for key, entry := range c.chain {
		delete(c.chain, key)
		entries = append(entries, entry)
	}
	building := make([]*singBoxOutboundBuildState, 0, len(c.building))
	for _, state := range c.building {
		state.invalidated = true
		building = append(building, state)
	}
	ready := c.markClosing(entries)
	c.mu.Unlock()
	err := c.closeEntries(ready)
	for _, state := range building {
		<-state.done
	}
	return err
}

func (c *singBoxOutboundCache) markClosing(entries []*singBoxCachedOutbound) []*singBoxCachedOutbound {
	ready := make([]*singBoxCachedOutbound, 0, len(entries))
	for _, entry := range entries {
		entry.closing = true
		if entry.refs == 0 && !entry.closed {
			entry.closed = true
			ready = append(ready, entry)
		}
	}
	return ready
}

func (c *singBoxOutboundCache) closeEntries(entries []*singBoxCachedOutbound) error {
	var err error
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.close != nil {
			err = errors.Join(err, entry.close())
		}
	}
	return err
}

func (e *singBoxCachedOutbound) hasFingerprint(fingerprint string) bool {
	return fingerprintsContain(e.fingerprints, fingerprint)
}

func fingerprintsContain(fingerprints []string, fingerprint string) bool {
	for _, item := range fingerprints {
		if item == fingerprint {
			return true
		}
	}
	return false
}

func durationMillis(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	return time.Since(start).Milliseconds()
}
