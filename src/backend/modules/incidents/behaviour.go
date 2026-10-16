// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/forecast"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Detector thresholds. A finding must be unmistakable in the numbers before it
// is shown: a behavioural finding people learn to ignore is worse than none.
const (
	climbMinDays     = forecast.MinDays
	climbMinR2       = 0.8
	climbMinGrowth   = 0.20
	forecastMinR2    = forecast.MinR2
	forecastMaxDays  = forecast.MaxDays
	restartMinDays   = 3
	restartSlot      = 20 * time.Minute
	deployMinAfter   = 5
	deployRatio      = 3.0
	findingStaleness = 48 * time.Hour
)

// Memory look-back bounds. The floor is the window the detectors shipped with;
// the ceiling keeps the ascending, row-limited history reads from truncating
// away the newest samples on a host that retains years of rollups.
const (
	memoryLookbackMinDays = 14
	memoryLookbackMaxDays = 90
)

// memoryLookbackDays is how far back the memory detectors read: the configured
// rollup retention, clamped so a short retention keeps the old 14 days and a
// long one stays inside what the row limits can hold.
func memoryLookbackDays(retentionDays int) int {
	switch {
	case retentionDays < memoryLookbackMinDays:
		return memoryLookbackMinDays
	case retentionDays > memoryLookbackMaxDays:
		return memoryLookbackMaxDays
	default:
		return retentionDays
	}
}

// Sample is one point of a series the detectors fit.
type Sample = forecast.Sample

// Fit is an ordinary least-squares line through a series, per day.
type Fit = forecast.Fit

// LinearFit fits value against time in days. It is core/forecast's line under
// this package's historical name, so capacity forecasting and the detectors
// cannot disagree about what a trend is.
func LinearFit(pts []Sample) (Fit, bool) {
	return forecast.Linear(pts)
}

// MemoryClimb reports a container whose memory has risen steadily: a week or
// more of data, a straight enough line, and at least a fifth more than it started.
func MemoryClimb(pts []Sample) (map[string]float64, bool) {
	fit, ok := LinearFit(pts)
	if !ok || fit.Days < climbMinDays || fit.R2 < climbMinR2 || fit.SlopePerDay <= 0 || fit.Intercept <= 0 {
		return nil, false
	}
	end := fit.Intercept + fit.SlopePerDay*fit.Days
	growth := (end - fit.Intercept) / fit.Intercept
	if growth < climbMinGrowth {
		return nil, false
	}
	return map[string]float64{
		"days": round(fit.Days, 1), "r2": round(fit.R2, 3), "growthPercent": round(growth*100, 1),
		"bytesPerDay": math.Round(fit.SlopePerDay), "startBytes": math.Round(fit.Intercept), "endBytes": math.Round(end),
	}, true
}

// HostMemoryForecast reports a host whose memory use, at its current rate,
// reaches 100% within a month.
func HostMemoryForecast(pts []Sample) (map[string]float64, bool) {
	return CapacityForecast(pts, 100)
}

// CapacityForecast projects a percentage series onto ceiling and returns the
// metric bag a finding stores. Every number here is rounded on the way in, so
// two runs over the same series write the same row and never churn last_seen_at.
func CapacityForecast(pts []Sample, ceiling float64) (map[string]float64, bool) {
	p, refusal := forecast.Exhaustion(pts, ceiling)
	if refusal != forecast.RefusalNone {
		return nil, false
	}
	return map[string]float64{
		"currentPercent": round(p.CurrentPercent, 1), "percentPerDay": round(p.PercentPerDay, 2),
		"daysUntilFull": round(p.DaysUntil, 1), "r2": round(p.R2, 3), "days": round(p.Days, 1),
		"ceilingPercent": round(p.Ceiling, 1),
	}, true
}

// ScheduledRestarts reports a container that restarts at the same time of day
// on several different days — the 02:40 restart nobody noticed.
func ScheduledRestarts(times []time.Time) (map[string]float64, bool) {
	slots := map[int]map[string]bool{}
	for _, t := range times {
		t = t.UTC()
		slot := (t.Hour()*60 + t.Minute()) / int(restartSlot.Minutes())
		if slots[slot] == nil {
			slots[slot] = map[string]bool{}
		}
		slots[slot][t.Format("2006-01-02")] = true
	}
	best, bestDays := -1, 0
	for slot, days := range slots {
		if len(days) > bestDays || (len(days) == bestDays && slot < best) {
			best, bestDays = slot, len(days)
		}
	}
	if bestDays < restartMinDays {
		return nil, false
	}
	minute := best * int(restartSlot.Minutes())
	return map[string]float64{"hour": float64(minute / 60), "minute": float64(minute % 60), "days": float64(bestDays)}, true
}

// PostDeployFailures reports a promotion after which failing exits rose
// sharply: at least five in the day after, three times the day before.
func PostDeployFailures(before, after int) (map[string]float64, bool) {
	if after < deployMinAfter || float64(after) < deployRatio*math.Max(float64(before), 1) {
		return nil, false
	}
	return map[string]float64{"failuresBefore": float64(before), "failuresAfter": float64(after)}, true
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// Behaviour runs the detectors over stored history and keeps findings current.
type Behaviour struct {
	DB  *sql.DB
	Now func() time.Time
}

func (b *Behaviour) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Run evaluates every detector once and returns how many findings are open.
func (b *Behaviour) Run(ctx context.Context) (int, error) {
	now := b.now().UTC()
	since := now.AddDate(0, 0, -memoryLookbackDays(coreSettings.ReadRetentionSettingsContext(ctx, b.DB).MetricsRollupDays))
	seen := map[string]bool{}
	record := func(env, subjectType, subject, kind string, metrics map[string]float64) error {
		id, err := b.upsert(ctx, env, subjectType, subject, kind, metrics, now)
		if err != nil {
			return err
		}
		seen[id] = true
		return nil
	}
	for _, step := range []func() error{
		func() error { return b.memoryClimbs(ctx, since, record) },
		func() error { return b.hostForecasts(ctx, since, record) },
		func() error { return b.scheduledRestarts(ctx, since, record) },
		func() error { return b.postDeploy(ctx, now, record) },
		func() error { return b.capacityForecasts(ctx, now, record) },
	} {
		if err := step(); err != nil {
			return 0, err
		}
	}
	if err := b.store().clearStaleFindings(ctx, now.Format(time.RFC3339), now.Add(-findingStaleness).Format(time.RFC3339)); err != nil {
		return 0, err
	}
	return len(seen), nil
}

// containerMemoryRowCap and hostMemoryRowCap bound the two memory reads. The
// reads run newest-first so that, when a large fleet hits the cap, it is the
// oldest buckets that fall off rather than the ones the forecast depends on;
// each series is reversed back to time order before the fit. Variables so a
// test can reach the cap without seeding 200,000 rows.
var (
	containerMemoryRowCap = 200000
	hostMemoryRowCap      = 100000
)

type recordFn func(env, subjectType, subject, kind string, metrics map[string]float64) error

func (b *Behaviour) store() *Store { return NewStore(b.DB) }

func (b *Behaviour) memoryClimbs(ctx context.Context, since time.Time, record recordFn) error {
	points, err := b.store().containerMemoryRollup(ctx, since.Format(metricsLayout), containerMemoryRowCap)
	if err != nil {
		return err
	}
	series := map[[2]string][]Sample{}
	for _, p := range points {
		t, err := time.Parse(metricsLayout, p.at)
		if err != nil || p.name == "" {
			continue
		}
		k := [2]string{p.env, p.name}
		series[k] = append(series[k], Sample{At: t, Value: p.value})
	}
	for k, pts := range series {
		slices.Reverse(pts)
		if m, ok := MemoryClimb(pts); ok {
			if err := record(k[0], "container", k[1], FindingMemoryClimb, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Behaviour) hostForecasts(ctx context.Context, since time.Time, record recordFn) error {
	points, err := b.store().hostMemoryRollup(ctx, since.Format(metricsLayout), hostMemoryRowCap)
	if err != nil {
		return err
	}
	series := map[string][]Sample{}
	for _, p := range points {
		if t, err := time.Parse(metricsLayout, p.at); err == nil {
			series[p.env] = append(series[p.env], Sample{At: t, Value: p.value})
		}
	}
	for env, pts := range series {
		slices.Reverse(pts)
		if m, ok := HostMemoryForecast(pts); ok {
			if err := record(env, "host", env, FindingHostMemoryFull, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Behaviour) scheduledRestarts(ctx context.Context, since time.Time, record recordFn) error {
	events, err := b.store().restartEvents(ctx, since.Format(time.RFC3339))
	if err != nil {
		return err
	}
	byContainer := map[[2]string][]time.Time{}
	for _, e := range events {
		if t, err := time.Parse(time.RFC3339, e.at); err == nil && e.name != "" {
			k := [2]string{e.env, e.name}
			byContainer[k] = append(byContainer[k], t)
		}
	}
	for k, times := range byContainer {
		if m, ok := ScheduledRestarts(times); ok {
			if err := record(k[0], "container", k[1], FindingScheduledRestart, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Behaviour) postDeploy(ctx context.Context, now time.Time, record recordFn) error {
	rows, err := b.store().succeededPromotions(ctx, now.AddDate(0, 0, -7).Format(time.RFC3339), now.Add(-24*time.Hour).Format(time.RFC3339))
	if err != nil {
		return err
	}
	type promo struct {
		env, pipeline string
		at            time.Time
	}
	var promos []promo
	for _, r := range rows {
		if t, err := time.Parse(time.RFC3339, r.at); err == nil && r.env != "" {
			promos = append(promos, promo{env: r.env, pipeline: r.pipeline, at: t})
		}
	}
	for _, p := range promos {
		before, err := b.failingExits(ctx, p.env, p.at.Add(-24*time.Hour), p.at)
		if err != nil {
			return err
		}
		after, err := b.failingExits(ctx, p.env, p.at, p.at.Add(24*time.Hour))
		if err != nil {
			return err
		}
		if m, ok := PostDeployFailures(before, after); ok {
			m["promotedAt"] = float64(p.at.Unix())
			if err := record(p.env, "pipeline", p.pipeline, FindingPostDeployErrors, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Behaviour) failingExits(ctx context.Context, env string, from, to time.Time) (int, error) {
	metas, err := b.store().dieEventMetadata(ctx, env, from.Format(time.RFC3339), to.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, meta := range metas {
		if code := eventAttrs(meta)["exitCode"]; code != "" && code != "0" {
			n++
		}
	}
	return n, nil
}

func (b *Behaviour) upsert(ctx context.Context, env, subjectType, subject, kind string, metrics map[string]float64, now time.Time) (string, error) {
	raw, err := json.Marshal(metrics)
	if err != nil {
		return "", fmt.Errorf("encoding finding: %w", err)
	}
	return b.store().upsertFinding(ctx, env, subjectType, subject, kind, string(raw), now.Format(time.RFC3339))
}

const explainSystem = `You explain a behavioural finding about a container or host to an operator.
The numbers were computed by a deterministic detector and are correct; restate them exactly and do not compute new ones.
Write two or three plain sentences: what the pattern is, why it matters, and what someone might look at first.
Never suggest a command that changes anything.`

// ExplainFinding has the model put a finding's numbers into words, under the
// same per-environment opt-in as narration. The numbers stay the detector's.
func (s *Service) ExplainFinding(ctx context.Context, id, by string) (*Finding, error) {
	f, err := s.Store.Finding(ctx, id)
	if err != nil || f == nil {
		return f, err
	}
	st, err := s.Store.Settings(ctx, f.EnvID)
	if err != nil {
		return nil, err
	}
	if !st.NarrateOptIn {
		return f, ErrNotOptedIn
	}
	if s.Model == nil {
		return f, ErrNoModel
	}
	info, err := s.Model.Info(ctx)
	if err != nil {
		return f, ErrNoModel
	}
	raw, err := json.Marshal(map[string]any{"kind": f.Kind, "subject": f.SubjectName, "subjectType": f.SubjectType, "metrics": f.Metrics})
	if err != nil {
		return nil, fmt.Errorf("encoding finding: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	prompt := "Finding (JSON):\n" + string(raw)
	text, err := s.Model.Complete(callCtx, explainSystem, prompt, 400)
	if err != nil {
		return f, fmt.Errorf("%w: %w", ErrModelFailed, err)
	}
	if len(text) > 2000 {
		text = text[:2000]
	}
	sum := sha256Hex(explainSystem + "\n\n" + prompt)
	meta := ModelMeta{Provider: info.Provider, Connection: info.Connection, Model: info.Model, RequestedBy: by,
		At: s.now().UTC().Format(time.RFC3339), PromptSHA256: sum}
	if err := s.Store.saveExplanation(ctx, f.ID, text, meta); err != nil {
		return nil, err
	}
	s.audit("incident.finding.explained", nil, map[string]any{"findingId": f.ID, "kind": f.Kind, "provider": info.Provider, "model": info.Model, "requestedBy": by})
	return s.Store.Finding(ctx, f.ID)
}
