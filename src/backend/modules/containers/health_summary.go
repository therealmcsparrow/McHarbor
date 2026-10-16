// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Fleet-wide availability, derived from the health and uptime history.
//
// The Overview tab's cards answer "how is this one container doing right
// now"; this file answers "how did every container do over the last thirty
// days", which is the question a report asks. The two read the same tables,
// but nothing here is per-container-request: a report that had to issue one
// HTTP call per workload would take minutes on a busy host.
//
// Two storage tiers are stitched together:
//
//   - container_state_samples / container_health_probes — the raw 30s series,
//     kept for healthHistoryRetentionDays (7). Authoritative, and the only
//     source for today.
//   - container_uptime_daily — one row per container per day, written by the
//     collector just before it prunes, kept for uptimeRollupRetentionDays.
//     The only source for anything older than a week.
//
// Where both cover the same day the raw rows win, because the rollup for a
// day is written once and the raw rows for that day may still be growing.

// uptimeRollupRetentionDays is how far back the daily rollup reaches. Long
// enough for a year-over-year comparison at one row per container per day.
const uptimeRollupRetentionDays = 400

// maxSummaryContainers bounds the response. A report showing five hundred
// workloads is not a report, and an unbounded query over a fleet's worth of
// samples is not a query.
const maxSummaryContainers = 500

// HealthSummary is the fleet's availability over a window.
type HealthSummary struct {
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`

	// RequestedDays is what the caller asked for; ObservedDays is how many
	// distinct days actually carry data. A report must print the second, not
	// the first — history that was never collected is not uptime.
	RequestedDays int `json:"requestedDays"`
	ObservedDays  int `json:"observedDays"`

	SampleIntervalSeconds int `json:"sampleIntervalSeconds"`
	RawRetentionDays      int `json:"rawRetentionDays"`
	RollupRetentionDays   int `json:"rollupRetentionDays"`

	Containers []ContainerAvailability `json:"containers"`
	Totals     AvailabilityTotals      `json:"totals"`
}

// ContainerAvailability is one workload's record over the window.
type ContainerAvailability struct {
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	EnvironmentID string `json:"environmentId,omitempty"`

	FirstDay string `json:"firstDay"`
	LastDay  string `json:"lastDay"`
	Days     int    `json:"days"`

	Samples          int `json:"samples"`
	RunningSamples   int `json:"runningSamples"`
	DownSamples      int `json:"downSamples"`
	UnhealthySamples int `json:"unhealthySamples"`

	// ObservedMinutes is samples × the sampling interval — the time the
	// platform actually watched this container, which is the denominator every
	// percentage here uses. It is deliberately not "days × 1440": a container
	// created halfway through the window was not down for the first half.
	ObservedMinutes float64 `json:"observedMinutes"`
	DowntimeMinutes float64 `json:"downtimeMinutes"`
	UptimePercent   float64 `json:"uptimePercent"`
	HealthyPercent  float64 `json:"healthyPercent"`

	// Outages counts running→stopped transitions; Transitions counts changes
	// in either direction. A workload with many transitions and little
	// downtime is flapping, which the two numbers together reveal and either
	// one alone hides.
	Outages     int `json:"outages"`
	Transitions int `json:"transitions"`

	MTBFHours   float64 `json:"mtbfHours"`
	MTTRMinutes float64 `json:"mttrMinutes"`
	FlapsPerDay float64 `json:"flapsPerDay"`

	Probes           int     `json:"probes"`
	ProbesPassed     int     `json:"probesPassed"`
	ProbesFailed     int     `json:"probesFailed"`
	ProbePassPercent float64 `json:"probePassPercent"`
	MeanProbeMs      float64 `json:"meanProbeMs"`
	MaxProbeMs       int     `json:"maxProbeMs"`
	HasHealthcheck   bool    `json:"hasHealthcheck"`
	LastFailureAt    string  `json:"lastFailureAt,omitempty"`

	CurrentState  string `json:"currentState,omitempty"`
	CurrentHealth string `json:"currentHealth,omitempty"`
}

// AvailabilityTotals is the fleet roll-up of the rows above.
type AvailabilityTotals struct {
	Containers         int     `json:"containers"`
	Tracked            int     `json:"tracked"`
	WithDowntime       int     `json:"withDowntime"`
	WithHealthcheck    int     `json:"withHealthcheck"`
	Samples            int     `json:"samples"`
	ObservedMinutes    float64 `json:"observedMinutes"`
	DowntimeMinutes    float64 `json:"downtimeMinutes"`
	UptimePercent      float64 `json:"uptimePercent"`
	Outages            int     `json:"outages"`
	Transitions        int     `json:"transitions"`
	Probes             int     `json:"probes"`
	ProbesFailed       int     `json:"probesFailed"`
	ProbePassPercent   float64 `json:"probePassPercent"`
	MeanUptimePercent  float64 `json:"meanUptimePercent"`
	WorstUptimePercent float64 `json:"worstUptimePercent"`
}

// dayStat is one container's tally for one calendar day, from whichever tier
// supplied it.
type dayStat struct {
	samples, running, unhealthy int
	outages, transitions        int
	probes, probesPassed        int
	probeMsTotal, probeMsMax    int
	sampleSeconds               int
	name                        string
	envID                       string

	// containerID is the physical container the day's rows came from. It is no
	// longer the map key — a workload recreated during the window has several —
	// but a report still has to name one, and the newest is the one an operator
	// can act on.
	containerID string
}

// healthSummary aggregates the availability of every container in the window.
func (s *Service) healthSummary(ctx context.Context, envID string, from, to time.Time, limit int) (*HealthSummary, error) {
	if limit <= 0 || limit > maxSummaryContainers {
		limit = maxSummaryContainers
	}
	if to.Before(from) {
		from, to = to, from
	}

	out := &HealthSummary{
		WindowStart:           from.UTC().Format(time.RFC3339),
		WindowEnd:             to.UTC().Format(time.RFC3339),
		RequestedDays:         int(to.Sub(from).Hours()/24 + 0.5),
		SampleIntervalSeconds: sampleBucketSeconds,
		RawRetentionDays:      healthHistoryRetentionDays,
		RollupRetentionDays:   uptimeRollupRetentionDays,
		Containers:            []ContainerAvailability{},
	}
	if s.db == nil {
		return out, nil
	}

	// container IDENTITY -> day -> tally, not container id: a workload that was
	// recreated during the window is one row in the report, not one per
	// recreate. The rollup is loaded first so the raw pass can overwrite any
	// day it also covers.
	byContainer := map[string]map[string]*dayStat{}

	if err := s.loadRollupDays(ctx, envID, from, to, byContainer); err != nil {
		return nil, err
	}
	if err := s.loadRawSampleDays(ctx, envID, from, to, byContainer); err != nil {
		return nil, err
	}
	if err := s.loadRawProbeDays(ctx, envID, from, to, byContainer); err != nil {
		return nil, err
	}

	failures, err := s.lastProbeFailures(ctx, envID, from, to)
	if err != nil {
		return nil, err
	}
	current, err := s.latestStates(ctx, envID, from, to)
	if err != nil {
		return nil, err
	}

	days := map[string]bool{}
	rows := make([]ContainerAvailability, 0, len(byContainer))
	for identity, byDay := range byContainer {
		row := ContainerAvailability{}
		var firstDay, lastDay string

		for day, st := range byDay {
			days[day] = true
			if firstDay == "" || day < firstDay {
				firstDay = day
			}
			if day > lastDay {
				lastDay = day
				// Report the id from the most recent day this identity was
				// seen, so a recreated workload names its live container
				// rather than a removed one.
				if st.containerID != "" {
					row.ContainerID = st.containerID
				}
			}
			if st.name != "" {
				row.ContainerName = st.name
			}
			if st.envID != "" {
				row.EnvironmentID = st.envID
			}

			interval := st.sampleSeconds
			if interval <= 0 {
				interval = sampleBucketSeconds
			}
			row.Samples += st.samples
			row.RunningSamples += st.running
			row.UnhealthySamples += st.unhealthy
			row.Outages += st.outages
			row.Transitions += st.transitions
			row.Probes += st.probes
			row.ProbesPassed += st.probesPassed
			row.ObservedMinutes += float64(st.samples*interval) / 60
			row.DowntimeMinutes += float64((st.samples-st.running)*interval) / 60
			row.MeanProbeMs += float64(st.probeMsTotal)
			if st.probeMsMax > row.MaxProbeMs {
				row.MaxProbeMs = st.probeMsMax
			}
		}

		row.Days = len(byDay)
		row.FirstDay, row.LastDay = firstDay, lastDay
		row.DownSamples = row.Samples - row.RunningSamples
		row.ProbesFailed = row.Probes - row.ProbesPassed
		row.HasHealthcheck = row.Probes > 0
		row.LastFailureAt = failures[identity]
		if st, ok := current[identity]; ok {
			row.CurrentState, row.CurrentHealth = st.state, st.health
			if row.ContainerName == "" {
				row.ContainerName = st.name
			}
			if st.containerID != "" {
				row.ContainerID = st.containerID
			}
		}

		if row.Samples > 0 {
			row.UptimePercent = float64(row.RunningSamples) / float64(row.Samples) * 100
			row.HealthyPercent = float64(row.Samples-row.UnhealthySamples) / float64(row.Samples) * 100
		}
		if row.Probes > 0 {
			row.ProbePassPercent = float64(row.ProbesPassed) / float64(row.Probes) * 100
			row.MeanProbeMs /= float64(row.Probes)
		} else {
			row.MeanProbeMs = 0
		}
		if row.Outages > 0 {
			row.MTTRMinutes = row.DowntimeMinutes / float64(row.Outages)
			row.MTBFHours = (row.ObservedMinutes - row.DowntimeMinutes) / float64(row.Outages) / 60
		}
		if row.Days > 0 {
			row.FlapsPerDay = float64(row.Transitions) / float64(row.Days)
		}

		rows = append(rows, row)
	}

	// Worst availability first: a report opens with what is wrong, and a
	// truncated list must keep the rows that matter.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UptimePercent != rows[j].UptimePercent {
			return rows[i].UptimePercent < rows[j].UptimePercent
		}
		return rows[i].ContainerName < rows[j].ContainerName
	})

	out.ObservedDays = len(days)
	out.Totals = availabilityTotals(rows)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out.Containers = rows
	return out, nil
}

func availabilityTotals(rows []ContainerAvailability) AvailabilityTotals {
	t := AvailabilityTotals{Containers: len(rows), WorstUptimePercent: -1}
	var uptimeSum float64
	for _, r := range rows {
		if r.Samples == 0 {
			continue
		}
		t.Tracked++
		t.Samples += r.Samples
		t.ObservedMinutes += r.ObservedMinutes
		t.DowntimeMinutes += r.DowntimeMinutes
		t.Outages += r.Outages
		t.Transitions += r.Transitions
		t.Probes += r.Probes
		t.ProbesFailed += r.ProbesFailed
		uptimeSum += r.UptimePercent
		if r.DownSamples > 0 {
			t.WithDowntime++
		}
		if r.HasHealthcheck {
			t.WithHealthcheck++
		}
		if t.WorstUptimePercent < 0 || r.UptimePercent < t.WorstUptimePercent {
			t.WorstUptimePercent = r.UptimePercent
		}
	}
	if t.ObservedMinutes > 0 {
		t.UptimePercent = (t.ObservedMinutes - t.DowntimeMinutes) / t.ObservedMinutes * 100
	}
	if t.Tracked > 0 {
		t.MeanUptimePercent = uptimeSum / float64(t.Tracked)
	}
	if t.Probes > 0 {
		t.ProbePassPercent = float64(t.Probes-t.ProbesFailed) / float64(t.Probes) * 100
	}
	if t.WorstUptimePercent < 0 {
		t.WorstUptimePercent = 0
	}
	return t
}

// dayOf returns the UTC calendar day a timestamp falls in, in the same
// YYYY-MM-DD form the rollup table and the substr() expressions use.
func dayOf(t time.Time) string { return t.UTC().Format("2006-01-02") }

// statFor returns the tally for one identity on one day, creating it on first
// use. `identity` is db.ContainerIdentityExpr's value, not a container id.
func statFor(byContainer map[string]map[string]*dayStat, identity, day string) *dayStat {
	byDay, ok := byContainer[identity]
	if !ok {
		byDay = map[string]*dayStat{}
		byContainer[identity] = byDay
	}
	st, ok := byDay[day]
	if !ok {
		st = &dayStat{sampleSeconds: sampleBucketSeconds}
		byDay[day] = st
	}
	return st
}

// loadRollupDays reads the pre-aggregated days.
func (s *Service) loadRollupDays(ctx context.Context, envID string, from, to time.Time, out map[string]map[string]*dayStat) error {
	rows, err := s.db.QueryContext(ctx,
		loadRollupDaysSQL,
		nullableEnv(envID), dayOf(from), dayOf(to),
	)
	if err != nil {
		return fmt.Errorf("querying uptime rollup: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	for rows.Next() {
		var (
			identity, id, name, env, day string
			st                           dayStat
		)
		if err := rows.Scan(&identity, &id, &name, &env, &day,
			&st.samples, &st.running, &st.unhealthy, &st.outages, &st.transitions,
			&st.probes, &st.probesPassed, &st.probeMsTotal, &st.probeMsMax, &st.sampleSeconds); err != nil {
			return fmt.Errorf("scanning uptime rollup row: %w", err)
		}
		st.name, st.envID, st.containerID = name, env, id
		target := statFor(out, identity, day)
		*target = st
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading uptime rollup rows: %w", err)
	}
	return nil
}

// loadRawSampleDays groups the raw state series by container and calendar day.
//
// The LAG window function is what turns a series of independent observations
// into transitions. Doing it in SQL rather than in Go keeps the whole series
// out of process memory: on a busy host the window can hold millions of rows,
// and only the per-day tallies come back.
func (s *Service) loadRawSampleDays(ctx context.Context, envID string, from, to time.Time, out map[string]map[string]*dayStat) error {
	rows, err := s.db.QueryContext(ctx,
		loadRawSampleDaysSQL,
		nullableEnv(envID), from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("querying state samples: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	for rows.Next() {
		var (
			identity, id, name, env, day                   string
			samples, running, unhealthy, outages, transits int
		)
		if err := rows.Scan(&identity, &id, &name, &env, &day, &samples, &running, &unhealthy, &outages, &transits); err != nil {
			return fmt.Errorf("scanning state sample tally: %w", err)
		}
		st := statFor(out, identity, day)
		st.containerID = id
		// Raw wins outright for a day it covers: the rollup for that day, if
		// any, was written from these same rows and adding them would double.
		st.samples, st.running, st.unhealthy = samples, running, unhealthy
		st.outages, st.transitions = outages, transits
		st.sampleSeconds = sampleBucketSeconds
		if name != "" {
			st.name = name
		}
		if env != "" {
			st.envID = env
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading state sample tallies: %w", err)
	}
	return nil
}

// loadRawProbeDays groups the raw probe series by container and calendar day.
func (s *Service) loadRawProbeDays(ctx context.Context, envID string, from, to time.Time, out map[string]map[string]*dayStat) error {
	rows, err := s.db.QueryContext(ctx,
		loadRawProbeDaysSQL,
		nullableEnv(envID), from.UTC().Format(probeTimeLayout), to.UTC().Format(probeTimeLayout),
	)
	if err != nil {
		return fmt.Errorf("querying health probes: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	for rows.Next() {
		var (
			identity, id, name, day     string
			probes, passed, total, max_ int
		)
		if err := rows.Scan(&identity, &id, &name, &day, &probes, &passed, &total, &max_); err != nil {
			return fmt.Errorf("scanning probe tally: %w", err)
		}
		st := statFor(out, identity, day)
		st.probes, st.probesPassed = probes, passed
		st.probeMsTotal, st.probeMsMax = total, max_
		if name != "" {
			st.name = name
		}
		if st.containerID == "" {
			st.containerID = id
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading probe tallies: %w", err)
	}
	return nil
}

// lastProbeFailures returns the most recent failing probe per container.
// Only the raw table is consulted: the rollup keeps counts, not timestamps,
// so a failure older than the raw retention window is reported as "none in
// the retained series" rather than invented.
func (s *Service) lastProbeFailures(ctx context.Context, envID string, from, to time.Time) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		lastProbeFailuresSQL,
		nullableEnv(envID), from.UTC().Format(probeTimeLayout), to.UTC().Format(probeTimeLayout),
	)
	if err != nil {
		return nil, fmt.Errorf("querying probe failures: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	out := map[string]string{}
	for rows.Next() {
		var id, at string
		if err := rows.Scan(&id, &at); err != nil {
			return nil, fmt.Errorf("scanning probe failure row: %w", err)
		}
		out[id] = at
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading probe failure rows: %w", err)
	}
	return out, nil
}

type latestState struct{ state, health, name, containerID string }

// latestStates returns each container's newest observation in the window, so
// a report can say what a workload is doing now as well as how it behaved.
func (s *Service) latestStates(ctx context.Context, envID string, from, to time.Time) (map[string]latestState, error) {
	// Ranked rather than self-joined on MAX(sampled_at): the newest sample for
	// an identity has to win across every container id that identity has worn,
	// and the join could otherwise match the same timestamp on two of them.
	rows, err := s.db.QueryContext(ctx,
		latestStatesSQL,
		nullableEnv(envID), from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("querying latest states: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	out := map[string]latestState{}
	for rows.Next() {
		var identity string
		var st latestState
		if err := rows.Scan(&identity, &st.containerID, &st.state, &st.health, &st.name); err != nil {
			return nil, fmt.Errorf("scanning latest state row: %w", err)
		}
		out[identity] = st
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading latest state rows: %w", err)
	}
	return out, nil
}

// rollupUptimeDaily folds every finished day in the raw tables into
// container_uptime_daily.
//
// Called from the prune path, immediately before the raw rows are deleted —
// the ordering is the whole point. Only days strictly before today are rolled
// up: today's rows are still arriving, and a partial day written into the
// rollup would be indistinguishable from a day with real downtime.
//
// INSERT OR REPLACE rather than INSERT OR IGNORE, so re-running for a day
// already rolled up corrects it. That matters because the collector runs the
// rollup roughly hourly, and a day near the retention edge may be visited
// several times before its raw rows expire.
func rollupUptimeDaily(ctx context.Context, database *sql.DB) (int64, error) {
	if database == nil {
		return 0, nil
	}

	today := dayOf(time.Now())

	rows, err := database.QueryContext(ctx,
		rollupUptimeDailySQL,
		today,
	)
	if err != nil {
		return 0, fmt.Errorf("rolling up state samples: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	// Keyed by identity, not container id: a workload recreated during the day
	// must fold into ONE rollup row, or the day it was updated would report two
	// partial days of uptime for the same service.
	type key struct{ identity, day string }
	agg := map[key]*dayStat{}
	containerKeys := map[string]sql.NullString{}
	for rows.Next() {
		var (
			identity, id, name, day                        string
			ckey, env                                      sql.NullString
			samples, running, unhealthy, outages, transits int
		)
		if err := rows.Scan(&identity, &id, &ckey, &name, &env, &day, &samples, &running, &unhealthy, &outages, &transits); err != nil {
			return 0, fmt.Errorf("scanning rollup row: %w", err)
		}
		agg[key{identity, day}] = &dayStat{
			samples: samples, running: running, unhealthy: unhealthy,
			outages: outages, transitions: transits,
			sampleSeconds: sampleBucketSeconds,
			name:          name, envID: env.String, containerID: id,
		}
		containerKeys[identity] = ckey
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reading rollup rows: %w", err)
	}

	probeRows, err := database.QueryContext(ctx,
		rollupUptimeDailySQL2,
		today,
	)
	if err != nil {
		return 0, fmt.Errorf("rolling up health probes: %w", err)
	}
	defer func() {
		if err := probeRows.Close(); err != nil {
			slog.Warn("closing probeRows failed", "error", err)
		}
	}()

	for probeRows.Next() {
		var (
			identity, id, day           string
			ckey                        sql.NullString
			probes, passed, total, max_ int
		)
		if err := probeRows.Scan(&identity, &id, &ckey, &day, &probes, &passed, &total, &max_); err != nil {
			return 0, fmt.Errorf("scanning probe rollup row: %w", err)
		}
		k := key{identity, day}
		st, ok := agg[k]
		if !ok {
			// Probes without a state sample for the same day: unusual, but a
			// container inspected from an open detail page on an agent
			// environment produces exactly that. Keep the probe record.
			st = &dayStat{sampleSeconds: sampleBucketSeconds, containerID: id}
			agg[k] = st
			containerKeys[identity] = ckey
		}
		st.probes, st.probesPassed = probes, passed
		st.probeMsTotal, st.probeMsMax = total, max_
	}
	if err := probeRows.Err(); err != nil {
		return 0, fmt.Errorf("reading probe rollup rows: %w", err)
	}

	var written int64
	// bounded by the container-days of one rollup pass; INSERT OR REPLACE makes a re-run safe
	for k, st := range agg {
		var env any
		if st.envID != "" {
			env = st.envID
		}
		// NULL rather than "" so a day folded entirely from pre-083 rows stays
		// on the readers' legacy identity branch.
		var ckey any
		if v := containerKeys[k.identity]; v.Valid && v.String != "" {
			ckey = v.String
		}
		result, err := database.ExecContext(ctx,
			`INSERT OR REPLACE INTO container_uptime_daily
			 (id, environment_id, container_id, container_name, container_key, day, samples, running_samples,
			  unhealthy_samples, outages, transitions, probes, probes_passed,
			  probe_ms_total, probe_ms_max, sample_seconds, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			xid.New().String(), env, st.containerID, st.name, ckey, k.day,
			st.samples, st.running, st.unhealthy, st.outages, st.transitions,
			st.probes, st.probesPassed, st.probeMsTotal, st.probeMsMax, st.sampleSeconds,
		)
		if err != nil {
			return written, fmt.Errorf("writing uptime rollup: %w", err)
		}
		written += db.RowsAffected(result)
	}

	return written, nil
}

// pruneUptimeRollup drops rollup days past the long retention window.
func pruneUptimeRollup(ctx context.Context, database *sql.DB) (int64, error) {
	if database == nil {
		return 0, nil
	}
	cutoff := dayOf(time.Now().AddDate(0, 0, -uptimeRollupRetentionDays))
	result, err := database.ExecContext(ctx,
		`DELETE FROM container_uptime_daily WHERE day < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning uptime rollup: %w", err)
	}
	return db.RowsAffected(result), nil
}

const loadRollupDaysSQL = `SELECT ` + db.ContainerIdentityExpr + `, container_id,
		        COALESCE(container_name, ''), COALESCE(environment_id, ''), day,
		        samples, running_samples, unhealthy_samples, outages, transitions,
		        probes, probes_passed, probe_ms_total, probe_ms_max, sample_seconds
		 FROM container_uptime_daily
		 WHERE environment_id IS ? AND day >= ? AND day <= ?
		 ORDER BY day
		 LIMIT 200000`

const loadRawSampleDaysSQL = `WITH ordered AS (
		     SELECT ` + db.ContainerIdentityExpr + ` AS identity,
		            container_id, container_name, environment_id, running, health, sampled_at,
		            LAG(running) OVER (PARTITION BY ` + db.ContainerIdentityExpr + ` ORDER BY sampled_at) AS prev
		     FROM container_state_samples
		     WHERE environment_id IS ? AND sampled_at >= ? AND sampled_at <= ?
		 )
		 SELECT identity,
		        MAX(container_id),
		        COALESCE(MAX(container_name), ''),
		        COALESCE(MAX(environment_id), ''),
		        substr(sampled_at, 1, 10) AS day,
		        COUNT(*),
		        SUM(running),
		        SUM(CASE WHEN health = 'unhealthy' THEN 1 ELSE 0 END),
		        SUM(CASE WHEN prev = 1 AND running = 0 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN prev IS NOT NULL AND prev <> running THEN 1 ELSE 0 END)
		 FROM ordered
		 GROUP BY identity, day
		 LIMIT 200000`

const loadRawProbeDaysSQL = `SELECT ` + db.ContainerIdentityExpr + ` AS identity,
		        MAX(container_id),
		        COALESCE(MAX(container_name), ''),
		        substr(probe_start, 1, 10) AS day,
		        COUNT(*),
		        SUM(CASE WHEN exit_code = 0 THEN 1 ELSE 0 END),
		        COALESCE(SUM(duration_ms), 0),
		        COALESCE(MAX(duration_ms), 0)
		 FROM container_health_probes
		 WHERE environment_id IS ? AND probe_start >= ? AND probe_start <= ?
		 GROUP BY identity, day
		 LIMIT 200000`

const lastProbeFailuresSQL = `SELECT ` + db.ContainerIdentityExpr + ` AS identity, MAX(probe_start)
		 FROM container_health_probes
		 WHERE environment_id IS ? AND probe_start >= ? AND probe_start <= ? AND exit_code <> 0
		 GROUP BY identity
		 LIMIT 1000`

const latestStatesSQL = `SELECT identity, container_id, state, health, container_name FROM (
		     SELECT ` + db.ContainerIdentityExpr + ` AS identity,
		            container_id, state,
		            COALESCE(health, '') AS health,
		            COALESCE(container_name, '') AS container_name,
		            ROW_NUMBER() OVER (
		                PARTITION BY ` + db.ContainerIdentityExpr + ` ORDER BY sampled_at DESC
		            ) AS rn
		     FROM container_state_samples
		     WHERE environment_id IS ? AND sampled_at >= ? AND sampled_at <= ?
		 ) ranked
		 WHERE rn = 1
		 LIMIT 1000`

const rollupUptimeDailySQL = `WITH ordered AS (
		     SELECT ` + db.ContainerIdentityExpr + ` AS identity,
		            container_id, container_name, container_key, environment_id, running, health, sampled_at,
		            LAG(running) OVER (PARTITION BY ` + db.ContainerIdentityExpr + ` ORDER BY sampled_at) AS prev
		     FROM container_state_samples
		 )
		 SELECT identity,
		        MAX(container_id),
		        MAX(container_key),
		        COALESCE(MAX(container_name), ''),
		        MAX(environment_id),
		        substr(sampled_at, 1, 10) AS day,
		        COUNT(*),
		        SUM(running),
		        SUM(CASE WHEN health = 'unhealthy' THEN 1 ELSE 0 END),
		        SUM(CASE WHEN prev = 1 AND running = 0 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN prev IS NOT NULL AND prev <> running THEN 1 ELSE 0 END)
		 FROM ordered
		 WHERE substr(sampled_at, 1, 10) < ?
		 GROUP BY identity, day
		 LIMIT 200000`

const rollupUptimeDailySQL2 = `SELECT ` + db.ContainerIdentityExpr + ` AS identity,
		        MAX(container_id), MAX(container_key),
		        substr(probe_start, 1, 10) AS day,
		        COUNT(*),
		        SUM(CASE WHEN exit_code = 0 THEN 1 ELSE 0 END),
		        COALESCE(SUM(duration_ms), 0),
		        COALESCE(MAX(duration_ms), 0)
		 FROM container_health_probes
		 WHERE substr(probe_start, 1, 10) < ?
		 GROUP BY identity, day
		 LIMIT 200000`
