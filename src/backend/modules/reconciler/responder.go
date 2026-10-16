// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// ErrStale is returned when reality moved after a plan was written; the plan is
// superseded and a fresh one takes its place.
var ErrStale = errors.New("reconciler: the drift changed since this plan was written")

// ErrUnknown is returned when the environment cannot be read, which is never a
// reason to act.
var ErrUnknown = errors.New("reconciler: the environment cannot be read right now")

// ErrNoDrift is returned when there is nothing to plan.
var ErrNoDrift = errors.New("reconciler: no drift to plan for")

// ErrCannotAct is returned when this node was built without an actuator.
var ErrCannotAct = errors.New("reconciler: this node cannot apply changes")

// SystemEnforcer is the resolver recorded on proposals enforcement applied.
const SystemEnforcer = "system:enforce"

// Responder turns drift into proposals and applies them, for the API and the
// loop alike, so a person's approval and enforcement take one path.
type Responder struct {
	Service   *Service
	Proposals *ProposalStore
	Settings  *SettingsStore
	Detector  *Detector
	ReaderFor func(envID string) (dockerReader, error)
	Actuator  Actuator
	Audit     *coreAudit.Logger
	Logger    *slog.Logger
}

// NewResponder builds the responder the API and the loop share, reading stacks
// through compose and acting through act, which main builds over the containers
// and stacks services. A nil pool yields one that cannot read or act.
func NewResponder(database *sql.DB, enc *encryption.Service, pool *docker.ClientPool, compose ComposeSource, act Actuator, auditLog *coreAudit.Logger, logger *slog.Logger) *Responder {
	r := &Responder{
		Service: NewService(database, enc), Proposals: NewProposalStore(database), Settings: NewSettingsStore(database),
		Detector: NewDetector(compose), Audit: auditLog, Logger: logger,
		ReaderFor: func(string) (dockerReader, error) { return nil, errors.New("no docker pool") },
	}
	if pool != nil {
		r.ReaderFor = func(envID string) (dockerReader, error) { return pool.Get(envID) }
		r.Actuator = act
	}
	if r.Logger == nil {
		r.Logger = slog.Default()
	}
	return r
}

func (r *Responder) detect(ctx context.Context, ds *DesiredState) (*DriftReport, dockerReader, error) {
	var cli dockerReader
	var err error
	if ds.Source == "" {
		cli, err = r.ReaderFor(ds.EnvID)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrUnknown, err)
		}
	}
	var peers []DesiredState
	if ds.Kind == KindEnvironment {
		if peers, err = r.Service.ListAll(ctx); err != nil {
			return nil, nil, err
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	report, err := r.Detector.Detect(checkCtx, cli, ds, peers)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrUnknown, err)
	}
	return report, cli, nil
}

// plan inspects the container a container-kind plan would recreate, because a
// recreate replaces env and labels wholesale and has to start from what is there.
func (r *Responder) plan(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport) ([]Step, error) {
	if ds.Source != "" {
		steps, err := planPlugin(ctx, ds, report)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnknown, err)
		}
		return steps, nil
	}
	var current *container.InspectResponse
	if ds.Kind == "" || ds.Kind == KindContainer {
		inspectCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		infoRes, err := cli.ContainerInspect(inspectCtx, ds.ContainerName, client.ContainerInspectOptions{})
		info := infoRes.Container
		cancel()
		switch {
		case err == nil:
			current = &info
		case !cerrdefs.IsNotFound(err):
			return nil, fmt.Errorf("%w: %w", ErrUnknown, err)
		}
	}
	return PlanFor(ds, report, current), nil
}

// ProposeNow checks a desired state and writes a plan for its current drift.
// An existing pending plan for the same drift is returned instead of a copy.
func (r *Responder) ProposeNow(ctx context.Context, ds *DesiredState) (*Proposal, error) {
	report, cli, err := r.detect(ctx, ds)
	if err != nil {
		return nil, err
	}
	p, _, err := r.propose(ctx, cli, ds, report, OriginPropose)
	return p, err
}

// propose returns the pending plan for this exact drift, writing one only when
// none exists; created says which happened, so callers audit a plan once.
func (r *Responder) propose(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport, origin string) (*Proposal, bool, error) {
	if !report.HasDrift {
		return nil, false, ErrNoDrift
	}
	if pending, err := r.Proposals.PendingFor(ctx, ds.ID); err != nil {
		return nil, false, err
	} else if pending != nil && pending.Fingerprint == Fingerprint(report) {
		return pending, false, nil
	}
	steps, err := r.plan(ctx, cli, ds, report)
	if err != nil {
		return nil, false, err
	}
	p, err := r.Proposals.Create(ctx, ds, origin, report, steps)
	return p, err == nil, err
}

// Approve applies a pending plan on a person's behalf. Reality is checked
// again first: a plan written for drift that has since changed is superseded,
// and the fresh plan is returned alongside ErrStale for the person to review.
func (r *Responder) Approve(ctx context.Context, proposalID, by string) (*Proposal, error) {
	if r.Actuator == nil {
		return nil, ErrCannotAct
	}
	p, err := r.Proposals.Get(ctx, proposalID)
	if err != nil || p == nil {
		return p, err
	}
	if p.Status != ProposalPending {
		return p, ErrProposalNotPending
	}
	ds, err := r.Service.ByID(ctx, p.DesiredStateID)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		if supErr := r.Proposals.SupersedePending(ctx, p.DesiredStateID, "desired state deleted"); supErr != nil {
			return nil, supErr
		}
		return p, ErrStale
	}
	report, cli, err := r.detect(ctx, ds)
	if err != nil {
		return p, err
	}
	if Fingerprint(report) != p.Fingerprint {
		if !report.HasDrift {
			if supErr := r.Proposals.SupersedePending(ctx, ds.ID, "drift resolved"); supErr != nil {
				return nil, supErr
			}
			return p, ErrStale
		}
		fresh, _, perr := r.propose(ctx, cli, ds, report, OriginPropose)
		if perr != nil {
			return nil, perr
		}
		return fresh, ErrStale
	}
	// The stored plan holds no env values, so each recreate is rebuilt from the container as
	// it is now; the fingerprint check above guarantees the drift is the one reviewed.
	fresh, err := r.plan(ctx, cli, ds, report)
	if err != nil {
		return p, err
	}
	steps := rehydrateSteps(p.Steps, fresh)
	if err := r.Proposals.Claim(ctx, p.ID, by); err != nil {
		return p, err
	}
	results, applyErr := ApplyFor(ctx, r.Actuator, ds, steps)
	status, msg := ProposalApplied, ""
	if applyErr != nil {
		status, msg = ProposalFailed, applyErr.Error()
	}
	if err := r.Proposals.Finish(ctx, p.ID, status, results, msg); err != nil {
		return nil, err
	}
	out, err := r.Proposals.Get(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Reject closes a pending plan without acting.
func (r *Responder) Reject(ctx context.Context, proposalID, by, note string) (*Proposal, error) {
	if err := r.Proposals.Resolve(ctx, proposalID, ProposalRejected, by, note, nil, ""); err != nil {
		return nil, err
	}
	return r.Proposals.Get(ctx, proposalID)
}

// enforcement is one pass's worth of enforcement budget and state.
type enforcement struct {
	settings Settings
	budget   int
	halted   bool
}

func newEnforcement(st Settings) *enforcement {
	return &enforcement{settings: st, budget: st.MaxActionsPerPass}
}

// respond applies a desired state's mode to confirmed drift from the loop. Any
// error is logged and the drift stays recorded; the loop never fails on it.
func (r *Responder) respond(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport, enf *enforcement, now time.Time) {
	switch ds.Mode {
	case ModePropose:
		r.proposeFromLoop(ctx, cli, ds, report)
	case ModeEnforce:
		r.enforce(ctx, cli, ds, report, enf, now)
	}
}

func (r *Responder) proposeFromLoop(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport) (*Proposal, bool) {
	p, created, err := r.propose(ctx, cli, ds, report, OriginPropose)
	if err != nil {
		r.Logger.Warn("reconciler: writing a proposal failed", "id", ds.ID, "env", ds.EnvID, "error", err)
		return nil, false
	}
	return p, created
}

// enforce acts on one desired state within the pass's limits. Whatever cannot
// be enforced — switch off, breaker open, cooldown, over budget, a manual step —
// falls back to a proposal, so the operator always has a plan to look at.
func (r *Responder) enforce(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport, enf *enforcement, now time.Time) {
	reason := ""
	switch {
	case r.Actuator == nil:
		reason = "no_actuator"
	case !enf.settings.EnforceEnabled:
		reason = "switched_off"
	case enf.settings.BreakerOpen || enf.halted:
		reason = "breaker_open"
	case inCooldown(ds, enf.settings, now):
		return
	}
	if reason != "" {
		if p, created := r.proposeFromLoop(ctx, cli, ds, report); created {
			r.auditEnforce(ds, "reconciler.enforce.deferred", "info", map[string]any{"reason": reason, "proposalId": p.ID})
		}
		return
	}
	steps, err := r.plan(ctx, cli, ds, report)
	if err != nil {
		r.Logger.Warn("reconciler: planning for enforcement failed", "id", ds.ID, "error", err)
		return
	}
	if !FullyApplicable(steps) || len(steps) > enf.budget || hasDisruptive(steps) {
		why := "manual_steps"
		switch {
		case !FullyApplicable(steps):
		case hasDisruptive(steps):
			why = "disruptive"
		default:
			why = "over_budget"
		}
		if p, created := r.proposeFromLoop(ctx, cli, ds, report); created {
			r.auditEnforce(ds, "reconciler.enforce.deferred", "info", map[string]any{"reason": why, "proposalId": p.ID})
		}
		return
	}
	enf.budget -= len(steps)

	p, err := r.Proposals.Create(ctx, ds, OriginEnforce, report, steps)
	if err == nil {
		err = r.Proposals.Claim(ctx, p.ID, SystemEnforcer)
	}
	if err != nil {
		r.Logger.Error("reconciler: recording an enforced action failed", "id", ds.ID, "error", err)
		return
	}
	results, applyErr := ApplyFor(ctx, r.Actuator, ds, steps)
	status, msg := ProposalApplied, ""
	if applyErr != nil {
		status, msg = ProposalFailed, applyErr.Error()
	}
	if err := r.Proposals.Finish(ctx, p.ID, status, results, msg); err != nil {
		r.Logger.Error("reconciler: recording an enforced outcome failed", "id", ds.ID, "error", err)
	}
	if err := r.Service.MarkEnforced(ctx, ds.ID, now); err != nil {
		r.Logger.Error("reconciler: recording enforcement time failed", "id", ds.ID, "error", err)
	}
	details := map[string]any{"proposalId": p.ID, "diffs": report.Diffs, "steps": redactSteps(steps), "results": results}
	if applyErr != nil {
		details["error"] = msg
		r.auditEnforce(ds, "reconciler.enforce.failed", "warning", details)
	} else {
		r.auditEnforce(ds, "reconciler.enforce.applied", "warning", details)
	}
	tripped, err := r.Settings.RecordOutcome(ctx, applyErr == nil, ds.Name+": "+msg)
	if err != nil {
		r.Logger.Error("reconciler: recording enforcement outcome failed", "error", err)
	}
	if tripped {
		enf.halted = true
		r.auditEnforce(ds, "reconciler.enforce.breaker_opened", "critical", map[string]any{
			"threshold": enf.settings.FailureThreshold, "lastError": msg,
		})
	}
}

func inCooldown(ds *DesiredState, st Settings, now time.Time) bool {
	if ds.LastEnforcedAt == "" || st.CooldownSeconds <= 0 {
		return false
	}
	last, err := time.Parse(time.RFC3339, ds.LastEnforcedAt)
	if err != nil {
		return false
	}
	return now.Sub(last) < time.Duration(st.CooldownSeconds)*time.Second
}

func (r *Responder) auditEnforce(ds *DesiredState, action, severity string, details map[string]any) {
	if r.Audit == nil {
		return
	}
	details["mode"] = ModeEnforce
	raw, err := json.Marshal(details)
	if err != nil {
		r.Logger.Error("reconciler: encoding audit details failed", "id", ds.ID, "error", err)
		raw = []byte("{}")
	}
	r.Audit.Log(nil, coreAudit.Entry{
		Action: action, Severity: severity, EntityType: "desired_state", EntityID: ds.ID,
		EntityName: ds.Name, Details: string(raw), EnvironmentID: ds.EnvID,
	})
}

// isNotPending reports the race where someone else resolved a proposal first.
func isNotPending(err error) bool { return errors.Is(err, ErrProposalNotPending) }

// reasonOf maps a responder error to the code the API returns in data.reason.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, ErrStale):
		return "stale"
	case errors.Is(err, ErrUnknown):
		return "unknown"
	case errors.Is(err, ErrNoDrift):
		return "no_drift"
	case errors.Is(err, ErrCannotAct):
		return "cannot_act"
	case isNotPending(err):
		return "not_pending"
	}
	return ""
}

// hasDisruptive reports a plan with a plugin step that interrupts service; enforce
// mode proposes those instead of acting, so a VM is never stopped unattended.
func hasDisruptive(steps []Step) bool {
	for _, s := range steps {
		if s.Disruptive {
			return true
		}
	}
	return false
}
