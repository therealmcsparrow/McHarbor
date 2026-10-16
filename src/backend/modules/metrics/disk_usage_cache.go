// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

// diskUsageTTL is how long one environment's disk usage is reused. The host
// page polls every 5s, and `docker system df` behind it takes seconds on a busy
// daemon while the numbers change over minutes.
const diskUsageTTL = 60 * time.Second

// diskUsageRefreshTimeout bounds one background refresh. It is detached from
// the request that triggered it, so it needs a deadline of its own.
const diskUsageRefreshTimeout = 2 * time.Minute

type diskUsageEntry struct {
	at      time.Time
	value   client.DiskUsageResult
	has     bool
	loading chan struct{}
	err     error
}

// diskUsageCache holds one environment's last disk usage. An expired value is
// still served while a single background refresh runs, so the poll that lands
// on the expiry does not wait out a full `docker system df`; only the very first
// read of an environment waits.
type diskUsageCache struct {
	mu      sync.Mutex
	entries map[string]*diskUsageEntry
	now     func() time.Time
}

var sharedDiskUsage = &diskUsageCache{entries: map[string]*diskUsageEntry{}, now: time.Now}

func (c *diskUsageCache) get(ctx context.Context, envID string, fetch func(context.Context) (client.DiskUsageResult, error)) (client.DiskUsageResult, error) {
	c.mu.Lock()
	e := c.entries[envID]
	if e == nil {
		e = &diskUsageEntry{}
		c.entries[envID] = e
	}
	if e.has {
		v := e.value
		if c.now().Sub(e.at) >= diskUsageTTL {
			c.refreshLocked(ctx, e, fetch)
		}
		c.mu.Unlock()
		return v, nil
	}
	wait := c.refreshLocked(ctx, e, fetch)
	c.mu.Unlock()

	select {
	case <-wait:
	case <-ctx.Done():
		return client.DiskUsageResult{}, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !e.has {
		return client.DiskUsageResult{}, e.err
	}
	return e.value, nil
}

// refreshLocked starts one refresh of e unless one is running, and returns the
// channel that closes when it finishes. c.mu must be held.
func (c *diskUsageCache) refreshLocked(ctx context.Context, e *diskUsageEntry, fetch func(context.Context) (client.DiskUsageResult, error)) chan struct{} {
	if e.loading != nil {
		return e.loading
	}
	done := make(chan struct{})
	e.loading = done
	go func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), diskUsageRefreshTimeout)
		v, err := fetch(rctx)
		cancel()
		c.mu.Lock()
		if err == nil {
			e.value, e.has, e.at = v, true, c.now()
		}
		e.err = err
		e.loading = nil
		c.mu.Unlock()
		close(done)
	}()
	return done
}
