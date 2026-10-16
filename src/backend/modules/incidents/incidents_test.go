// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	_ "modernc.org/sqlite"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

var t0 = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "incidents.db")+"?_pragma=foreign_keys(1)")
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
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('e1', 'prod')`); err != nil {
		t.Fatal(err)
	}
	return database
}

// stored is when the collector would have written an event: the fixture's
// clock, since the detector keys on insertion time, not on the event's own time.
var stored = func() time.Time { return t0 }

func event(t *testing.T, database *sql.DB, name, action string, at time.Time, attrs map[string]string) {
	t.Helper()
	created := stored()
	if at.After(created) {
		created = at
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO lifecycle_events (id, environment_id, subject_type, subject_id, subject_name, event_type,
		action, severity, metadata, source, timestamp, created_at, updated_at)
		VALUES (?, 'e1', 'container', ?, ?, 'container', ?, 'info', ?, 'docker', ?, ?, ?)`,
		fmt.Sprintf("%s-%s-%d", name, action, at.UnixNano()), "id-"+name, name, action, string(raw),
		at.Format(time.RFC3339), created.Format(time.RFC3339), created.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

func die(t *testing.T, database *sql.DB, name string, at time.Time, code string) {
	event(t, database, name, "die", at, map[string]string{"exitCode": code, "image": "api:2", "com.docker.compose.project": "shop"})
}

type fakeDocker struct {
	logs string
}

func (f fakeDocker) ContainerInspect(_ context.Context, name string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{Container: container.InspectResponse{
		Image: "sha256:0123456789abcdef", RestartCount: 7,
		State:  &container.State{Status: "restarting", ExitCode: 1},
		Config: &container.Config{Image: "api:2", Tty: true},
	}}, nil
}

func (f fakeDocker) ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return io.NopCloser(bytes.NewBufferString(f.logs)), nil
}

type fakeModel struct {
	mu      sync.Mutex
	answer  string
	fail    error
	prompts []string
	none    bool
}

func (m *fakeModel) Info(context.Context) (ModelInfo, error) {
	if m.none {
		return ModelInfo{}, ErrNoModel
	}
	return ModelInfo{Provider: "fake", Connection: "test", Model: "fake-1"}, nil
}

func (m *fakeModel) Complete(_ context.Context, system, prompt string, _ int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, system+"\n"+prompt)
	if m.fail != nil {
		return "", m.fail
	}
	return m.answer, nil
}

func (m *fakeModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.prompts)
}

const goodAnswer = `Here you go: {"summary":"api has crash-looped since 10:01 after the image moved to api:2.",
"startedAt":"2026-09-26T10:01:00Z","changes":["image api:1 -> api:2"],
"causes":[{"title":"api:2 fails to start","evidence":["die exit 1 at 10:01","log: panic: missing DATABASE_URL"],"confidence":"high"},
{"title":"no evidence cause","evidence":[],"confidence":"high"}],"confidence":"medium","undetermined":false}`

type fixture struct {
	db     *sql.DB
	svc    *Service
	det    *Detector
	eng    *Engine
	model  *fakeModel
	now    time.Time
	docker fakeDocker
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	database := openDB(t)
	f := &fixture{db: database, model: &fakeModel{answer: goodAnswer}, now: t0.Add(5 * time.Minute)}
	f.docker = fakeDocker{logs: "2026-09-26T10:01:00Z starting api\n" +
		"2026-09-26T10:01:01Z connecting to postgres://app:hunter2secret@db:5432/app\n" +
		"2026-09-26T10:01:01Z auth eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U\n" +
		"2026-09-26T10:01:02Z docker at unix:///var/run/docker.sock\n" +
		"2026-09-26T10:01:02Z panic: missing DATABASE_URL\n"}
	clock := func() time.Time { return f.now }
	stored = clock
	store := &Store{db: database, now: clock}
	asm := &Assembler{DB: database, Now: clock, ReaderFor: func(string) (DockerReader, error) { return f.docker, nil }}
	f.svc = &Service{Store: store, Assembler: asm, Model: f.model, Audit: coreAudit.NewLogger(database, "s"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock}
	f.det = &Detector{DB: database, Store: store, Now: clock}
	f.eng = &Engine{Service: f.svc, Detector: f.det, Logger: f.svc.Logger}
	return f
}

func (f *fixture) optIn(t *testing.T, proactive bool, hourlyCap int) {
	t.Helper()
	if _, err := f.svc.Store.SaveSettings(context.Background(), Settings{EnvID: "e1", NarrateOptIn: true, Proactive: proactive, HourlyCap: hourlyCap}, "admin"); err != nil {
		t.Fatal(err)
	}
}

// TestDetectorOpensOneIncidentPerFailingContainer covers the thresholds: one
// failing exit is not an incident, three in ten minutes is a crash loop, a
// clean exit is nothing, and later signals extend rather than duplicate.
func TestDetectorOpensOneIncidentPerFailingContainer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	die(t, f.db, "api", t0, "1")
	die(t, f.db, "web", t0, "0")
	die(t, f.db, "web", t0.Add(30*time.Second), "0")
	die(t, f.db, "web", t0.Add(time.Minute), "0")
	res, err := f.det.Scan(ctx)
	if err != nil || len(res.Opened) != 0 {
		t.Fatalf("one exit opened an incident: %+v %v", res, err)
	}
	die(t, f.db, "api", t0.Add(time.Minute), "1")
	f.now = t0.Add(6 * time.Minute)
	res, err = f.det.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Opened) != 1 || res.Opened[0].Signal != SignalExitRepeat || res.Opened[0].StackName != "shop" {
		t.Fatalf("two exits: %+v", res.Opened)
	}
	die(t, f.db, "api", t0.Add(2*time.Minute), "1")
	f.now = t0.Add(7 * time.Minute)
	res, err = f.det.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Opened) != 0 || len(res.Escalated) != 1 || res.Escalated[0].Signal != SignalCrashLoop {
		t.Fatalf("third exit should escalate the same incident: %+v", res)
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM incidents`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("incidents = %d %v", n, err)
	}
	event(t, f.db, "db", "oom", t0.Add(7*time.Minute), map[string]string{})
	f.now = t0.Add(8 * time.Minute)
	res, err = f.det.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Opened) != 1 || res.Opened[0].Signal != SignalOOM {
		t.Fatalf("oom: %+v", res.Opened)
	}
}

// TestIncidentStaysOpenWhileTheContainerIsDown checks a container that died
// and stayed down is not resolved just because it went quiet.
func TestIncidentStaysOpenWhileTheContainerIsDown(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		die(t, f.db, "api", t0.Add(time.Duration(i)*time.Minute), "1")
	}
	f.now = t0.Add(4 * time.Minute)
	if _, err := f.det.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	f.now = t0.Add(2 * time.Hour)
	f.eng.resolveQuiet(ctx)
	in, err := f.svc.Store.OpenFor(ctx, "e1", "api")
	if err != nil {
		t.Fatal(err)
	}
	if in == nil {
		t.Fatal("a container still down was resolved")
	}
	event(t, f.db, "api", "start", t0.Add(90*time.Minute), map[string]string{})
	f.eng.resolveQuiet(ctx)
	if in, err := f.svc.Store.OpenFor(ctx, "e1", "api"); err != nil || in != nil {
		t.Fatal("a recovered container's incident stayed open")
	}
	var status, timeline string
	if err := f.db.QueryRow(`SELECT status, timeline FROM incidents`).Scan(&status, &timeline); err != nil {
		t.Fatal(err)
	}
	if status != StatusResolved || !strings.Contains(timeline, `"kind":"symptom"`) || !strings.Contains(timeline, `"kind":"resolved"`) {
		t.Fatalf("resolved timeline: %s %s", status, timeline)
	}
}

func openCrashLoop(t *testing.T, f *fixture) *Incident {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		die(t, f.db, "api", t0.Add(time.Duration(i)*time.Minute), "1")
	}
	if _, err := f.db.Exec(`INSERT INTO audit_logs (id, username, action, entity_type, entity_name, environment_id, timestamp)
		VALUES ('a1', 'alice', 'container.recreate', 'container', 'api', 'e1', ?)`, t0.Add(-10*time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO container_metrics (id, environment_id, container_id, container_name, cpu_percent, mem_used, sampled_at)
		VALUES ('m1', 'e1', 'id-api', 'api', 91.5, 1000, ?)`, t0.Add(time.Minute).Format(metricsLayout)); err != nil {
		t.Fatal(err)
	}
	f.now = t0.Add(4 * time.Minute)
	res, err := f.det.Scan(ctx)
	if err != nil || len(res.Opened) != 1 {
		t.Fatalf("opening: %+v %v", res, err)
	}
	return res.Opened[0]
}

// TestEvidenceIsCorrelatedRedactedAndBounded checks the bundle joins the
// container's events, the person who touched it, its metrics and its logs —
// with every credential, token and socket path removed.
func TestEvidenceIsCorrelatedRedactedAndBounded(t *testing.T) {
	f := newFixture(t)
	in := openCrashLoop(t, f)
	ev, err := f.svc.Assembler.Assemble(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Events) != 3 || len(ev.Audit) != 1 || ev.Audit[0].Who != "alice" || len(ev.ContainerCPU) != 1 || ev.ContainerCPU[0].Value != 91.5 {
		t.Fatalf("correlation: events=%d audit=%+v cpu=%+v", len(ev.Events), ev.Audit, ev.ContainerCPU)
	}
	if ev.Container.Restarts != 7 || ev.Container.Image != "api:2" {
		t.Fatalf("container facts: %+v", ev.Container)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"hunter2secret", "eyJhbGciOiJIUzI1NiJ9", "docker.sock"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("evidence still contains %q", secret)
		}
	}
	if !strings.Contains(string(raw), "panic: missing DATABASE_URL") {
		t.Error("redaction removed the failure itself")
	}

	f.docker.logs = strings.Repeat("2026-09-26T10:01:02Z "+strings.Repeat("x", 300)+"\n", 5000)
	big, err := f.svc.Assembler.Assemble(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(big)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxEvidenceBytes {
		t.Fatalf("bundle is %d bytes, over the %d ceiling", len(raw), maxEvidenceBytes)
	}
}

// TestNarrationNeedsOptInAndIsAttributed checks nothing reaches the model
// before the environment opts in; then the narrative is parsed, a cause with
// no evidence is dropped, and the model, prompt and requester are recorded.
func TestNarrationNeedsOptInAndIsAttributed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := openCrashLoop(t, f)
	if _, err := f.svc.Narrate(ctx, in.ID, "bob", false); !errors.Is(err, ErrNotOptedIn) {
		t.Fatalf("narrating without opt-in: %v", err)
	}
	if f.model.calls() != 0 {
		t.Fatal("the model was called before the environment opted in")
	}
	f.optIn(t, false, 10)
	out, err := f.svc.Narrate(ctx, in.ID, "bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if out.NarrativeStatus != NarrativeReady || out.Narrative == nil || len(out.Narrative.Causes) != 1 || out.Narrative.Confidence != "medium" {
		t.Fatalf("narrative: %+v", out.Narrative)
	}
	m := out.NarrativeMeta
	if m == nil || m.Provider != "fake" || m.Model != "fake-1" || m.RequestedBy != "bob" || len(m.PromptSHA256) != 64 {
		t.Fatalf("attribution: %+v", m)
	}
	if !strings.Contains(out.NarrativePrompt, "panic: missing DATABASE_URL") || strings.Contains(f.model.prompts[0], "hunter2secret") {
		t.Fatal("the stored prompt is not what was sent, or a secret was sent")
	}
	var audited int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'incident.narrated'`).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("narration audit rows = %d %v", audited, err)
	}
}

// TestNarrationIsBoundedAndHonest covers the cap, the single in-flight claim,
// a model failure recorded on the incident, and an answer that is not the
// required JSON kept as undetermined rather than trusted.
func TestNarrationIsBoundedAndHonest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := openCrashLoop(t, f)
	f.optIn(t, true, 1)
	if _, err := f.svc.Narrate(ctx, in.ID, "system:narrator", true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Narrate(ctx, in.ID, "system:narrator", true); !errors.Is(err, ErrCapReached) {
		t.Fatalf("second proactive narration inside the hour: %v", err)
	}
	if f.model.calls() != 1 {
		t.Fatalf("model calls = %d, want 1", f.model.calls())
	}
	if ok, err := f.svc.Store.ClaimNarration(ctx, in.ID); err != nil || !ok {
		t.Fatal("first claim refused")
	}
	if ok, err := f.svc.Store.ClaimNarration(ctx, in.ID); err != nil || ok {
		t.Fatal("a second narration was allowed while one runs")
	}
	if err := f.svc.Store.FailNarration(ctx, in.ID, NarrativeNone, ""); err != nil {
		t.Fatal(err)
	}

	f.model.fail = errors.New("upstream 529 overloaded")
	if _, err := f.svc.Narrate(ctx, in.ID, "bob", false); !errors.Is(err, ErrModelFailed) {
		t.Fatalf("model failure: %v", err)
	}
	got, err := f.svc.Store.Get(ctx, in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NarrativeStatus != NarrativeFailed || !strings.Contains(got.NarrativeError, "529 overloaded") {
		t.Fatalf("failure not recorded: %s %q", got.NarrativeStatus, got.NarrativeError)
	}

	f.model.fail, f.model.answer = nil, "I think it is probably the database."
	got, err = f.svc.Narrate(ctx, in.ID, "bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Narrative.Undetermined || got.Narrative.Confidence != "low" || len(got.Narrative.Causes) != 0 {
		t.Fatalf("free text must be kept as undetermined, low confidence: %+v", got.Narrative)
	}
}

// TestEngineNarratesOnlyWhereProactiveIsOn checks automatic narration follows
// the environment's choice and nothing else.
func TestEngineNarratesOnlyWhereProactiveIsOn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.optIn(t, false, 10)
	for i := 0; i < 3; i++ {
		die(t, f.db, "api", t0.Add(time.Duration(i)*time.Minute), "1")
	}
	f.now = t0.Add(4 * time.Minute)
	f.eng.Tick(ctx)
	if f.model.calls() != 0 {
		t.Fatal("narrated with proactive off")
	}
	in, err := f.svc.Store.OpenFor(ctx, "e1", "api")
	if err != nil {
		t.Fatal(err)
	}
	if in == nil || in.Evidence == nil {
		t.Fatal("the opened incident has no evidence")
	}
	f.optIn(t, true, 10)
	for i := 0; i < 3; i++ {
		die(t, f.db, "worker", t0.Add(time.Duration(4+i)*time.Minute), "2")
	}
	f.now = t0.Add(8 * time.Minute)
	f.eng.Tick(ctx)
	if f.model.calls() != 1 {
		t.Fatalf("proactive narration calls = %d, want 1", f.model.calls())
	}
}

// TestDetectorsAreDeterministic pins each behavioural detector's threshold.
func TestDetectorsAreDeterministic(t *testing.T) {
	var climb, flat []Sample
	for d := 0; d < 10; d++ {
		at := t0.AddDate(0, 0, d)
		climb = append(climb, Sample{At: at, Value: 100e6 + float64(d)*10e6})
		flat = append(flat, Sample{At: at, Value: 100e6 + float64(d%2)*5e6})
	}
	m, ok := MemoryClimb(climb)
	if !ok || m["growthPercent"] != 90 || m["days"] != 9 {
		t.Fatalf("steady climb: %v %v", m, ok)
	}
	if _, ok := MemoryClimb(flat); ok {
		t.Fatal("a flat, noisy series was reported as a climb")
	}
	var noisy []Sample
	for d := 0; d < 10; d++ {
		noisy = append(noisy, Sample{At: t0.AddDate(0, 0, d), Value: 100e6 + float64(d)*5e6 + float64(d%2)*60e6})
	}
	if _, ok := MemoryClimb(noisy); ok {
		t.Fatal("a rising but erratic series was reported as a steady climb")
	}
	if _, ok := MemoryClimb(climb[:5]); ok {
		t.Fatal("five days of data were enough")
	}
	var host []Sample
	for d := 0; d < 10; d++ {
		host = append(host, Sample{At: t0.AddDate(0, 0, d), Value: 50 + float64(d)*2})
	}
	f, ok := HostMemoryForecast(host)
	if !ok || f["daysUntilFull"] != 16 {
		t.Fatalf("forecast: %v %v", f, ok)
	}
	var restarts []time.Time
	for d := 0; d < 4; d++ {
		restarts = append(restarts, time.Date(2026, 9, 10+d, 2, 41, 0, 0, time.UTC))
	}
	restarts = append(restarts, time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC))
	r, ok := ScheduledRestarts(restarts)
	if !ok || r["hour"] != 2 || r["minute"] != 40 || r["days"] != 4 {
		t.Fatalf("nightly restarts: %v %v", r, ok)
	}
	if _, ok := ScheduledRestarts(restarts[:2]); ok {
		t.Fatal("two days is a coincidence, not a schedule")
	}
	if _, ok := PostDeployFailures(2, 6); !ok {
		t.Fatal("6 failures after vs 2 before should be flagged")
	}
	if _, ok := PostDeployFailures(3, 6); ok {
		t.Fatal("doubling is not a threefold rise")
	}
}

// TestFindingExplanationKeepsTheNumbers checks explaining a finding needs the
// opt-in and never changes the detector's numbers.
func TestFindingExplanationKeepsTheNumbers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	b := &Behaviour{DB: f.db, Now: func() time.Time { return f.now }}
	id, err := b.upsert(ctx, "e1", "container", "api", FindingMemoryClimb, map[string]float64{"growthPercent": 42}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ExplainFinding(ctx, id, "bob"); !errors.Is(err, ErrNotOptedIn) {
		t.Fatalf("explaining without opt-in: %v", err)
	}
	f.optIn(t, false, 10)
	f.model.answer = "Memory grew 42% over the week."
	got, err := f.svc.ExplainFinding(ctx, id, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if got.Explanation != "Memory grew 42% over the week." || got.Metrics["growthPercent"] != 42 || got.ExplanationMeta.Model != "fake-1" {
		t.Fatalf("finding: %+v", got)
	}
}
