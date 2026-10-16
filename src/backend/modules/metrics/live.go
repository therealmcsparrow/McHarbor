// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// Pull deadlines for one full stats sweep. The agent one is the collector's
// historical bound; the local one leaves the bounded fan-out room at 300+
// containers, where the old 20s deadline was measured at 19.3s.
const (
	pullTimeoutLocal = 30 * time.Second
	pullTimeoutAgent = 60 * time.Second
)

// NewStatsHub returns the process-wide live stats hub. Its pull path is the
// bounded AllContainerStats sweep; agents that can push feed it directly.
func NewStatsHub(db *sql.DB, pool *docker.ClientPool) *statshub.Hub {
	svc := NewService(pool)
	return statshub.New(statshub.Options{
		Pull: func(ctx context.Context, envID string) ([]statshub.Sample, error) {
			metrics, err := svc.AllContainerStats(ctx, envID)
			if err != nil {
				return nil, err
			}
			return toSamples(metrics), nil
		},
		PullTimeout: func(envID string) time.Duration {
			if envID != "" && pool.IsAgentEnv(envID) {
				return pullTimeoutAgent
			}
			return pullTimeoutLocal
		},
		BaseInterval: func() time.Duration { return statshub.ReadPushInterval(db) },
	})
}

// LiveContainerStats returns envID's current per-container stats through hub,
// in the shape every live stats endpoint already serves.
func LiveContainerStats(ctx context.Context, hub *statshub.Hub, envID string) ([]ContainerMetric, error) {
	snap, err := hub.Snapshot(ctx, envID)
	if err != nil {
		return nil, err
	}
	return fromSamples(snap.Samples), nil
}

func toSamples(metrics []ContainerMetric) []statshub.Sample {
	out := make([]statshub.Sample, len(metrics))
	for i, m := range metrics {
		out[i] = statshub.Sample{
			ID: m.ID, Name: m.Name, Key: m.Key,
			CPUPercent: m.CPUPercent, MemUsage: m.MemUsage, MemLimit: m.MemLimit, MemPercent: m.MemPercent,
			NetRx: m.NetRx, NetTx: m.NetTx, BlockRead: m.BlockRead, BlockWrite: m.BlockWrite, PIDs: m.PIDs,
		}
	}
	return out
}

func fromSamples(samples []statshub.Sample) []ContainerMetric {
	out := make([]ContainerMetric, len(samples))
	for i, s := range samples {
		out[i] = ContainerMetric{
			ID: s.ID, Name: s.Name, Key: s.Key,
			CPUPercent: s.CPUPercent, MemUsage: s.MemUsage, MemLimit: s.MemLimit, MemPercent: s.MemPercent,
			NetRx: s.NetRx, NetTx: s.NetTx, BlockRead: s.BlockRead, BlockWrite: s.BlockWrite, PIDs: s.PIDs,
		}
	}
	return out
}
