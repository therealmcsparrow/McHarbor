// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// TestTelemetryStatsSeparatesStaleFromMeasuring guards the wedged-agent signal:
// a socket still in the pool whose pongs stopped must read as stale, not live.
func TestTelemetryStatsSeparatesStaleFromMeasuring(t *testing.T) {
	pool := NewAgentPool(slog.New(slog.NewTextHandler(io.Discard, nil)))
	pool.pingInterval.Store(int64(10 * time.Second))
	now := time.Now()

	fresh := &AgentConnection{EnvID: "fresh"}
	live := &AgentConnection{EnvID: "live", lastPingAt: now.Add(-time.Second), lastPongAt: now.Add(-time.Second)}
	wedged := &AgentConnection{EnvID: "wedged", lastPingAt: now.Add(-time.Second), lastPongAt: now.Add(-45 * time.Second)}
	pool.conns = map[string]*AgentConnection{"fresh": fresh, "live": live, "wedged": wedged}

	stats := pool.TelemetryStats()
	if stats.Connected != 3 || stats.Measuring != 1 || stats.Stale != 1 {
		t.Fatalf("stats = %+v, want 3 connected, 1 measuring, 1 stale", stats)
	}
}
