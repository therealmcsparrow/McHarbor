// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// confirmPasses is the default number of consecutive passes that must see
// drift before it is recorded. A container mid-restart or mid-deploy is briefly
// "not running", and recording that would make the drift flag flicker on every
// rolling update. The operator can raise it in the reconciler settings.
const confirmPasses = 2

// maxEnvConcurrency bounds how many environments one pass checks at once; an
// agent-backed inspect is a WebSocket round trip, and a pass must not crowd out
// the requests an operator is waiting on.
const maxEnvConcurrency = 8

// checkTimeout bounds each container inspect, so one wedged agent delays only
// its own environment's rows and reads as unknown.
const checkTimeout = 15 * time.Second

type reconcileCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
}

// Loop compares every desired state with reality on an interval. Detection is
// read-only; a row in propose or enforce mode is handed to the Responder, which
// alone holds an Actuator and applies enforcement's limits.
type Loop struct {
	service      *Service
	detector     *Detector
	inspectorFor func(envID string) (dockerReader, error)
	auditLog     *coreAudit.Logger
	logger       *slog.Logger
	coord        reconcileCoordinator
	interval     time.Duration
	now          func() time.Time
	responder    *Responder
	sources      *SourceSyncer
	confirm      int

	mu      sync.Mutex
	streaks map[string]int

	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewLoop returns a loop over pool's environments. coord may be nil on a single
// node; interval <= 0 yields a loop whose Start does nothing.
func NewLoop(service *Service, pool *docker.ClientPool, compose ComposeSource, auditLog *coreAudit.Logger, logger *slog.Logger, coord reconcileCoordinator, interval time.Duration) *Loop {
	return &Loop{
		service:  service,
		detector: NewDetector(compose),
		inspectorFor: func(envID string) (dockerReader, error) {
			return pool.Get(envID)
		},
		auditLog: auditLog,
		logger:   logger,
		coord:    coord,
		interval: interval,
		now:      time.Now,
		confirm:  confirmPasses,
		streaks:  map[string]int{},
		done:     make(chan struct{}),
	}
}

// Start runs passes until Stop or ctx ends. The first pass waits a short while
// so boot-time agent reconnects do not all read as unknown.
func (l *Loop) Start(ctx context.Context) {
	if l.interval <= 0 {
		l.logger.Info("reconciler loop disabled", "interval", l.interval)
		return
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		first := time.NewTimer(15 * time.Second)
		defer first.Stop()
		select {
		case <-ctx.Done():
			return
		case <-l.done:
			return
		case <-first.C:
			quiesce.Run(func() { l.pass(ctx) })
		}
		ticker := time.NewTicker(l.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-l.done:
				return
			case <-ticker.C:
				quiesce.Run(func() { l.pass(ctx) })
			}
		}
	}()
	l.logger.Info("reconciler loop started", "interval", l.interval, "responder", l.responder != nil)
}

// SetResponder gives the loop a way to answer drift in propose and enforce
// mode. Without one every row is observed only, whatever its mode says.
func (l *Loop) SetResponder(r *Responder) {
	l.responder = r
}

// SetSources makes each pass start by syncing Git sources whose branch moved,
// so a commit changes intent within one interval. Call it before Start.
func (l *Loop) SetSources(s *SourceSyncer) {
	l.sources = s
}

// Stop ends the loop and waits for a pass in flight to finish.
func (l *Loop) Stop() {
	l.stopOnce.Do(func() { close(l.done) })
	l.wg.Wait()
}

type checkResult struct {
	ds     DesiredState
	report *DriftReport
	cli    dockerReader
	err    error
}

func (l *Loop) pass(ctx context.Context) {
	started := l.now()
	if l.coord != nil && !l.coord.LeaderOf(ctx, "reconciler") {
		telemetry.SetReconcilerOldestCheck(time.Time{})
		return
	}

	if l.sources != nil {
		l.sources.SyncDue(ctx)
	}

	states, err := l.service.ListAll(ctx)
	if err != nil {
		l.logger.Error("reconciler loop: listing desired states failed", "error", err)
		telemetry.ReconcilerPass(l.now().Sub(started), true)
		return
	}

	var enf *enforcement
	if l.responder != nil && l.responder.Settings != nil {
		st, err := l.responder.Settings.Load(ctx)
		if err != nil {
			l.logger.Error("reconciler loop: reading settings failed", "error", err)
		}
		l.confirm = st.ConfirmPasses
		enf = newEnforcement(st)
	}

	results := l.check(ctx, states)

	var observed telemetry.ReconcilerStates
	var oldest time.Time
	seen := make(map[string]bool, len(results))
	for _, res := range results {
		seen[res.ds.ID] = true
		checkedAt := l.record(ctx, res, &observed, enf)
		if !checkedAt.IsZero() && (oldest.IsZero() || checkedAt.Before(oldest)) {
			oldest = checkedAt
		}
	}

	l.mu.Lock()
	for id := range l.streaks {
		if !seen[id] {
			delete(l.streaks, id)
		}
	}
	l.mu.Unlock()

	telemetry.ReconcilerObserved(observed, oldest)
	telemetry.ReconcilerPass(l.now().Sub(started), false)
}

// check inspects every desired state, one goroutine per environment so an
// unreachable agent costs its own rows their timeout and nobody else's.
func (l *Loop) check(ctx context.Context, states []DesiredState) []checkResult {
	byEnv := map[string][]int{}
	for i, ds := range states {
		byEnv[ds.EnvID] = append(byEnv[ds.EnvID], i)
	}
	results := make([]checkResult, len(states))
	sem := make(chan struct{}, maxEnvConcurrency)
	var wg sync.WaitGroup
	for envID, idx := range byEnv {
		wg.Add(1)
		go func(envID string, idx []int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var cli dockerReader
			var err error
			needsDocker := false
			for _, i := range idx {
				needsDocker = needsDocker || states[i].Source == ""
			}
			if needsDocker {
				cli, err = l.inspectorFor(envID)
			}
			for _, i := range idx {
				results[i].ds = states[i]
				results[i].cli = cli
				if err != nil && states[i].Source == "" {
					results[i].err = err
					continue
				}
				checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
				results[i].report, results[i].err = l.detector.Detect(checkCtx, cli, &states[i], states)
				cancel()
			}
		}(envID, idx)
	}
	wg.Wait()
	return results
}

// record applies one result and returns when the row was last known to be
// checked, which is what the lag metric is measured from.
func (l *Loop) record(ctx context.Context, res checkResult, observed *telemetry.ReconcilerStates, enf *enforcement) time.Time {
	ds := res.ds
	if res.err != nil {
		observed.Unknown++
		l.logger.Debug("reconciler loop: state unknown", "id", ds.ID, "env", ds.EnvID, "error", res.err)
		return lastKnownCheck(ds)
	}

	l.mu.Lock()
	if res.report.HasDrift {
		l.streaks[ds.ID]++
	} else {
		l.streaks[ds.ID] = 0
	}
	streak := l.streaks[ds.ID]
	l.mu.Unlock()

	need := l.confirm
	if need < 1 {
		need = confirmPasses
	}
	drifted := res.report.HasDrift && (ds.DriftDetected || streak >= need)
	switch {
	case drifted:
		observed.Drifted++
	case res.report.HasDrift:
		observed.Pending++
	default:
		observed.InSync++
	}

	recorded := drifted
	if res.report.HasDrift && !drifted {
		recorded = ds.DriftDetected
	}
	mergeFirstSeen(ds.LastReport, res.report)
	now := l.now()
	if err := l.service.MarkObserved(ctx, ds.ID, recorded, res.report, now); err != nil {
		l.logger.Error("reconciler loop: recording check failed", "id", ds.ID, "error", err)
		return lastKnownCheck(ds)
	}
	if recorded != ds.DriftDetected {
		l.auditTransition(ds, res.report, recorded)
	}
	if l.responder != nil {
		switch {
		case !res.report.HasDrift:
			if err := l.responder.Proposals.SupersedePending(ctx, ds.ID, "drift resolved"); err != nil {
				l.logger.Error("reconciler loop: closing resolved proposals failed", "id", ds.ID, "error", err)
			}
		case drifted && enf != nil:
			l.responder.respond(ctx, res.cli, &ds, res.report, enf, now)
		}
	}
	return now
}

func (l *Loop) auditTransition(ds DesiredState, report *DriftReport, detected bool) {
	telemetry.ReconcilerDriftTransition(detected)
	if l.auditLog == nil {
		return
	}
	action, severity := "reconciler.drift.resolved", "info"
	if detected {
		action, severity = "reconciler.drift.detected", "warning"
	}
	details, err := json.Marshal(map[string]any{"mode": ds.Mode, "diffs": report.Diffs, "checkedAt": report.CheckedAt})
	if err != nil {
		l.logger.Error("reconciler loop: encoding audit details failed", "id", ds.ID, "error", err)
		details = []byte("{}")
	}
	l.auditLog.Log(nil, coreAudit.Entry{
		Action:        action,
		Severity:      severity,
		EntityType:    "desired_state",
		EntityID:      ds.ID,
		EntityName:    ds.Name,
		Details:       string(details),
		EnvironmentID: ds.EnvID,
	})
}

// lastKnownCheck is the row's last successful check, or its creation when it
// has never been checked, so an environment that has always been unreachable
// still shows up as lag instead of hiding at zero.
func lastKnownCheck(ds DesiredState) time.Time {
	for _, stamp := range []string{ds.LastReconcile, ds.CreatedAt} {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, stamp); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}
