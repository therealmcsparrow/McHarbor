// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// rawSampleCadence is how often a pushed batch is written to history. It is the
// collector's tick, so a pushed environment's raw series has the same density as
// a pulled one while the live view still refreshes every push interval.
const rawSampleCadence = 30 * time.Second

// PushSink applies agent stats pushes to the hub and writes them to history.
type PushSink struct {
	hub    *statshub.Hub
	db     *sql.DB
	logger *slog.Logger

	mu            sync.Mutex
	lastPersisted map[string]time.Time

	// readHost supplies the real host reading for the history row. It is set
	// after construction because the reader lives on the collector, which needs
	// the agent pool.
	readHost HostReader
}

// SetHostReader makes pushed history carry the host's own CPU and memory, so a
// pushing agent's host row matches what the pull path records. Without it the
// row is the container aggregate, as before.
func (p *PushSink) SetHostReader(r HostReader) {
	p.mu.Lock()
	p.readHost = r
	p.mu.Unlock()
}

// hostReading takes one live reading for envID, or nil when none is wired or
// available. It runs on the batch's own goroutine, never the agent read loop,
// so the host_stats round trip cannot deadlock the transport.
func (p *PushSink) hostReading(ctx context.Context, envID string) *HostReading {
	p.mu.Lock()
	read := p.readHost
	p.mu.Unlock()
	if read == nil {
		return nil
	}
	reading, ok := read(ctx, envID)
	if !ok {
		return nil
	}
	return &reading
}

var _ coreagent.StatsPushSink = (*PushSink)(nil)

// NewPushSink returns the sink every agent transport hands its stats_push to.
func NewPushSink(hub *statshub.Hub, db *sql.DB, logger *slog.Logger) *PushSink {
	return &PushSink{hub: hub, db: db, logger: logger, lastPersisted: make(map[string]time.Time)}
}

// IngestStatsPush applies one batch and returns its ack. A write failure is
// acked as not accepted, which sends the batch into the agent's offline buffer
// for replay instead of losing it.
func (p *PushSink) IngestStatsPush(ctx context.Context, envID string, batch *coreagent.StatsPushPayload) coreagent.StatsPushPayload {
	ack := coreagent.StatsPushPayload{Session: batch.Session, Seq: batch.Seq}
	if !coreSettings.ReadAgentSettingsContext(ctx, p.db).MetricsEnabled {
		p.hub.Forget(envID)
		ack.Accepted = true
		ack.Stop = true
		return ack
	}

	sampledAt, err := time.Parse(time.RFC3339, batch.SampledAt)
	if err != nil {
		sampledAt = time.Now().UTC()
	}
	metrics := offlineToMetrics(batch.Samples)
	res := p.hub.Push(envID, statshub.Batch{
		Session:   batch.Session,
		Seq:       batch.Seq,
		SampledAt: sampledAt,
		Samples:   toSamples(metrics),
	})
	ack.IntervalSeconds = int(res.Interval / time.Second)
	ack.Duplicate = res.Duplicate
	ack.Accepted = true
	if !res.Applied || !p.persistDue(envID, sampledAt) || !collectionEnabled(ctx, p.db, envID) {
		return ack
	}
	if err := ingestSamples(ctx, p.db, envID, sampledAt, metrics, p.hostReading(ctx, envID)); err != nil {
		p.logger.Error("stats push: history write failed", "env", envID, "seq", batch.Seq, "error", err)
		p.forgetPersisted(envID)
		ack.Accepted = false
		ack.Error = "history write failed"
	}
	return ack
}

// persistDue claims the history slot for sampledAt when the last written batch
// is at least one raw cadence old. A little slack keeps a 10s interval landing
// on every third batch rather than drifting to every fourth.
func (p *PushSink) persistDue(envID string, sampledAt time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	last, ok := p.lastPersisted[envID]
	if ok && sampledAt.Sub(last) < rawSampleCadence-2*time.Second {
		return false
	}
	p.lastPersisted[envID] = sampledAt
	return true
}

func (p *PushSink) forgetPersisted(envID string) {
	p.mu.Lock()
	delete(p.lastPersisted, envID)
	p.mu.Unlock()
}

func collectionEnabled(ctx context.Context, db *sql.DB, envID string) bool {
	var enabled int
	if err := db.QueryRowContext(ctx,
		"SELECT collect_container_metrics_enabled FROM environments WHERE id = ? LIMIT 1", envID,
	).Scan(&enabled); err != nil {
		return true
	}
	return enabled == 1
}

func offlineToMetrics(entries []coreagent.OfflineMetricEntry) []ContainerMetric {
	out := make([]ContainerMetric, len(entries))
	for i, e := range entries {
		m := ContainerMetric{
			ID: e.ContainerID, Name: e.ContainerName, Key: e.ContainerKey,
			CPUPercent: e.CPUPercent, MemUsage: e.MemUsed, MemLimit: e.MemLimit,
			NetRx: e.NetRx, NetTx: e.NetTx, BlockRead: e.BlockRead, BlockWrite: e.BlockWrite,
			PIDs: uint64(max(e.PIDs, 0)),
		}
		if e.MemLimit > 0 {
			m.MemPercent = float64(e.MemUsed) / float64(e.MemLimit) * 100
		}
		out[i] = m
	}
	return out
}
