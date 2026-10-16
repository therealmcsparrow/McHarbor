// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/cronspec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// ErrCronInvalid is returned for a schedule that is not a five-field cron
// expression, so a schedule the engine would never fire is refused when saved.
var ErrCronInvalid = errors.New("runbooks: schedule is not a valid cron expression")

// ErrPrincipalInvalid is returned when the chosen principal is not an active
// service principal. Automation never acts as a person.
var ErrPrincipalInvalid = errors.New("runbooks: automation needs an active service principal")

// ErrFirstStepInstruction refuses unattended mode for a runbook that opens with
// an instruction: every run of it would stop before doing anything.
var ErrFirstStepInstruction = errors.New("runbooks: the first step needs a person")

// ErrAlertNeedsCondition refuses the alert trigger on a runbook that is not
// attached to an alert type or rule, because nothing would ever match it.
var ErrAlertNeedsCondition = errors.New("runbooks: the alert trigger needs an alert attachment")

// ErrNoTrigger refuses an automation with a principal but neither trigger.
var ErrNoTrigger = errors.New("runbooks: automation needs the alert trigger, a schedule, or both")

// ErrAutomateRequired refuses an edit to an automated runbook's steps by someone
// without runbooks.automate: the steps are what the principal will execute.
var ErrAutomateRequired = errors.New("runbooks: editing an automated runbook needs runbooks.automate")

// alertLookback caps how far back the engine answers an alert. An alert that
// fired while the engine was down for an hour is stale, and restarting a
// container for it now would be acting on old news.
const alertLookback = 10 * time.Minute

// ConfigureAutomation validates and stores how a runbook runs unattended. It is
// where the decisions live: only a service principal, a cron the scheduler can
// parse, the alert trigger only on an alert attachment, and never a runbook
// whose first step is an instruction.
func (s *Service) ConfigureAutomation(ctx context.Context, runbookID string, in AutomationInput, updatedBy string) (*Runbook, error) {
	rb, err := s.Store.Get(ctx, runbookID)
	if err != nil {
		return nil, err
	}
	in.PrincipalID = strings.TrimSpace(in.PrincipalID)
	in.Cron = strings.Join(strings.Fields(in.Cron), " ")
	if in.PrincipalID == "" {
		if err := s.Store.SetAutomation(ctx, rb.ID, AutomationInput{}, "", updatedBy); err != nil {
			return nil, err
		}
		return s.Store.Get(ctx, rb.ID)
	}
	if err := validateAutomation(rb, in); err != nil {
		return nil, err
	}
	principal, err := s.Store.ServicePrincipal(ctx, in.PrincipalID)
	if errors.Is(err, ErrNotFound) || (err == nil && !principal.Active) {
		return nil, ErrPrincipalInvalid
	}
	if err != nil {
		return nil, err
	}
	next := ""
	if in.Cron != "" {
		next = s.nextRun(in.Cron)
	}
	if err := s.Store.SetAutomation(ctx, rb.ID, in, next, updatedBy); err != nil {
		return nil, err
	}
	return s.Store.Get(ctx, rb.ID)
}

// validateAutomation holds the rules that need no database, so they are the
// first thing a bad request meets.
func validateAutomation(rb *Runbook, in AutomationInput) error {
	if !in.OnAlert && in.Cron == "" {
		return ErrNoTrigger
	}
	if in.Cron != "" && !cronspec.Valid(in.Cron) {
		return ErrCronInvalid
	}
	if in.OnAlert && rb.AttachKind != AttachAlertCondition {
		return ErrAlertNeedsCondition
	}
	if len(rb.Steps) == 0 || rb.Steps[0].Kind == StepInstruction {
		return ErrFirstStepInstruction
	}
	return nil
}

// GuardAutomatedEdit refuses a step edit on an automated runbook unless userID
// holds runbooks.automate. Without it, runbooks.manage alone could change what
// a service principal executes overnight — a grant nobody made.
func (s *Service) GuardAutomatedEdit(ctx context.Context, r *http.Request, runbookID, userID string) error {
	rb, err := s.Store.Get(ctx, runbookID)
	if err != nil {
		return err
	}
	if !rb.Automation.Active() {
		return nil
	}
	allowed, err := s.Authz.HasPermission(ctx, userID, "", rbac.PermRunbooksAutomate)
	if err != nil {
		return fmt.Errorf("checking runbooks.automate: %w", err)
	}
	if !allowed {
		return ErrAutomateRequired
	}
	return nil
}

// RunUnattended follows rb once as its service principal. Each step goes
// through RecordStep — the same per-step HasPermission gate a person's run
// uses, before any runtime is resolved — with the principal as the operator.
//
// It stops at the first instruction step (escalated: a person is needed there),
// and at the first denied or failed step (abandoned). The returned run carries
// where it stopped in its note and in its results.
func (s *Service) RunUnattended(ctx context.Context, rb *Runbook, trigger string, alert *firingAlert) (*Run, error) {
	principal, err := s.Store.ServicePrincipal(ctx, rb.Automation.PrincipalID)
	if errors.Is(err, ErrNotFound) || (err == nil && !principal.Active) {
		return nil, ErrPrincipalInvalid
	}
	if err != nil {
		return nil, err
	}
	op := Operator{UserID: principal.ID, Username: principal.Username}
	run, err := s.start(ctx, nil, startFor(rb, alert), op, trigger)
	if err != nil {
		return nil, err
	}

	outcome, note := OutcomeCompleted, ""
	for _, step := range rb.Steps {
		if step.Kind == StepInstruction {
			outcome = OutcomeEscalated
			note = fmt.Sprintf("stopped at step %d (%s): an instruction needs a person", step.Position, step.Title)
			break
		}
		_, stepErr := s.RecordStep(ctx, nil, run.ID, step.ID, StepInputResult{State: StateDone}, op)
		if stepErr == nil {
			continue
		}
		outcome = OutcomeAbandoned
		var denied *ForbiddenError
		var failed *StepFailedError
		switch {
		case errors.As(stepErr, &denied):
			message := fmt.Sprintf("the service principal %s does not hold %s", principal.Username, denied.Permission)
			if _, err := s.record(ctx, nil, op, run, StepResult{
				RunID: run.ID, StepID: step.ID, StepPosition: step.Position, StepTitle: step.Title,
				StepKind: step.Kind, State: StateFailed, ActedBy: op.UserID, ActedByName: op.Username,
				LastError: message,
			}, "runbook.step.denied"); err != nil {
				return nil, err
			}
			note = fmt.Sprintf("stopped at step %d (%s): %s", step.Position, step.Title, message)
		case errors.As(stepErr, &failed):
			note = fmt.Sprintf("stopped at step %d (%s): %s", step.Position, step.Title, failed.Message)
		default:
			note = fmt.Sprintf("stopped at step %d (%s): the step could not be recorded", step.Position, step.Title)
			if s.Logger != nil {
				s.Logger.Error("runbooks: unattended step failed", "error", stepErr, "run", run.ID, "step", step.ID)
			}
		}
		break
	}
	return s.Finish(ctx, nil, run.ID, FinishInput{Outcome: outcome, Note: note}, op)
}

// startFor builds the run's subject: the alert's when an alert started it,
// otherwise whatever the runbook itself is attached to.
func startFor(rb *Runbook, alert *firingAlert) StartInput {
	in := StartInput{RunbookID: rb.ID, EnvironmentID: rb.EnvironmentID}
	if alert != nil {
		if alert.EnvironmentID != "" {
			in.EnvironmentID = alert.EnvironmentID
		}
		in.SubjectKind, in.SubjectID, in.SubjectName = alert.SourceType, alert.SubjectID, alert.SubjectName
		in.AlertEventID = alert.ID
		return in
	}
	switch rb.AttachKind {
	case AttachContainer, AttachStack:
		in.SubjectKind, in.SubjectName = rb.AttachKind, rb.AttachSubject
	case AttachEnvironment:
		in.SubjectKind, in.SubjectID = AttachEnvironment, rb.EnvironmentID
	case AttachAlertCondition:
	default:
		// Any other stored kind is a provider's attach kind, whose subject is the object id.
		in.SubjectKind, in.SubjectID, in.SubjectName = rb.AttachKind, rb.AttachSubject, rb.AttachSubject
	}
	return in
}

// nextRun is the next fire time after now, in the same local-time reading the
// backup scheduler uses, stored as UTC RFC3339 so it orders as a string.
func (s *Service) nextRun(cron string) string {
	return cronspec.Next(cron, s.clock()).UTC().Format(time.RFC3339)
}

func (s *Service) clock() time.Time {
	if s.Store != nil && s.Store.Now != nil {
		return s.Store.Now()
	}
	return time.Now()
}
