// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// sampleBucketSeconds is the grid the uptime series is snapped to. Both the
// background collector and the on-read sampler truncate to this boundary so
// the two never write competing rows for the same moment — the unique index
// on (container_id, sampled_at) turns a collision into a no-op instead of a
// duplicate block in the strip.
const sampleBucketSeconds = 30

// maxProbeOutput caps the stored probe output. Healthcheck commands
// occasionally dump an entire stack trace; the card only ever shows the first
// couple of lines, and an unbounded column would bloat the database.
const maxProbeOutput = 2000

// healthHistoryRetentionDays is how long probe and sample rows are kept
// before the collector prunes them.
const healthHistoryRetentionDays = 7

// probeTimeLayout is RFC3339 with a fixed nine-digit fraction. The stdlib
// nanosecond layout trims trailing zeros, which makes the text column sort
// wrong inside a single second (".5Z" would sort after ".50001Z"). Probe rows
// are ordered by this column, so the padding is load-bearing, not cosmetic.
const probeTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// HealthProbe is a single healthcheck run, as Docker reported it.
type HealthProbe struct {
	Start      string `json:"start"`
	End        string `json:"end,omitempty"`
	ExitCode   int    `json:"exitCode"`
	DurationMs int    `json:"durationMs"`
	Output     string `json:"output,omitempty"`
}

// Passed reports whether the probe satisfied the Docker healthcheck contract
// (exit code 0 means healthy; 1 and 2 mean unhealthy, anything else means the
// probe itself failed to run).
func (p HealthProbe) Passed() bool { return p.ExitCode == 0 }

// UptimeSample is one observation of a container's run state.
type UptimeSample struct {
	SampledAt string `json:"sampledAt"`
	State     string `json:"state"`
	Running   bool   `json:"running"`
	Health    string `json:"health,omitempty"`
}

// HealthHistory is the aggregate the Overview tab's Health and Uptime cards
// render. It combines the persisted series with the live inspect so the cards
// are correct even on the very first page load, before the collector has
// completed a tick.
type HealthHistory struct {
	ContainerID    string `json:"containerId"`
	HasHealthcheck bool   `json:"hasHealthcheck"`
	Status         string `json:"status"`
	FailingStreak  int    `json:"failingStreak"`

	IntervalSeconds int `json:"intervalSeconds"`
	TimeoutSeconds  int `json:"timeoutSeconds"`
	Retries         int `json:"retries"`

	Probes    []HealthProbe `json:"probes"`
	PassCount int           `json:"passCount"`
	FailCount int           `json:"failCount"`

	Samples               []UptimeSample `json:"samples"`
	SampleIntervalSeconds int            `json:"sampleIntervalSeconds"`
	RunningCount          int            `json:"runningCount"`
	DownCount             int            `json:"downCount"`
	UptimePercent         float64        `json:"uptimePercent"`

	CurrentState string `json:"currentState"`
	StartedAt    string `json:"startedAt,omitempty"`
	RestartCount int    `json:"restartCount"`
}

// healthHistory assembles the Health + Uptime payload for one container.
//
// Reading is also a write path on purpose: the live State.Health.Log entries
// and the current run state are folded into the tables first. Docker keeps
// only the last five probe results, so an open detail page effectively acts as
// a second collector for the container the user is looking at.
func (s *Service) healthHistory(ctx context.Context, envID, containerID string, limit int) (*HealthHistory, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}

	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	inspectCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, err := cli.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspecting container: %w", err)
	}
	info := infoRes.Container

	out := &HealthHistory{
		ContainerID:           containerID,
		SampleIntervalSeconds: sampleBucketSeconds,
		Probes:                []HealthProbe{},
		Samples:               []UptimeSample{},
	}

	name := strings.TrimPrefix(info.Name, "/")

	// The container's stable identity, and the row filter derived from it.
	// Both the probe strip and the uptime strip are keyed on this rather than
	// on the id in the URL, so recreating a container — which mints a new id —
	// no longer restarts its history from nothing.
	var labels map[string]string
	if info.Config != nil {
		labels = info.Config.Labels
	}
	key := docker.ContainerKey(labels, name)
	scope := db.NewContainerScope(key, name, containerID)

	if info.State != nil {
		out.CurrentState = string(info.State.Status)
		out.StartedAt = info.State.StartedAt
	}
	out.RestartCount = info.RestartCount

	var healthcheck *container.HealthConfig
	if info.Config != nil {
		healthcheck = info.Config.Healthcheck
	}
	if hc := healthcheck; hc != nil && len(hc.Test) > 0 && hc.Test[0] != "NONE" {
		out.HasHealthcheck = true
		out.IntervalSeconds = int(hc.Interval / time.Second)
		out.TimeoutSeconds = int(hc.Timeout / time.Second)
		out.Retries = hc.Retries
	}

	var health *container.Health
	if info.State != nil {
		health = info.State.Health
	}
	if health != nil {
		out.Status = string(health.Status)
		out.FailingStreak = health.FailingStreak
		if err := s.recordHealthProbes(ctx, envID, containerID, name, key, health.Log); err != nil {
			return nil, err
		}
	}

	if info.State != nil {
		sample := UptimeSample{
			SampledAt: currentSampleBucket(),
			State:     string(info.State.Status),
			Running:   info.State.Running,
			Health:    out.Status,
		}
		if err := s.recordStateSample(ctx, envID, containerID, name, key, sample); err != nil {
			return nil, err
		}
	}

	probes, err := s.listHealthProbes(ctx, envID, scope, limit)
	if err != nil {
		return nil, err
	}
	out.Probes = probes
	for _, p := range probes {
		if p.Passed() {
			out.PassCount++
		} else {
			out.FailCount++
		}
	}

	samples, err := s.listUptimeSamples(ctx, envID, scope, limit)
	if err != nil {
		return nil, err
	}
	out.Samples = samples
	for _, sample := range samples {
		if sample.Running {
			out.RunningCount++
		} else {
			out.DownCount++
		}
	}
	if total := len(samples); total > 0 {
		out.UptimePercent = float64(out.RunningCount) / float64(total) * 100
	}

	return out, nil
}

// listHealthProbes returns the newest `limit` probes in chronological order,
// which is the order the strip renders them in (oldest block on the left).
func (s *Service) listHealthProbes(ctx context.Context, envID string, scope db.ContainerScope, limit int) ([]HealthProbe, error) {
	if s.db == nil {
		return []HealthProbe{}, nil
	}

	rows, err := s.db.QueryContext(ctx, healthProbesSelectSQL+scope.Where+healthProbesTailSQL, scope.Args(nullableEnv(envID), limit)...) // SQL identifier: scope.Where is a fixed fragment from db.NewContainerScope, every value is bound
	if err != nil {
		return nil, fmt.Errorf("querying health probes: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	var probes []HealthProbe
	for rows.Next() {
		var p HealthProbe
		if err := rows.Scan(&p.Start, &p.End, &p.ExitCode, &p.DurationMs, &p.Output); err != nil {
			return nil, fmt.Errorf("scanning health probe row: %w", err)
		}
		probes = append(probes, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading health probe rows: %w", err)
	}

	reverseProbes(probes)
	if probes == nil {
		probes = []HealthProbe{}
	}
	return probes, nil
}

// listUptimeSamples returns the newest `limit` run-state samples in
// chronological order.
func (s *Service) listUptimeSamples(ctx context.Context, envID string, scope db.ContainerScope, limit int) ([]UptimeSample, error) {
	if s.db == nil {
		return []UptimeSample{}, nil
	}

	rows, err := s.db.QueryContext(ctx, uptimeSamplesSelectSQL+scope.Where+uptimeSamplesTailSQL, scope.Args(nullableEnv(envID), limit)...) // SQL identifier: scope.Where is a fixed fragment from db.NewContainerScope, every value is bound
	if err != nil {
		return nil, fmt.Errorf("querying uptime samples: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	var samples []UptimeSample
	for rows.Next() {
		var (
			sample  UptimeSample
			running int
		)
		if err := rows.Scan(&sample.SampledAt, &sample.State, &running, &sample.Health); err != nil {
			return nil, fmt.Errorf("scanning uptime sample row: %w", err)
		}
		sample.Running = running == 1
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading uptime sample rows: %w", err)
	}

	reverseSamples(samples)
	if samples == nil {
		samples = []UptimeSample{}
	}
	return samples, nil
}

// recordHealthProbes stores every probe Docker still remembers. The unique
// index on (container_id, probe_start) makes re-seeing the same probe on the
// next poll a no-op, so callers can hand over the whole log every time.
func (s *Service) recordHealthProbes(ctx context.Context, envID, containerID, name, key string, log []*container.HealthcheckResult) error {
	if s.db == nil || len(log) == 0 {
		return nil
	}

	// bounded by the healthcheck log Docker keeps per container; INSERT OR IGNORE makes a re-seen probe a no-op
	for _, entry := range log {
		if entry == nil || entry.Start.IsZero() {
			continue
		}

		duration := 0
		if !entry.End.IsZero() && entry.End.After(entry.Start) {
			duration = int(entry.End.Sub(entry.Start) / time.Millisecond)
		}

		end := ""
		if !entry.End.IsZero() {
			end = entry.End.UTC().Format(probeTimeLayout)
		}

		output := entry.Output
		if len(output) > maxProbeOutput {
			output = output[:maxProbeOutput]
		}

		result, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO container_health_probes
			 (id, environment_id, container_id, container_name, container_key, probe_start, probe_end,
			  exit_code, duration_ms, output)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), nullableEnv(envID), containerID, name, nullableKey(key),
			entry.Start.UTC().Format(probeTimeLayout), end,
			entry.ExitCode, duration, output,
		)
		if err != nil {
			return fmt.Errorf("recording health probe: %w", err)
		}
		_ = db.RowsAffected(result)
	}

	return nil
}

// recordStateSample stores one run-state observation. Repeated calls inside
// the same bucket collapse into the first one.
func (s *Service) recordStateSample(ctx context.Context, envID, containerID, name, key string, sample UptimeSample) error {
	if s.db == nil {
		return nil
	}

	running := 0
	if sample.Running {
		running = 1
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO container_state_samples
		 (id, environment_id, container_id, container_name, container_key, state, running, health, sampled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), nullableEnv(envID), containerID, name, nullableKey(key),
		sample.State, running, sample.Health, sample.SampledAt,
	)
	if err != nil {
		return fmt.Errorf("recording state sample: %w", err)
	}
	_ = db.RowsAffected(result)
	return nil
}

// pruneHealthHistory drops rows past the retention window.
//
// The rollup runs first, and its failure aborts the prune: deleting raw rows
// that were never folded into container_uptime_daily would destroy the only
// record of those days, and a report asking for last month would come back
// with a hole it could not explain.
func pruneHealthHistory(ctx context.Context, database *sql.DB) (int64, error) {
	if database == nil {
		return 0, nil
	}

	if _, err := rollupUptimeDaily(ctx, database); err != nil {
		return 0, fmt.Errorf("rolling up before prune: %w", err)
	}
	if _, err := pruneUptimeRollup(ctx, database); err != nil {
		return 0, fmt.Errorf("pruning rollup: %w", err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -healthHistoryRetentionDays).Format(time.RFC3339)

	probeResult, err := database.ExecContext(ctx,
		`DELETE FROM container_health_probes WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning health probes: %w", err)
	}

	sampleResult, err := database.ExecContext(ctx,
		`DELETE FROM container_state_samples WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning state samples: %w", err)
	}

	return db.RowsAffected(probeResult) + db.RowsAffected(sampleResult), nil
}

// nullableKey maps an unresolvable identity to SQL NULL rather than "", so
// such a row stays on the readers' legacy `container_key IS NULL` branch
// instead of sharing an empty-string key with every other unidentifiable
// container.
func nullableKey(key string) any {
	if key == "" {
		return nil
	}
	return key
}

// nullableEnv maps the implicit local environment ("") to SQL NULL so the
// `environment_id IS ?` comparisons match the rows the collector writes.
func nullableEnv(envID string) any {
	if envID == "" {
		return nil
	}
	return envID
}

// currentSampleBucket returns now snapped to the uptime sampling grid.
func currentSampleBucket() string {
	return time.Now().UTC().Truncate(sampleBucketSeconds * time.Second).Format(time.RFC3339)
}

func reverseProbes(probes []HealthProbe) {
	for i, j := 0, len(probes)-1; i < j; i, j = i+1, j-1 {
		probes[i], probes[j] = probes[j], probes[i]
	}
}

func reverseSamples(samples []UptimeSample) {
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}
}

const healthProbesSelectSQL = `SELECT probe_start, COALESCE(probe_end, ''), exit_code, duration_ms, COALESCE(output, '')
		 FROM container_health_probes
		 WHERE `

const healthProbesTailSQL = ` AND environment_id IS ?
		 ORDER BY probe_start DESC
		 LIMIT ?`

const uptimeSamplesSelectSQL = `SELECT sampled_at, state, running, COALESCE(health, '')
		 FROM container_state_samples
		 WHERE `

const uptimeSamplesTailSQL = ` AND environment_id IS ?
		 ORDER BY sampled_at DESC
		 LIMIT ?`
