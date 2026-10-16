// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"context"
	"sync"
	"time"
)

// rootfsUsageTTL is how long a measured rootfs size is served before it is
// measured again. A walk of a large rootfs costs seconds of disk reads, and a
// stats panel polling every few seconds must not repeat one per poll.
const rootfsUsageTTL = 5 * time.Minute

// rootfsUsageWalkTimeout bounds one measurement, so a wedged filesystem cannot
// hold a goroutine for the life of the agent.
const rootfsUsageWalkTimeout = time.Minute

// usageEntry is one container's last measurement and the refresh in flight.
type usageEntry struct {
	bytes   uint64
	at      time.Time
	running chan struct{}
}

// rootfsUsageCache serves each container's rootfs size from a cache that is
// refreshed in the background, at most one walk per container at a time.
type rootfsUsageCache struct {
	mu      sync.Mutex
	entries map[string]*usageEntry
	measure func(ctx context.Context, root string) (uint64, error)
	now     func() time.Time
}

// newRootfsUsageCache builds a cache that measures with measure.
func newRootfsUsageCache(measure func(context.Context, string) (uint64, error)) *rootfsUsageCache {
	return &rootfsUsageCache{
		entries: make(map[string]*usageEntry),
		measure: measure,
		now:     time.Now,
	}
}

// get returns ctID's rootfs size. A stale entry is refreshed in the background
// and the old value served meanwhile; a container never measured waits up to
// wait for its first walk, so the first sample is not a meaningless zero.
func (c *rootfsUsageCache) get(ctx context.Context, ctID, root string, wait time.Duration) uint64 {
	if c == nil || root == "" {
		return 0
	}
	c.mu.Lock()
	entry := c.entries[ctID]
	if entry == nil {
		entry = &usageEntry{}
		c.entries[ctID] = entry
	}
	stale := entry.at.IsZero() || c.now().Sub(entry.at) > rootfsUsageTTL
	if stale && entry.running == nil {
		entry.running = make(chan struct{})
		go c.refresh(context.WithoutCancel(ctx), root, entry, entry.running)
	}
	first, done, bytes := entry.at.IsZero(), entry.running, entry.bytes
	c.mu.Unlock()

	if first && done != nil && wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		}
		c.mu.Lock()
		bytes = entry.bytes
		c.mu.Unlock()
	}
	return bytes
}

// refresh measures one rootfs and records the result. A failed walk keeps the
// previous value and still stamps the entry, so a broken tree is retried once
// per TTL rather than on every sample.
func (c *rootfsUsageCache) refresh(parent context.Context, root string, entry *usageEntry, done chan struct{}) {
	ctx, cancel := context.WithTimeout(parent, rootfsUsageWalkTimeout)
	defer cancel()
	bytes, err := c.measure(ctx, root)

	c.mu.Lock()
	if err == nil {
		entry.bytes = bytes
	}
	entry.at = c.now()
	entry.running = nil
	c.mu.Unlock()
	close(done)
}

// forget drops ctID's entry, so a deleted container's size is not served to a
// new one that reuses the id.
func (c *rootfsUsageCache) forget(ctID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, ctID)
	c.mu.Unlock()
}
