// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "runbooks.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

// grants is an Authorizer over a fixed set, which is what lets the identity
// test deny one key without standing up a role graph.
type grants struct {
	held  map[rbac.Permission]bool
	calls int
}

func (g *grants) HasPermission(_ context.Context, _, _ string, perm rbac.Permission) (bool, error) {
	g.calls++
	return g.held[perm], nil
}

// spyRuntime counts every call a step makes, so a test can assert the absence
// of a runtime call rather than only the refusal it produced.
type spyRuntime struct {
	inspects  int
	logs      int
	restarts  int
	failWith  error
	tty       bool
	logsBody  string
	lastState string
	labels    map[string]string
	stops     int
}

func (s *spyRuntime) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	s.inspects++
	if s.failWith != nil {
		return client.ContainerInspectResult{}, s.failWith
	}
	state := s.lastState
	if state == "" {
		state = "running"
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{
		State:  &container.State{Status: container.ContainerState(state), Running: true},
		Config: &container.Config{Tty: s.tty, Labels: s.labels},
	}}, nil
}

func (s *spyRuntime) ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	s.logs++
	if s.failWith != nil {
		return nil, s.failWith
	}
	return io.NopCloser(readerOf(s.logsBody)), nil
}

func (s *spyRuntime) ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, errors.New("start is not used by these tests")
}

func (s *spyRuntime) ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error) {
	s.stops++
	return client.ContainerStopResult{}, s.failWith
}

func (s *spyRuntime) ContainerRestart(context.Context, string, client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
	s.restarts++
	return client.ContainerRestartResult{}, s.failWith
}

type stringReader struct {
	s string
	i int
}

func readerOf(s string) io.Reader { return &stringReader{s: s} }

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

type fixture struct {
	svc     *Service
	store   *Store
	authz   *grants
	rt      *spyRuntime
	db      *sql.DB
	runbook *Runbook
}

const actor = "u-ops"
const actorName = "ops"

// newRequest builds a request carrying the operator in context, which is the
// only reason the audit rows this module writes have a non-NULL actor —
// Log(nil, …) is exactly what this module must never do.
func newRequest() *http.Request {
	return asActor(httptest.NewRequest("POST", "/api/runbooks/runs", nil))
}

func newRequestCtx(ctx context.Context) *http.Request {
	return asActor(httptest.NewRequestWithContext(ctx, "POST", "/api/runbooks/runs", nil))
}

func asActor(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), auth.UserContextKey,
		&auth.User{ID: actor, Username: actorName}))
}

func newFixture(t *testing.T, steps []StepInput, held ...rbac.Permission) *fixture {
	t.Helper()
	database := openDB(t)
	store := NewStore(database)
	authz := &grants{held: map[rbac.Permission]bool{}}
	for _, p := range held {
		authz.held[p] = true
	}
	rt := &spyRuntime{logsBody: "line one\nline two\n", tty: true}
	svc := &Service{
		Store: store, Authz: authz, Audit: audit.NewLogger(database, "test-secret"),
		RuntimeFor: func(string) (Runtime, error) { return rt, nil },
	}
	id, err := store.Save(context.Background(), "", RunbookInput{
		Name: "Restart loop", AttachKind: AttachAlertCondition, AttachSourceType: "container",
		AttachConditionKey: "restarts", EnvironmentID: "e1", Enabled: true, Steps: steps,
	}, actor)
	if err != nil {
		t.Fatalf("save runbook: %v", err)
	}
	rb, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get runbook: %v", err)
	}
	return &fixture{svc: svc, store: store, authz: authz, rt: rt, db: database, runbook: rb}
}

func (f *fixture) start(t *testing.T) *Run {
	t.Helper()
	run, err := f.svc.Start(context.Background(), f.req(), StartInput{
		RunbookID: f.runbook.ID, EnvironmentID: "e1", SubjectKind: "container",
		SubjectName: "web", AlertEventID: "ev-1", IncidentID: "in-1",
	}, Operator{UserID: actor, Username: actorName})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	return run
}

func (f *fixture) req() *http.Request { return newRequest() }

func TestActionStepDeniedNamesTheKeyAndTouchesNoRuntime(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Restart it", ActionKey: "container.restart", Required: true},
	}, rbac.PermRunbooksRun)
	run := f.start(t)

	_, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName})

	var denied *ForbiddenError
	if !errors.As(err, &denied) {
		t.Fatalf("want a ForbiddenError, got %v", err)
	}
	if denied.Permission != rbac.PermContainersManage {
		t.Fatalf("the refusal must name containers.manage, got %q", denied.Permission)
	}
	if f.rt.restarts != 0 {
		t.Fatalf("a denied step made %d runtime call(s); it must make none", f.rt.restarts)
	}
	results, err := f.store.Results(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("a denied step recorded %d result(s); it must record none", len(results))
	}
}

func TestPermissionRevokedMidRunStopsTheLaterStep(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
		{Kind: StepAction, Title: "Restart it", ActionKey: "container.restart", Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersView, rbac.PermContainersManage)
	run := f.start(t)
	op := Operator{UserID: actor, Username: actorName}

	if _, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, op); err != nil {
		t.Fatalf("step 1 should have been allowed: %v", err)
	}

	// The revocation. A run can span a shift; a run authorised at step 1 must
	// not still be authorised at step 2.
	f.authz.held[rbac.PermContainersManage] = false

	_, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[1].ID,
		StepInputResult{State: StateDone}, op)
	var denied *ForbiddenError
	if !errors.As(err, &denied) {
		t.Fatalf("want the later step refused after the revocation, got %v", err)
	}
	if f.rt.restarts != 0 {
		t.Fatalf("the revoked step still reached the runtime %d time(s)", f.rt.restarts)
	}
}

func TestEveryExecutedStepAuditsTheRealOperatorAgainstTheRun(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersView)
	run := f.start(t)

	if _, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone, Finding: "restarting every 40s"},
		Operator{UserID: actor, Username: actorName}); err != nil {
		t.Fatalf("step: %v", err)
	}

	rows, err := f.db.Query(
		`SELECT action, COALESCE(user_id, ''), COALESCE(username, ''), COALESCE(entity_type, ''), COALESCE(entity_id, '')
		 FROM audit_logs WHERE entity_type = 'runbook_run' ORDER BY timestamp ASC`)
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
		var action, userID, username, entityType, entityID string
		if err := rows.Scan(&action, &userID, &username, &entityType, &entityID); err != nil {
			t.Fatal(err)
		}
		seen++
		if userID == "" {
			t.Fatalf("audit row %q has a NULL actor; a runbook step with no actor is worthless", action)
		}
		if username != actorName {
			t.Fatalf("audit row %q names %q, want %q", action, username, actorName)
		}
		if entityID != run.ID {
			t.Fatalf("audit row %q names entity %q, want the run %q", action, entityID, run.ID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen < 2 {
		t.Fatalf("want an audit row for the start and for the step, got %d", seen)
	}
}

func TestRunRecordsWhereItStoppedAndIsOfferedBackForResume(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepInstruction, Title: "Check the dashboard", Required: true},
		{Kind: StepDiagnostic, Title: "Look at it", DiagnosticKey: "container.state", Required: true},
		{Kind: StepInstruction, Title: "Page the owner", Required: false},
	}, rbac.PermRunbooksRun, rbac.PermContainersView)
	run := f.start(t)
	op := Operator{UserID: actor, Username: actorName}

	for _, i := range []int{0, 1} {
		if _, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[i].ID,
			StepInputResult{State: StateDone, Finding: "still looping"}, op); err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
	}

	reread, err := f.store.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stoppedAt(reread); got != 2 {
		t.Fatalf("the run stopped at step %d, want 2", got)
	}
	if reread.Outcome != OutcomeInProgress {
		t.Fatalf("an unfinished run is %q, want %q", reread.Outcome, OutcomeInProgress)
	}

	resolved, err := f.svc.Resolve(context.Background(),
		ResolveQuery{EnvironmentID: "e1", SourceType: "container", ConditionKey: "restarts", ContainerName: "web"}, "ev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.OpenRuns) != 1 || resolved.OpenRuns[0].ID != run.ID {
		t.Fatalf("the in-progress run must be offered back for resume, got %+v", resolved.OpenRuns)
	}
	if resolved.OpenRuns[0].StartedByName != actorName {
		t.Fatalf("a resumable run must name who started it, got %q", resolved.OpenRuns[0].StartedByName)
	}
	if len(resolved.Runbooks) != 1 {
		t.Fatalf("the condition-attached runbook must resolve, got %d", len(resolved.Runbooks))
	}
}

func TestSkipRecordsItsReasonAndSurvivesAReorder(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepInstruction, Title: "Check the dashboard", Required: true},
		{Kind: StepInstruction, Title: "Page the owner", Required: false},
	}, rbac.PermRunbooksRun)
	run := f.start(t)

	if _, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[1].ID,
		StepInputResult{State: StateSkipped, SkipReason: "the owner is already on the call"},
		Operator{UserID: actor, Username: actorName}); err != nil {
		t.Fatalf("skip: %v", err)
	}

	// Reorder the runbook. The steps are deleted and rewritten, so a result that
	// pointed only at a step id would lose its meaning here.
	if _, err := f.store.Save(context.Background(), f.runbook.ID, RunbookInput{
		Name: "Restart loop", AttachKind: AttachAlertCondition, AttachSourceType: "container",
		AttachConditionKey: "restarts", EnvironmentID: "e1", Enabled: true,
		Steps: []StepInput{
			{Kind: StepInstruction, Title: "Page the owner", Required: false},
			{Kind: StepInstruction, Title: "Check the dashboard", Required: true},
		},
	}, actor); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	reread, err := f.store.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reread.Results) != 1 {
		t.Fatalf("want the skip still recorded, got %d result(s)", len(reread.Results))
	}
	got := reread.Results[0]
	if got.State != StateSkipped || got.SkipReason != "the owner is already on the call" {
		t.Fatalf("the skip lost its reason: %+v", got)
	}
	if got.StepPosition != 2 || got.StepTitle != "Page the owner" {
		t.Fatalf("the result must still say which step was which: position %d, title %q",
			got.StepPosition, got.StepTitle)
	}
}

func TestAFailedStepPersistsTheServerMessageForTheOperator(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Restart it", ActionKey: "container.restart", Required: true},
	}, rbac.PermRunbooksRun, rbac.PermContainersManage)
	f.rt.failWith = errors.New("cannot restart container web: tried to kill container, but did not receive an exit event")
	run := f.start(t)

	_, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName})
	var failed *StepFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("want a StepFailedError, got %v", err)
	}
	if failed.Message != f.rt.failWith.Error() {
		t.Fatalf("the operator must get the message intact, got %q", failed.Message)
	}
	reread, err := f.store.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reread.Results) != 1 || reread.Results[0].State != StateFailed {
		t.Fatalf("a failure must be recorded on the run, got %+v", reread.Results)
	}
	if reread.Results[0].LastError != f.rt.failWith.Error() {
		t.Fatalf("lastError was not persisted whole: %q", reread.Results[0].LastError)
	}
}

func TestAFinishedRunRefusesFurtherStepsAndKeepsItsOutcome(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepInstruction, Title: "Check the dashboard", Required: true},
	}, rbac.PermRunbooksRun)
	run := f.start(t)
	op := Operator{UserID: actor, Username: actorName}

	if _, err := f.svc.Finish(context.Background(), f.req(), run.ID,
		FinishInput{Outcome: OutcomeAbandoned, Note: "restart did not clear it"}, op); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, op); !errors.Is(err, ErrRunFinished) {
		t.Fatalf("want ErrRunFinished, got %v", err)
	}
	if _, err := f.svc.Finish(context.Background(), f.req(), run.ID,
		FinishInput{Outcome: OutcomeCompleted}, op); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a finished run must not be relabelled, got %v", err)
	}
	reread, err := f.store.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Outcome != OutcomeAbandoned || reread.Note != "restart did not clear it" {
		t.Fatalf("the abandonment and its reason must survive: %+v", reread)
	}
}

func TestEveryCatalogEntryDeclaresAPermission(t *testing.T) {
	svc := allProvidersService(t)
	for _, group := range [][]catalogEntry{svc.actionCatalog(), svc.diagnosticCatalog()} {
		for _, e := range group {
			if len(e.Required) == 0 {
				t.Errorf("catalog entry %q declares no permission, so a step naming it is ungated", e.Key)
			}
			if (e.Run == nil) == (e.Call == nil) {
				t.Errorf("catalog entry %q must have exactly one of Run or Call, or a step naming it would fail at 03:00", e.Key)
			}
			if (e.Call == nil) != (e.Domain == "") {
				t.Errorf("catalog entry %q must name its runbook domain exactly when it calls a plugin", e.Key)
			}
		}
	}
}

func TestDiagnosticOutputIsRedactedAndBounded(t *testing.T) {
	long := ""
	for len(long) <= MaxOutputBytes {
		long += "tcp://10.0.0.5:2375 is the endpoint. "
	}
	out := redact(long)
	if len(out) > MaxOutputBytes+len("\n[truncated]") {
		t.Fatalf("output is %d bytes, over the bound", len(out))
	}
	if containsAny(out, "tcp://", "docker.sock") {
		t.Fatalf("a socket endpoint survived redaction: %q", out[:80])
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		for i := 0; i+len(n) <= len(s); i++ {
			if s[i:i+len(n)] == n {
				return true
			}
		}
	}
	return false
}
