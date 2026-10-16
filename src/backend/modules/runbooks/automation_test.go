// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

const principalID = "svc-runbooks"
const principalName = "runbook-bot"

// principalGrants is an Authorizer that records who every check was made for,
// so a test can show the step was authorised as the principal and nobody else.
type principalGrants struct {
	held map[rbac.Permission]bool
	who  []string
}

func (p *principalGrants) HasPermission(_ context.Context, userID, _ string, perm rbac.Permission) (bool, error) {
	p.who = append(p.who, userID)
	return p.held[perm], nil
}

func seedPrincipal(t *testing.T, db *sql.DB, id, name, kind string, active bool) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, is_active, principal_kind, auth_provider) VALUES (?, ?, '', ?, ?, ?)`,
		id, name, boolInt(active), kind, kind); err != nil {
		t.Fatalf("seeding principal %s: %v", id, err)
	}
}

func automatedFixture(t *testing.T, steps []StepInput, held ...rbac.Permission) (*fixture, *principalGrants) {
	t.Helper()
	f := newFixture(t, steps)
	authz := &principalGrants{held: map[rbac.Permission]bool{}}
	for _, p := range held {
		authz.held[p] = true
	}
	f.svc.Authz = authz
	seedPrincipal(t, f.db, principalID, principalName, principalKindService, true)
	rb, err := f.svc.ConfigureAutomation(context.Background(), f.runbook.ID,
		AutomationInput{PrincipalID: principalID, OnAlert: true, Cron: "0 3 * * *"}, actor)
	if err != nil {
		t.Fatalf("configuring automation: %v", err)
	}
	f.runbook = rb
	return f, authz
}

// TestUnattendedStepDeniedWhenThePrincipalLacksTheKey is the identity test for
// unattended runs: the principal holds runbooks.run but not containers.manage,
// so the restart is refused, recorded as failed naming the key, the run is
// abandoned there — and the runtime is never reached.
func TestUnattendedStepDeniedWhenThePrincipalLacksTheKey(t *testing.T) {
	f, authz := automatedFixture(t, []StepInput{
		{Kind: StepAction, Title: "Restart it", ActionKey: "container.restart", Required: true},
	}, rbac.PermRunbooksRun)

	run, err := f.svc.RunUnattended(context.Background(), f.runbook, TriggerSchedule, nil)
	if err != nil {
		t.Fatalf("RunUnattended: %v", err)
	}
	if f.rt.restarts != 0 || f.rt.inspects != 0 {
		t.Fatalf("a denied unattended step reached the runtime (%d restarts, %d inspects)", f.rt.restarts, f.rt.inspects)
	}
	if run.Outcome != OutcomeAbandoned {
		t.Fatalf("want the run abandoned at the denied step, got %s", run.Outcome)
	}
	if len(run.Results) != 1 || run.Results[0].State != StateFailed ||
		!strings.Contains(run.Results[0].LastError, "containers.manage") {
		t.Fatalf("want one failed result naming containers.manage, got %+v", run.Results)
	}
	for _, who := range authz.who {
		if who != principalID {
			t.Fatalf("a step was authorised as %q; unattended steps are authorised as the principal", who)
		}
	}
	if len(authz.who) == 0 {
		t.Fatal("no permission check was made at all")
	}
}

// TestUnattendedRunIsAttributedToThePrincipal: the run's started_by, every step's
// actor and every audit row's user_id are the principal's real id — never NULL,
// never a label — and the trigger is recorded.
func TestUnattendedRunIsAttributedToThePrincipal(t *testing.T) {
	f, _ := automatedFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersView)

	run, err := f.svc.RunUnattended(context.Background(), f.runbook, TriggerSchedule, nil)
	if err != nil {
		t.Fatalf("RunUnattended: %v", err)
	}
	if run.Outcome != OutcomeCompleted || run.StartedBy != principalID || run.Trigger != TriggerSchedule {
		t.Fatalf("want a completed schedule run started by the principal, got %s by %q via %q",
			run.Outcome, run.StartedBy, run.Trigger)
	}
	if f.rt.inspects != 1 {
		t.Fatalf("the allowed diagnostic should have run once, ran %d", f.rt.inspects)
	}
	rows, err := f.db.Query(`SELECT action, COALESCE(user_id, ''), COALESCE(username, '') FROM audit_logs
		WHERE entity_type = 'runbook_run' AND entity_id = ?`, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	seen := 0
	for rows.Next() {
		var action, userID, username string
		if err := rows.Scan(&action, &userID, &username); err != nil {
			t.Fatal(err)
		}
		seen++
		if userID != principalID || username != principalName {
			t.Fatalf("audit row %q has actor %q/%q; want the service principal", action, userID, username)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen < 3 {
		t.Fatalf("want start, step and finish audited, saw %d row(s)", seen)
	}
}

// TestUnattendedRunStopsAtTheFirstInstruction: a person is needed there, so the
// run escalates and says which step it stopped at; nothing after it runs.
func TestUnattendedRunStopsAtTheFirstInstruction(t *testing.T) {
	f, _ := automatedFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
		{Kind: StepInstruction, Title: "Call the owner", Required: true},
		{Kind: StepAction, Title: "Restart it", ActionKey: "container.restart", Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersView, rbac.PermContainersManage)

	run, err := f.svc.RunUnattended(context.Background(), f.runbook, TriggerSchedule, nil)
	if err != nil {
		t.Fatalf("RunUnattended: %v", err)
	}
	if run.Outcome != OutcomeEscalated || !strings.Contains(run.Note, "step 2") {
		t.Fatalf("want escalated at step 2, got %s: %q", run.Outcome, run.Note)
	}
	if f.rt.restarts != 0 {
		t.Fatal("a step after the instruction ran")
	}
}

// TestAutomationRefusals pins every configuration the engine could never run
// honestly: a bad cron, a person as principal, an opening instruction, the alert
// trigger on a runbook with no alert attachment, and no trigger at all.
func TestAutomationRefusals(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
	})
	seedPrincipal(t, f.db, principalID, principalName, principalKindService, true)
	seedPrincipal(t, f.db, "u-person", "person", "user", true)
	ctx := context.Background()

	cases := []struct {
		name string
		in   AutomationInput
		want error
	}{
		{"unparseable schedule", AutomationInput{PrincipalID: principalID, Cron: "every night at three"}, ErrCronInvalid},
		{"out-of-range schedule", AutomationInput{PrincipalID: principalID, Cron: "61 3 * * *"}, ErrCronInvalid},
		{"a person as principal", AutomationInput{PrincipalID: "u-person", Cron: "0 3 * * *"}, ErrPrincipalInvalid},
		{"no trigger", AutomationInput{PrincipalID: principalID}, ErrNoTrigger},
	}
	for _, tc := range cases {
		if _, err := f.svc.ConfigureAutomation(ctx, f.runbook.ID, tc.in, actor); !errors.Is(err, tc.want) {
			t.Errorf("%s: want %v, got %v", tc.name, tc.want, err)
		}
	}
	rb, err := f.store.Get(ctx, f.runbook.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Automation.Active() {
		t.Fatal("a refused configuration was stored")
	}

	instr := newFixture(t, []StepInput{{Kind: StepInstruction, Title: "Look first", Required: true}})
	seedPrincipal(t, instr.db, principalID, principalName, principalKindService, true)
	if _, err := instr.svc.ConfigureAutomation(ctx, instr.runbook.ID,
		AutomationInput{PrincipalID: principalID, Cron: "0 3 * * *"}, actor); !errors.Is(err, ErrFirstStepInstruction) {
		t.Errorf("an opening instruction: want ErrFirstStepInstruction, got %v", err)
	}

	container, err := f.store.Save(ctx, "", RunbookInput{Name: "web", AttachKind: AttachContainer, AttachSubject: "web",
		Enabled: true, Steps: []StepInput{{Kind: StepDiagnostic, Title: "Look", DiagnosticKey: "container.state"}}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ConfigureAutomation(ctx, container,
		AutomationInput{PrincipalID: principalID, OnAlert: true}, actor); !errors.Is(err, ErrAlertNeedsCondition) {
		t.Errorf("the alert trigger on a container runbook: want ErrAlertNeedsCondition, got %v", err)
	}
}

// TestEngineAnswersEachAlertOnce drives the engine: one firing alert matching
// the runbook's condition starts one alert-triggered run, and the next tick does
// not start a second.
func TestEngineAnswersEachAlertOnce(t *testing.T) {
	f, _ := automatedFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersView)
	now := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	if _, err := f.db.Exec(`INSERT INTO alert_events (id, alert_id, alert_name, source_type, condition_key,
		environment_id, subject_name, state, severity, message, created_at)
		VALUES ('ev-9', 'al-1', 'Restarts', 'container', 'restarts', 'e1', 'web', 'firing', 'warning', 'x', ?)`, now); err != nil {
		t.Fatalf("seeding alert: %v", err)
	}
	engine := NewEngine(f.svc, nil)
	engine.Tick(context.Background())
	engine.Tick(context.Background())

	runs, total, err := f.store.ListRuns(context.Background(), RunFilter{StartedBy: principalID, Trigger: TriggerAlert}, 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || runs[0].AlertEventID != "ev-9" || runs[0].SubjectName != "web" {
		t.Fatalf("want exactly one alert run against ev-9/web, got %d: %+v", total, runs)
	}
}

// TestEditingAnAutomatedRunbookNeedsAutomate: runbooks.manage alone must not be
// able to change what a service principal executes unattended.
func TestEditingAnAutomatedRunbookNeedsAutomate(t *testing.T) {
	f, authz := automatedFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
	}, rbac.PermRunbooksManage)
	if err := f.svc.GuardAutomatedEdit(context.Background(), nil, f.runbook.ID, actor); !errors.Is(err, ErrAutomateRequired) {
		t.Fatalf("want ErrAutomateRequired without runbooks.automate, got %v", err)
	}
	authz.held[rbac.PermRunbooksAutomate] = true
	if err := f.svc.GuardAutomatedEdit(context.Background(), nil, f.runbook.ID, actor); err != nil {
		t.Fatalf("with runbooks.automate the edit must pass, got %v", err)
	}
}
