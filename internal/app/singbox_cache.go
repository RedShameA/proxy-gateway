package app

import (
	"errors"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

type singBoxOutboundCache struct {
	mu     sync.Mutex
	single map[string]*singBoxCachedOutbound
	chain  map[string]*singBoxCachedOutbound
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
		single: make(map[string]*singBoxCachedOutbound),
		chain:  make(map[string]*singBoxCachedOutbound),
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

func (c *singBoxOutboundCache) acquireSingle(key string, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, error) {
	return c.acquire(c.single, key, build)
}

func (c *singBoxOutboundCache) acquireChain(key string, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, error) {
	return c.acquire(c.chain, key, build)
}

func (c *singBoxOutboundCache) acquire(entries map[string]*singBoxCachedOutbound, key string, build func() (*singBoxCachedOutbound, error)) (*singBoxCachedOutbound, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := entries[key]; ok && !entry.closing {
		entry.refs++
		entry.lastUsedAt = time.Now()
		return entry, nil
	}
	entry, err := build()
	if err != nil {
		return nil, err
	}
	entry.refs = 1
	entries[key] = entry
	return entry, nil
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
	ready := c.markClosing(entries)
	c.mu.Unlock()
	_ = c.closeEntries(ready)
}

func (c *singBoxOutboundCache) closeAll() error {
	c.mu.Lock()
	entries := make([]*singBoxCachedOutbound, 0, len(c.single)+len(c.chain))
	for key, entry := range c.single {
		delete(c.single, key)
		entries = append(entries, entry)
	}
	for key, entry := range c.chain {
		delete(c.chain, key)
		entries = append(entries, entry)
	}
	ready := c.markClosing(entries)
	c.mu.Unlock()
	return c.closeEntries(ready)
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
	for _, item := range e.fingerprints {
		if item == fingerprint {
			return true
		}
	}
	return false
}
