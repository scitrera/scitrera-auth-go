// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// Package cache provides a tiny TTL+singleflight cache used by the resolver
// to mirror Python's `cached_async(TTLCache(16384, ttl=300))` pattern over
// MT Postgres lookups. Hot-path lookups (user-by-email, tenant-by-domain,
// tenant_config:auth:checks:*) hit this cache and the singleflight group
// dedupes concurrent misses.
package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Loader is the function used to populate cache misses. It receives the
// caller's context so the database round-trip is cancellable.
type Loader[V any] func(ctx context.Context, key string) (V, error)

// TTLCache is a fixed-capacity TTL cache keyed by string with optional
// generic value type. Eviction is naive (random drop on overflow) — fine
// for hot-path identity lookups where the working set is small relative to
// capacity.
type TTLCache[V any] struct {
	mu         sync.RWMutex
	entries    map[string]ttlEntry[V]
	capacity   int
	ttl        time.Duration
	loader     Loader[V]
	flight     singleflight.Group
	generation uint64
}

type ttlEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// New returns a TTL cache with the given capacity and TTL. capacity <= 0
// disables size-based eviction (entries still expire on TTL).
func New[V any](capacity int, ttl time.Duration, loader Loader[V]) *TTLCache[V] {
	return &TTLCache[V]{
		entries:  make(map[string]ttlEntry[V]),
		capacity: capacity,
		ttl:      ttl,
		loader:   loader,
	}
}

// Get returns the cached value for key. On miss it invokes the loader,
// using singleflight so concurrent misses share a single load.
func (c *TTLCache[V]) Get(ctx context.Context, key string) (V, error) {
	for {
		if err := ctx.Err(); err != nil {
			var zero V
			return zero, err
		}
		c.mu.RLock()
		gen := c.generation
		entry, ok := c.entries[key]
		c.mu.RUnlock()
		if ok && time.Now().Before(entry.expiresAt) {
			return entry.value, nil
		}
		result := c.flight.DoChan(fmt.Sprintf("%d:%s", gen, key), func() (any, error) {
			c.mu.RLock()
			e, ok := c.entries[key]
			c.mu.RUnlock()
			if ok && time.Now().Before(e.expiresAt) {
				return e.value, nil
			}
			started := time.Now()
			value, err := c.loader(ctx, key)
			if err != nil {
				return nil, err
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if gen == c.generation {
				if c.capacity > 0 && len(c.entries) >= c.capacity {
					for k := range c.entries {
						delete(c.entries, k)
						break
					}
				}
				c.entries[key] = ttlEntry[V]{value: value, expiresAt: started.Add(c.ttl)}
			}
			return value, nil
		})
		select {
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		case loaded := <-result:
			c.mu.RLock()
			changed := gen != c.generation
			c.mu.RUnlock()
			if changed {
				continue
			}
			if loaded.Err != nil {
				var zero V
				return zero, loaded.Err
			}
			value, _ := loaded.Val.(V)
			return value, nil
		}
	}
}

// Invalidate also retires in-flight loads; their callers retry in a new generation.
func (c *TTLCache[V]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	delete(c.entries, key)
}

// Purge reconciles a committed database revision, including cached absence.
func (c *TTLCache[V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	clear(c.entries)
}

// Len returns the current entry count. Useful for tests and metrics.
func (c *TTLCache[V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *TTLCache[V]) fetch(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		var zero V
		return zero, false
	}
	if time.Now().After(e.expiresAt) {
		c.mu.Lock()
		// Re-check before delete to avoid removing a freshly-replaced entry.
		if cur, ok := c.entries[key]; ok && time.Now().After(cur.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *TTLCache[V]) put(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.capacity > 0 && len(c.entries) >= c.capacity {
		// Evict an arbitrary entry. Picks the first one map iteration yields,
		// which Go intentionally randomises. Good enough for our use case;
		// upgrade to LRU only if metrics show this hurts hit rate.
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[key] = ttlEntry[V]{value: value, expiresAt: time.Now().Add(c.ttl)}
}
