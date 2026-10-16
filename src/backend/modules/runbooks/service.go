// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// stepTimeout bounds every Docker or agent call a step makes. The agent
// transport has no deadline of its own, so a wedged remote host would otherwise
// hold the operator's request open forever.
const stepTimeout = 30 * time.Second

// Authorizer is the permission check a step is put through. It is the rbac
// service in production; the narrow interface is what lets the identity test
// deny one key without standing up a role graph.
type Authorizer interface {
	HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error)
}

// Operator is the person a step executes as, resolved from the request by the
// handler. There is no impersonation primitive on this platform, so a step runs
// only inside this operator's own request — never overnight, never headless.
type Operator struct {
	UserID   string
	Username string
}

// ForbiddenError reports that the operator does not hold what a step needs. It
// carries the key, because a bare 403 at 03:00 tells an operator nothing they
// can act on.
type ForbiddenError struct {
	Permission rbac.Permission
}

func (e *ForbiddenError) Error() string {
	return "runbooks: missing permission " + string(e.Permission)
}

// StepFailedError reports that an authorised step ran and failed. Message is
// what the run persists for the operator to read behind StatusError; it is
// never handed back to the client as a raw err.Error().
type StepFailedError struct {
	Message string
}

func (e *StepFailedError) Error() string { return e.Message }

// Service is the runbook engine. RuntimeFor is nil in an install with no Docker
// pool, which makes every action and diagnostic step fail with a recorded
// message rather than panicking.
type Service struct {
	Store      *Store
	Authz      Authorizer
	Audit      *audit.Logger
	Logger     *slog.Logger
	RuntimeFor func(envID string) (Runtime, error)

	// Plugins reaches plugin-backed entries through the live plugin router. Nil
	// in an install without one, which hides those entries and fails their steps.
	Plugins PluginGateway
}

// Catalog returns the actions and diagnostics a step may name, each with the
// permission it will be checked against. Plugin entries appear only while their
// plugin is loaded.
func (s *Service) Catalog() CatalogView {
	view := s.catalogView()
	view.Targets = s.TargetKindViews()
	view.AttachKinds = s.AttachKindViews()
	return view
}

// providerFor names the loaded plugin serving a runbook domain, or "" when none
// does — uninstalled, disabled, or never declared.
func (s *Service) providerFor(domain string) string {
	for _, c := range s.providers() {
		if c.Domain == domain {
			return c.Plugin
		}
	}
	return ""
}

// providers returns one runbook-provider capability per domain, from plugins that are
// loaded now. Two plugins claiming a domain resolve to the first, as the registry does.
func (s *Service) providers() []plugincaps.Capability {
	if s.Plugins == nil {
		return nil
	}
	var out []plugincaps.Capability
	seen := map[string]bool{}
	for _, c := range s.Plugins.Providers() {
		if seen[c.Domain] || !s.Plugins.Loaded(c.Plugin) {
			continue
		}
		seen[c.Domain] = true
		out = append(out, c)
	}
	return out
}

// Resolve returns the runbooks that apply here plus any run still in progress,
// which is what the alert row, the on-call inbox and the container page read.
func (s *Service) Resolve(ctx context.Context, q ResolveQuery, alertEventID string) (*ResolvedRunbooks, error) {
	books, err := s.Store.Resolve(ctx, q)
	if err != nil {
		return nil, err
	}
	open, err := s.Store.OpenRuns(ctx, q, alertEventID)
	if err != nil {
		return nil, err
	}
	return &ResolvedRunbooks{Runbooks: books, OpenRuns: open}, nil
}

// Start opens a run. The runbook must exist and be enabled; the operator comes
// from the request, so a client cannot start a run as somebody else.
func (s *Service) Start(ctx context.Context, r *http.Request, in StartInput, op Operator) (*Run, error) {
	return s.start(ctx, r, in, op, TriggerManual)
}

func (s *Service) start(ctx context.Context, r *http.Request, in StartInput, op Operator, trigger string) (*Run, error) {
	rb, err := s.Store.Get(ctx, in.RunbookID)
	if err != nil {
		return nil, err
	}
	if in.EnvironmentID == "" {
		in.EnvironmentID = rb.EnvironmentID
	}
	run, err := s.Store.StartRun(ctx, in, rb.Name, op.UserID, op.Username, trigger)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, r, op, "runbook.run.start", run, fmt.Sprintf(
		`{"runbook":%q,"runbookId":%q,"subject":%q,"trigger":%q}`, rb.Name, rb.ID, run.SubjectName, trigger))
	return run, nil
}

// RecordStep is where the feature's central claim is kept: an action step is
// authorised against this operator, on this request, every single time it is
// pressed — never once per run, because a run can span a shift and a permission
// can be revoked mid-incident.
//
// A refusal returns *ForbiddenError naming the key and performs no runtime call
// at all. A failure returns *StepFailedError and persists the message.
func (s *Service) RecordStep(ctx context.Context, r *http.Request, runID, stepID string, in StepInputResult, op Operator) (*StepResult, error) {
	run, err := s.Store.Run(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.Outcome != OutcomeInProgress {
		return nil, ErrRunFinished
	}
	steps, err := s.Store.Steps(ctx, run.RunbookID)
	if err != nil {
		return nil, err
	}
	step, ok := findStep(steps, stepID)
	if !ok {
		return nil, ErrNotFound
	}
	step = s.withSubject(step, run)

	result := StepResult{
		RunID: run.ID, StepID: step.ID, StepPosition: step.Position, StepTitle: step.Title,
		StepKind: step.Kind, State: in.State, SkipReason: in.SkipReason,
		Finding: clip(in.Finding, MaxFindingBytes), ActedBy: op.UserID, ActedByName: op.Username,
	}

	// A skip is recorded as the operator gave it. Nothing runs, and nothing is
	// checked — declining to do a thing needs no permission to do it.
	if in.State == StateSkipped {
		return s.record(ctx, r, op, run, result, "runbook.step.skip")
	}

	if step.Kind == StepInstruction {
		return s.record(ctx, r, op, run, result, "runbook.step.complete")
	}

	entry, ok := s.entryFor(step)
	if !ok {
		key := step.ActionKey
		if step.Kind == StepDiagnostic {
			key = step.DiagnosticKey
		}
		domain, _, dotted := strings.Cut(key, ".")
		if !dotted || domain == "container" || s.providerFor(domain) != "" {
			return nil, ErrCatalogUnknown
		}
		// The plugin that declared this step is gone, and with it the permissions the
		// step was gated by; nothing runs, and the run records why.
		err := fmt.Errorf("%w: no installed plugin serves %s steps, so %s cannot run", ErrPluginNotLoaded, domain, key)
		return s.failStep(ctx, r, op, run, step, result, key, err)
	}

	// The gate. Before any runtime is even resolved, so a denial provably
	// cannot have touched Docker.
	for _, perm := range entry.Required {
		allowed, err := s.Authz.HasPermission(ctx, op.UserID, run.EnvironmentID, perm)
		if err != nil {
			return nil, fmt.Errorf("checking %s for a runbook step: %w", perm, err)
		}
		if !allowed {
			return nil, &ForbiddenError{Permission: perm}
		}
	}
	if entry.SelfProtected {
		if err := s.guardSelfTarget(ctx, run.EnvironmentID, step); err != nil {
			var denied *ForbiddenError
			if errors.As(err, &denied) {
				return nil, err
			}
			return s.failStep(ctx, r, op, run, step, result, entry.Key, err)
		}
	}

	output, runErr := s.execute(ctx, op, run.EnvironmentID, step, entry)
	if runErr != nil {
		return s.failStep(ctx, r, op, run, step, result, entry.Key, runErr)
	}
	result.Output = clip(output, MaxOutputBytes)
	result.State = StateDone
	action := "runbook.step.diagnose"
	if step.Kind == StepAction {
		action = "runbook.step.act"
	}
	return s.record(ctx, r, op, run, result, action)
}

// failStep records a step that ran and failed, and returns the StepFailedError the
// handler shows the operator whole.
func (s *Service) failStep(ctx context.Context, r *http.Request, op Operator, run *Run, step Step, result StepResult, key string, runErr error) (*StepResult, error) {
	result.State = StateFailed
	result.LastError = runErr.Error()
	if s.Logger != nil {
		s.Logger.Error("runbooks: step failed", "error", runErr, "run", run.ID, "step", step.ID,
			"kind", step.Kind, "key", key)
	}
	if _, err := s.record(ctx, r, op, run, result, "runbook.step.fail"); err != nil {
		return nil, err
	}
	return nil, &StepFailedError{Message: result.LastError}
}

// execute runs the catalog entry under a deadline: a plugin entry through the
// plugin's own route as op, a host entry against the environment's runtime.
func (s *Service) execute(ctx context.Context, op Operator, envID string, step Step, entry catalogEntry) (string, error) {
	if entry.Call != nil {
		runCtx, cancel := context.WithTimeout(ctx, stepTimeout)
		defer cancel()
		return s.dispatchPlugin(runCtx, op, envID, entry, step)
	}
	if s.RuntimeFor == nil {
		return "", errors.New("no container runtime is configured for this environment")
	}
	rt, err := s.RuntimeFor(envID)
	if err != nil {
		return "", fmt.Errorf("reaching the environment: %w", err)
	}
	// The agent transport has no deadline of its own, so a wedged remote host
	// would otherwise hold the operator's request open forever.
	runCtx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	return entry.Run(runCtx, rt, step)
}

// Finish closes a run. Abandoning and escalating are outcomes, not failures —
// an abandoned run with a reason is the most informative thing in a summary.
func (s *Service) Finish(ctx context.Context, r *http.Request, runID string, in FinishInput, op Operator) (*Run, error) {
	if err := s.Store.FinishRun(ctx, runID, in.Outcome, in.Note); err != nil {
		return nil, err
	}
	run, err := s.Store.Run(ctx, runID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, r, op, "runbook.run."+in.Outcome, run, fmt.Sprintf(
		`{"outcome":%q,"stoppedAt":%d,"steps":%d}`, in.Outcome, stoppedAt(run), len(run.Results)))
	return run, nil
}

// record persists a step result and audits it as its own action, with the run
// as the entity and the real operator as the actor.
func (s *Service) record(ctx context.Context, r *http.Request, op Operator, run *Run, result StepResult, action string) (*StepResult, error) {
	saved, err := s.Store.RecordResult(ctx, result)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, r, op, action, run, fmt.Sprintf(
		`{"step":%d,"title":%q,"kind":%q,"state":%q,"reason":%q}`,
		saved.StepPosition, saved.StepTitle, saved.StepKind, saved.State, saved.SkipReason))
	return saved, nil
}

// audit writes the row with the run as its entity. It goes through Log(r, …)
// with the real request so the actor column is populated, and through
// Record(userID, username, …) when there genuinely is no request. It never
// calls Log(nil, …) — that is why every incident audit row has a NULL actor,
// and a runbook step whose actor is NULL is worthless.
func (s *Service) audit(ctx context.Context, r *http.Request, op Operator, action string, run *Run, details string) {
	if s.Audit == nil {
		return
	}
	entry := audit.Entry{
		Action:        action,
		EntityType:    "runbook_run",
		EntityID:      run.ID,
		EntityName:    run.RunbookName,
		Details:       details,
		EnvironmentID: run.EnvironmentID,
	}
	if r != nil {
		s.Audit.Log(r, entry)
		return
	}
	if err := s.Audit.RecordContext(context.WithoutCancel(ctx), op.UserID, op.Username, entry); err != nil && s.Logger != nil {
		s.Logger.Error("runbooks: failed to record step audit", "error", err, "run", run.ID)
	}
}

// ErrRunFinished is returned when a step or a second outcome is posted to a run
// that has already ended.
var ErrRunFinished = errors.New("runbooks: run already finished")

// ErrCatalogUnknown is returned for a step naming an action or diagnostic that
// is not in the allow-list, which is how an old runbook fails closed after the
// entry it used is withdrawn.
var ErrCatalogUnknown = errors.New("runbooks: action or diagnostic is not in the catalog")

func findStep(steps []Step, id string) (Step, bool) {
	for _, st := range steps {
		if st.ID == id {
			return st, true
		}
	}
	return Step{}, false
}

// withSubject fills the step's target parameter from the run's subject when the
// author did not pin one and the subject is the kind of object the entry acts
// on, so one runbook serves every container, VM or workload its alert fires for.
// A container is addressed by name; every plugin object by its id.
func (s *Service) withSubject(step Step, run *Run) Step {
	params := map[string]string{}
	for k, v := range step.ActionParams {
		params[k] = v
	}
	entry, ok := s.entryFor(step)
	name := entry.targetParam()
	if ok && name != "" && strings.TrimSpace(params[name]) == "" && s.subjectFits(run.SubjectKind, entry.Target) {
		first, second := run.SubjectID, run.SubjectName
		if entry.Target == TargetContainer {
			first, second = run.SubjectName, run.SubjectID
		}
		if first == "" {
			first = second
		}
		params[name] = first
	}
	step.ActionParams = params
	return step
}

// stoppedAt is the position of the last step the operator reached, which is the
// number a post-incident summary is actually asking for.
func stoppedAt(run *Run) int {
	last := 0
	for _, r := range run.Results {
		if r.StepPosition > last {
			last = r.StepPosition
		}
	}
	return last
}

func clip(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
