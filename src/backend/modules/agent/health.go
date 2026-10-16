// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// The four health states an agent environment can be in. These are the values
// stored in agent_health_samples.health and returned as Info.Health, and
// they are deliberately separate from the connection Status: an agent can be
// connected and degraded at the same time, which is exactly the state a plain
// connected/disconnected flag could never express.
const (
	// HealthHealthy - socket up, keepalive answered recently, link fast.
	HealthHealthy = "healthy"
	// HealthDegraded - socket up, but the keepalive is stale or the link slow.
	HealthDegraded = "degraded"
	// HealthUnhealthy - no socket.
	HealthUnhealthy = "unhealthy"
	// HealthUnknown - an agent environment that has never connected, so there
	// is nothing to judge yet. Distinguished from unhealthy so a freshly
	// created environment awaiting its first agent does not read as broken.
	HealthUnknown = "unknown"
)

// staleKeepaliveFactor is how many ping intervals may pass with no pong before
// the link counts as degraded.
//
// Two, not one: a pong that arrives a moment after the next ping was sent is
// ordinary jitter on a busy or distant host, and flagging it would make the
// strip flicker for every agent that is merely slow rather than sick.
const staleKeepaliveFactor = 2

// degradedLatencyMs is the round-trip threshold above which a connected agent
// counts as degraded.
//
// The figure comes from what the transport actually costs at this latency: a
// one-shot container stats call over the agent is documented at roughly two
// seconds, and that is with a healthy link. A keepalive round trip past two
// seconds means ordinary Docker operations through this agent are already into
// timeout territory, which is worth showing before the user finds out from a
// stalled page.
const degradedLatencyMs = 2000

// agentSampleBucketSeconds is the grid the series is snapped to. Both the
// collector and the on-read sampler truncate to this boundary, so the unique
// index on (environment_id, sampled_at) turns a collision between the two into
// a no-op rather than a duplicate block in the strip.
const agentSampleBucketSeconds = 30

// agentHealthRetentionDays matches the container series' retention. Seven days
// of 30-second samples is 20,160 rows per agent, which is the right trade for a
// 30-block strip and cheap enough not to need a rollup tier.
const agentHealthRetentionDays = 7

// maxAgentHealthLimit caps the window a caller may request, so a hand-written
// limit parameter cannot turn a card into a full-table scan.
const maxAgentHealthLimit = 200

// HealthSample is one observation of an agent's connection and health.
type HealthSample struct {
	SampledAt string `json:"sampledAt"`
	Connected bool   `json:"connected"`
	Health    string `json:"health"`
	// LatencyMs is nil when there was no live connection to measure. It is not
	// zero, because zero would draw a floor on the latency chart that reads as
	// a perfect link rather than as an absent one.
	LatencyMs *int `json:"latencyMs,omitempty"`
}

// LatencyPoint is one point of the latency chart under the health strip.
type LatencyPoint struct {
	Timestamp string `json:"timestamp"`
	Value     int    `json:"value"`
}

// HealthHistory is the payload behind the environment Overview tab's
// Agent health and Agent uptime cards.
type HealthHistory struct {
	EnvID  string `json:"envId"`
	Status string `json:"status"`
	Health string `json:"health"`

	// LatencyMs is the most recent measured round trip, nil when the agent is
	// not connected.
	LatencyMs           *int `json:"latencyMs,omitempty"`
	PingIntervalSeconds int  `json:"pingIntervalSeconds"`

	Samples               []HealthSample `json:"samples"`
	SampleIntervalSeconds int            `json:"sampleIntervalSeconds"`

	ConnectedCount int     `json:"connectedCount"`
	DownCount      int     `json:"downCount"`
	DegradedCount  int     `json:"degradedCount"`
	UptimePercent  float64 `json:"uptimePercent"`
	// Outages counts connected to disconnected transitions inside the window.
	// A flapping agent and an agent that has simply been down since before the
	// window opened produce very different numbers here and identical uptime
	// percentages, which is why both are shown.
	Outages int `json:"outages"`

	LatencySeries []LatencyPoint `json:"latencySeries"`
	AvgLatencyMs  int            `json:"avgLatencyMs"`
	MaxLatencyMs  int            `json:"maxLatencyMs"`

	LastSeen string `json:"lastSeen,omitempty"`
}

// deriveHealth turns a keepalive snapshot into one of the four health states.
//
// connected is passed separately rather than inferred from the snapshot,
// because the caller already knows it and the two must never disagree.
func deriveHealth(connected, everConnected bool, ka coreagent.Keepalive, pingInterval time.Duration, now time.Time) (string, *int) {
	if !connected {
		if !everConnected {
			return HealthUnknown, nil
		}
		return HealthUnhealthy, nil
	}

	var latency *int
	if ka.Latency > 0 {
		ms := int(ka.Latency / time.Millisecond)
		latency = &ms
	}

	staleAfter := time.Duration(staleKeepaliveFactor) * pingInterval

	// No pong yet. A connection that has only just been established has not
	// had time to answer, so it counts as healthy until a ping has actually
	// gone unanswered for the whole stale window.
	if ka.LastPong.IsZero() {
		if !ka.LastPing.IsZero() && now.Sub(ka.LastPing) > staleAfter {
			return HealthDegraded, latency
		}
		return HealthHealthy, latency
	}

	if now.Sub(ka.LastPong) > staleAfter {
		return HealthDegraded, latency
	}
	if latency != nil && *latency > degradedLatencyMs {
		return HealthDegraded, latency
	}
	return HealthHealthy, latency
}

// pingInterval reads the configured keepalive cadence, guarding against a zero
// or negative stored value that would make the stale window meaningless.
func (s *Service) pingInterval(ctx context.Context) time.Duration {
	seconds := coreSettings.ReadAgentSettingsContext(ctx, s.db).PingInterval
	if seconds <= 0 {
		seconds = coreSettings.DefaultAgentSettings().PingInterval
	}
	return time.Duration(seconds) * time.Second
}

// health returns the derived health and latency for one agent environment.
// everConnected distinguishes "never came up" from "went down".
func (s *Service) health(ctx context.Context, envID string, everConnected bool) (string, *int) {
	if s.agentPool == nil {
		return HealthUnknown, nil
	}
	ka, connected := s.agentPool.Keepalive(envID)
	return deriveHealth(connected, everConnected, ka, s.pingInterval(ctx), time.Now())
}

// currentAgentSampleBucket truncates now to the sampling grid, so the collector
// and the on-read sampler cannot write competing rows for the same moment.
func currentAgentSampleBucket() string {
	return time.Now().UTC().
		Truncate(agentSampleBucketSeconds * time.Second).
		Format(time.RFC3339)
}

// recordAgentSample appends one observation. The unique index on
// (environment_id, sampled_at) makes a second write for the same bucket a
// no-op, so the collector and an open detail page can both sample freely.
func recordAgentSample(ctx context.Context, database *sql.DB, envID string, sample HealthSample, agentVersion string, pingSeconds int) error {
	var latency any
	if sample.LatencyMs != nil {
		latency = *sample.LatencyMs
	}

	_, err := database.ExecContext(ctx, `
		INSERT INTO agent_health_samples
			(id, environment_id, connected, health, latency_ms, agent_version,
			 ping_interval_seconds, sampled_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(environment_id, sampled_at) DO NOTHING
	`, xid.New().String(), envID, boolToInt(sample.Connected), sample.Health,
		latency, nullIfEmpty(agentVersion), pingSeconds, sample.SampledAt)
	if err != nil {
		return fmt.Errorf("recording agent health sample: %w", err)
	}
	return nil
}

// listAgentSamples returns the most recent samples in chronological order,
// which is the order the strip renders them in.
func listAgentSamples(ctx context.Context, database *sql.DB, envID string, limit int) ([]HealthSample, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT sampled_at, connected, health, latency_ms
		FROM agent_health_samples
		WHERE environment_id = ?
		ORDER BY sampled_at DESC
		LIMIT ?
	`, envID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying agent health samples: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	samples := []HealthSample{}
	for rows.Next() {
		var sample HealthSample
		var connected int
		var latency sql.NullInt64
		if err := rows.Scan(&sample.SampledAt, &connected, &sample.Health, &latency); err != nil {
			return nil, fmt.Errorf("scanning agent health sample: %w", err)
		}
		sample.Connected = connected == 1
		if latency.Valid {
			ms := int(latency.Int64)
			sample.LatencyMs = &ms
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading agent health samples: %w", err)
	}

	// The query is newest-first so LIMIT takes the right end of the series;
	// the strip wants oldest-first so new blocks arrive from the right.
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}
	return samples, nil
}

// pruneAgentHealth drops samples past the retention window.
func pruneAgentHealth(ctx context.Context, database *sql.DB) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -agentHealthRetentionDays).Format(time.RFC3339)
	result, err := database.ExecContext(ctx,
		`DELETE FROM agent_health_samples WHERE sampled_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning agent health samples: %w", err)
	}
	return db.RowsAffected(result), nil
}

// HealthHistory assembles the Agent health and Agent uptime payload for one
// environment. A nil result with a nil error means the environment exists but
// is not an agent environment.
//
// Reading is also a write path, the same way the container equivalent is: the
// live observation is folded into the table before the series is read back, so
// the cards are correct on the very first page load rather than blank until the
// collector's next tick.
func (s *Service) HealthHistory(ctx context.Context, envID string, limit int) (*HealthHistory, error) {
	if limit <= 0 || limit > maxAgentHealthLimit {
		limit = 30
	}

	info, err := s.AgentStatus(ctx, envID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}

	pingSeconds := int(s.pingInterval(ctx) / time.Second)

	out := &HealthHistory{
		EnvID:                 envID,
		Status:                info.Status,
		Health:                info.Health,
		LatencyMs:             info.LatencyMs,
		PingIntervalSeconds:   pingSeconds,
		SampleIntervalSeconds: agentSampleBucketSeconds,
		LastSeen:              info.LastSeen,
		Samples:               []HealthSample{},
		LatencySeries:         []LatencyPoint{},
	}

	live := HealthSample{
		SampledAt: currentAgentSampleBucket(),
		Connected: info.Status == "connected",
		Health:    info.Health,
		LatencyMs: info.LatencyMs,
	}
	if err := recordAgentSample(ctx, s.db, envID, live, info.AgentVersion, pingSeconds); err != nil {
		return nil, err
	}

	samples, err := listAgentSamples(ctx, s.db, envID, limit)
	if err != nil {
		return nil, err
	}
	out.Samples = samples
	summariseAgentSamples(out)
	return out, nil
}

// summariseAgentSamples fills the counters and the latency series from the
// sample slice already on the aggregate.
func summariseAgentSamples(out *HealthHistory) {
	var latencyTotal, latencyCount int
	previousConnected := true

	for i, sample := range out.Samples {
		if sample.Connected {
			out.ConnectedCount++
		} else {
			out.DownCount++
			// A window that opens on a down sample has no transition to
			// attribute the outage to, so only a fall from connected counts.
			if i > 0 && previousConnected {
				out.Outages++
			}
		}
		if sample.Health == HealthDegraded {
			out.DegradedCount++
		}
		if sample.LatencyMs != nil {
			out.LatencySeries = append(out.LatencySeries, LatencyPoint{
				Timestamp: sample.SampledAt,
				Value:     *sample.LatencyMs,
			})
			latencyTotal += *sample.LatencyMs
			latencyCount++
			if *sample.LatencyMs > out.MaxLatencyMs {
				out.MaxLatencyMs = *sample.LatencyMs
			}
		}
		previousConnected = sample.Connected
	}

	if total := len(out.Samples); total > 0 {
		out.UptimePercent = float64(out.ConnectedCount) / float64(total) * 100
	}
	if latencyCount > 0 {
		out.AvgLatencyMs = latencyTotal / latencyCount
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
