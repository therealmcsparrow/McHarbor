// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

type fakeActuator struct {
	mu    sync.Mutex
	calls []string
	fail  error
	onAct func(action, name string)
}

func (a *fakeActuator) record(action, name string) error {
	a.mu.Lock()
	a.calls = append(a.calls, action+" "+name)
	fail, hook := a.fail, a.onAct
	a.mu.Unlock()
	if fail != nil {
		return fail
	}
	if hook != nil {
		hook(action, name)
	}
	return nil
}

func (a *fakeActuator) Start(_ context.Context, _, name string) error { return a.record("start", name) }
func (a *fakeActuator) Stop(_ context.Context, _, name string) error  { return a.record("stop", name) }
func (a *fakeActuator) Remove(_ context.Context, _, name string) error {
	return a.record("remove", name)
}
func (a *fakeActuator) Recreate(_ context.Context, _, name string, _ docker.RecreateRequest) error {
	return a.record("recreate", name)
}
func (a *fakeActuator) StackUp(_ context.Context, name string) error {
	return a.record("stack_up", name)
}

func (a *fakeActuator) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func withResponder(t *testing.T, loop *Loop, database *sql.DB) (*Responder, *fakeActuator) {
	t.Helper()
	act := &fakeActuator{}
	r := &Responder{
		Service: loop.service, Proposals: NewProposalStore(database), Settings: NewSettingsStore(database),
		Detector: loop.detector, ReaderFor: loop.inspectorFor, Actuator: act,
		Audit: coreAudit.NewLogger(database, "test-secret"), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	loop.SetResponder(r)
	return r, act
}

func setMode(t *testing.T, database *sql.DB, id, mode string) {
	t.Helper()
	if _, err := database.Exec(`UPDATE desired_states SET mode = ? WHERE id = ?`, mode, id); err != nil {
		t.Fatal(err)
	}
}

func proposalsOf(t *testing.T, database *sql.DB, id, status string) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM reconcile_proposals WHERE desired_state_id = ? AND status = ?`, id, status).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func setEnforcement(t *testing.T, database *sql.DB, enabled bool, perPass, cooldown, threshold int) {
	t.Helper()
	st := DefaultSettings()
	st.EnforceEnabled, st.MaxActionsPerPass, st.CooldownSeconds, st.FailureThreshold = enabled, perPass, cooldown, threshold
	if err := NewSettingsStore(database).SetEnforcement(context.Background(), st); err != nil {
		t.Fatal(err)
	}
}

// TestProposeModeWritesOnePlanPerDrift checks the loop writes a plan once the
// drift is confirmed, does not duplicate it, and closes it when drift clears.
func TestProposeModeWritesOnePlanPerDrift(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	_, act := withResponder(t, loop, database)
	setMode(t, database, "ds-drift", ModePropose)
	ctx := context.Background()

	loop.pass(ctx)
	if n := proposalsOf(t, database, "ds-drift", ProposalPending); n != 0 {
		t.Fatalf("a plan was written before the drift was confirmed: %d", n)
	}
	loop.pass(ctx)
	loop.pass(ctx)
	if n := proposalsOf(t, database, "ds-drift", ProposalPending); n != 1 {
		t.Fatalf("pending plans after confirmed drift = %d, want 1", n)
	}
	if act.count() != 0 {
		t.Fatalf("propose mode acted: %v", act.calls)
	}
	local.set("worker", true)
	loop.pass(ctx)
	if n := proposalsOf(t, database, "ds-drift", ProposalSuperseded); n != 1 {
		t.Fatalf("the plan was not closed when the drift cleared")
	}
}

// TestApproveAppliesOnceAndRefusesASecondApproval guards the double click: the
// plan runs once, and the second approval is refused without acting.
func TestApproveAppliesOnceAndRefusesASecondApproval(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	r, act := withResponder(t, loop, database)
	act.onAct = func(action, name string) { local.set(name, true) }
	ctx := context.Background()
	ds, err := r.Service.ByID(t.Context(), "ds-drift")
	if err != nil || ds == nil {
		t.Fatal(err)
	}
	p, err := r.ProposeNow(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Action != ActionStart {
		t.Fatalf("plan = %+v, want one start", p.Steps)
	}
	done, err := r.Approve(ctx, p.ID, "u1")
	if err != nil || done.Status != ProposalApplied || done.ResolvedBy != "u1" {
		t.Fatalf("approve: %+v %v", done, err)
	}
	if _, err := r.Approve(ctx, p.ID, "u2"); !errors.Is(err, ErrProposalNotPending) {
		t.Fatalf("second approval: %v", err)
	}
	if act.count() != 1 {
		t.Fatalf("actions = %v, want exactly one", act.calls)
	}
}

// TestApproveRefusesAStalePlan checks a plan written for drift that has since
// changed is superseded, not applied, and a fresh plan is offered.
func TestApproveRefusesAStalePlan(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	r, act := withResponder(t, loop, database)
	ctx := context.Background()
	ds, err := r.Service.ByID(t.Context(), "ds-drift")
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.ProposeNow(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	local.mu.Lock()
	local.images["worker"] = "nginx:2"
	local.mu.Unlock()
	fresh, err := r.Approve(ctx, p.ID, "u1")
	if !errors.Is(err, ErrStale) || fresh == nil || fresh.ID == p.ID {
		t.Fatalf("stale approval: %+v %v", fresh, err)
	}
	if act.count() != 0 {
		t.Fatalf("a stale plan acted: %v", act.calls)
	}
	old, err := r.Proposals.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != ProposalSuperseded {
		t.Fatalf("old plan status = %s", old.Status)
	}
}

// TestEnforceNeedsTheSwitchAndKeepsToTheBudget covers the switch defaulting
// off, the per-pass cap, and the per-row cooldown.
func TestEnforceNeedsTheSwitchAndKeepsToTheBudget(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	_, act := withResponder(t, loop, database)
	local.set("web", false)
	setMode(t, database, "ds-drift", ModeEnforce)
	setMode(t, database, "ds-sync", ModeEnforce)
	ctx := context.Background()

	loop.pass(ctx)
	loop.pass(ctx)
	if act.count() != 0 {
		t.Fatalf("enforced with the switch off: %v", act.calls)
	}
	if proposalsOf(t, database, "ds-drift", ProposalPending) != 1 || auditCount(t, database, "reconciler.enforce.deferred") != 2 {
		t.Fatalf("switched-off enforcement must fall back to one plan per row and say so")
	}

	setEnforcement(t, database, true, 1, 3600, 3)
	loop.pass(ctx)
	if act.count() != 1 {
		t.Fatalf("actions in one pass with a budget of 1 = %v", act.calls)
	}
	loop.pass(ctx)
	if act.count() != 2 {
		t.Fatalf("the second row was not enforced on the next pass: %v", act.calls)
	}
	loop.pass(ctx)
	if act.count() != 2 {
		t.Fatalf("a row was enforced again inside its cooldown: %v", act.calls)
	}
	if auditCount(t, database, "reconciler.enforce.applied") != 2 {
		t.Fatalf("enforced actions were not audited")
	}
}

// TestBreakerStopsEnforcementUntilReset checks repeated failures open the
// breaker, nothing more is attempted, and only a reset resumes.
func TestBreakerStopsEnforcementUntilReset(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	_, act := withResponder(t, loop, database)
	act.fail = errors.New("daemon said no")
	local.set("web", false)
	setMode(t, database, "ds-drift", ModeEnforce)
	setMode(t, database, "ds-sync", ModeEnforce)
	setEnforcement(t, database, true, 5, 0, 2)
	ctx := context.Background()

	loop.pass(ctx)
	loop.pass(ctx)
	st, err := NewSettingsStore(database).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.BreakerOpen || auditCount(t, database, "reconciler.enforce.breaker_opened") != 1 {
		t.Fatalf("breaker did not open after two failures: %+v", st)
	}
	tried := act.count()
	loop.pass(ctx)
	loop.pass(ctx)
	if act.count() != tried {
		t.Fatalf("enforcement continued with the breaker open: %v", act.calls)
	}
	if err := NewSettingsStore(database).Reset(ctx); err != nil {
		t.Fatal(err)
	}
	act.mu.Lock()
	act.fail = nil
	act.mu.Unlock()
	loop.pass(ctx)
	if act.count() == tried {
		t.Fatalf("enforcement did not resume after the reset")
	}
}

// TestEnforceNeverActsOnUnknownOrManual checks an unreachable environment and
// a plan with a manual step never reach the actuator.
func TestEnforceNeverActsOnUnknownOrManual(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	_, act := withResponder(t, loop, database)
	local.mu.Lock()
	delete(local.running, "web")
	local.mu.Unlock()
	setMode(t, database, "ds-far", ModeEnforce)
	setMode(t, database, "ds-sync", ModeEnforce)
	setEnforcement(t, database, true, 5, 0, 3)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		loop.pass(ctx)
	}
	if act.count() != 0 {
		t.Fatalf("acted on unknown or manual drift: %v", act.calls)
	}
	if proposalsOf(t, database, "ds-sync", ProposalPending) != 1 {
		t.Fatalf("a missing container must become a plan for a person")
	}
	if proposalsOf(t, database, "ds-far", ProposalPending) != 0 {
		t.Fatalf("an unreachable environment produced a plan")
	}
}

// TestClaimIsWonByOneCaller guards two approvals that both read the plan as
// pending: only the first claim lands, so the plan cannot run twice.
func TestClaimIsWonByOneCaller(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	r, _ := withResponder(t, loop, database)
	ctx := context.Background()
	ds, err := r.Service.ByID(t.Context(), "ds-drift")
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.ProposeNow(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Proposals.Claim(ctx, p.ID, "u1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := r.Proposals.Claim(ctx, p.ID, "u2"); !errors.Is(err, ErrProposalNotPending) {
		t.Fatalf("second claim: %v", err)
	}
}
