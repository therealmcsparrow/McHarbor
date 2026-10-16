// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	errdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	_ "modernc.org/sqlite"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

type fakeDocker struct {
	mu          sync.Mutex
	running     map[string]bool
	images      map[string]string
	unreachable bool
	calls       []string
}

func (f *fakeDocker) ContainerInspect(_ context.Context, name string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if f.unreachable {
		return client.ContainerInspectResult{}, errors.New("agent not connected")
	}
	running, ok := f.running[name]
	if !ok {
		return client.ContainerInspectResult{}, errdefs.ErrNotFound.WithMessage("No such container: " + name)
	}
	status := "exited"
	if running {
		status = "running"
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{
		State:  &container.State{Running: running, Status: container.ContainerState(status)},
		Config: &container.Config{Image: f.images[name]},
	}}, nil
}

func (f *fakeDocker) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}

func (f *fakeDocker) ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{}, errdefs.ErrNotFound.WithMessage("no such image")
}

func (f *fakeDocker) set(name string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running[name] = running
}

type leaderStub struct{ leader bool }

func (l leaderStub) LeaderOf(context.Context, string) bool { return l.leader }

func newLoopFixture(t *testing.T) (*Loop, *sql.DB, *fakeDocker, *fakeDocker) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "reconciler.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing the test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('env-far', 'far')`); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	seed := []struct{ id, env, name, status string }{
		{"ds-sync", "", "web", "running"},
		{"ds-drift", "", "worker", "running"},
		{"ds-far", "env-far", "remote", "running"},
	}
	for _, s := range seed {
		var env any
		if s.env != "" {
			env = s.env
		}
		if _, err := database.Exec(`INSERT INTO desired_states (id, name, env_id, container_name, image_ref, desired_status, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'nginx:1', ?, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`,
			s.id, s.id, env, s.name, s.status); err != nil {
			t.Fatalf("seed desired state: %v", err)
		}
	}

	local := &fakeDocker{running: map[string]bool{"web": true, "worker": false}, images: map[string]string{"web": "nginx:1", "worker": "nginx:1"}}
	far := &fakeDocker{unreachable: true}
	loop := NewLoop(NewService(database, testEncryption(t)), nil, nil, coreAudit.NewLogger(database, "test-secret"),
		slog.New(slog.NewTextHandler(io.Discard, nil)), leaderStub{leader: true}, time.Minute)
	loop.inspectorFor = func(envID string) (dockerReader, error) {
		if envID == "env-far" {
			return far, nil
		}
		return local, nil
	}
	return loop, database, local, far
}

func driftFlag(t *testing.T, database *sql.DB, id string) (bool, sql.NullString) {
	t.Helper()
	var drift bool
	var last sql.NullString
	if err := database.QueryRow(`SELECT drift_detected, last_reconcile FROM desired_states WHERE id = ?`, id).Scan(&drift, &last); err != nil {
		t.Fatal(err)
	}
	return drift, last
}

func auditCount(t *testing.T, database *sql.DB, action string) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestLoopConfirmsDriftBeforeRecordingIt guards the debounce: one pass that
// catches a container mid-restart must not flip the flag or write an audit row.
func TestLoopConfirmsDriftBeforeRecordingIt(t *testing.T) {
	loop, database, local, _ := newLoopFixture(t)
	ctx := context.Background()

	loop.pass(ctx)
	if drift, last := driftFlag(t, database, "ds-drift"); drift || !last.Valid {
		t.Fatalf("after one pass: drift=%v checked=%v, want unrecorded but checked", drift, last.Valid)
	}
	if n := auditCount(t, database, "reconciler.drift.detected"); n != 0 {
		t.Fatalf("audit rows after one pass = %d, want 0", n)
	}

	loop.pass(ctx)
	if drift, _ := driftFlag(t, database, "ds-drift"); !drift {
		t.Fatal("drift seen on two consecutive passes was not recorded")
	}
	if n := auditCount(t, database, "reconciler.drift.detected"); n != 1 {
		t.Fatalf("detected audit rows = %d, want 1", n)
	}

	loop.pass(ctx)
	if n := auditCount(t, database, "reconciler.drift.detected"); n != 1 {
		t.Fatalf("persisting drift wrote another audit row: %d", n)
	}

	local.set("worker", true)
	loop.pass(ctx)
	if drift, _ := driftFlag(t, database, "ds-drift"); drift {
		t.Fatal("resolved drift still recorded")
	}
	if n := auditCount(t, database, "reconciler.drift.resolved"); n != 1 {
		t.Fatalf("resolved audit rows = %d, want 1", n)
	}
}

// TestLoopTreatsAnUnreachableEnvironmentAsUnknown guards the rule that an agent
// you cannot see is unknown, never drifted: the row must not be touched at all.
func TestLoopTreatsAnUnreachableEnvironmentAsUnknown(t *testing.T) {
	loop, database, _, far := newLoopFixture(t)
	loop.pass(context.Background())
	loop.pass(context.Background())

	drift, last := driftFlag(t, database, "ds-far")
	if drift || last.Valid {
		t.Fatalf("unreachable row was written: drift=%v last_reconcile=%v", drift, last)
	}
	if len(far.calls) == 0 {
		t.Fatal("the unreachable environment was never asked")
	}
}

func TestLoopNeverActsOnContainers(t *testing.T) {
	var _ dockerReader = (*fakeDocker)(nil)
	loop, _, local, _ := newLoopFixture(t)
	loop.pass(context.Background())
	for _, call := range local.calls {
		if call != "web" && call != "worker" {
			t.Fatalf("unexpected call %q", call)
		}
	}
}

func TestLoopSkipsWhenNotLeader(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	loop.coord = leaderStub{leader: false}
	loop.pass(context.Background())
	if _, last := driftFlag(t, database, "ds-sync"); last.Valid {
		t.Fatal("a follower wrote a check")
	}
}

func TestDetectDriftDistinguishesMissingFromUnreachable(t *testing.T) {
	ds := &DesiredState{ID: "x", ContainerName: "gone", DesiredStatus: "running"}
	report, err := DetectDrift(context.Background(), &fakeDocker{running: map[string]bool{}}, ds)
	if err != nil || !report.HasDrift || report.Diffs[0].Field != "existence" {
		t.Fatalf("missing container: report=%+v err=%v", report, err)
	}
	if _, err := DetectDrift(context.Background(), &fakeDocker{unreachable: true}, ds); err == nil {
		t.Fatal("an unreachable daemon was reported as a missing container")
	}
}

func TestLoopStoresTheTypedReportAndKeepsFirstSeen(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	first := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	loop.detector.now = func() time.Time { return first }
	loop.pass(context.Background())
	loop.detector.now = func() time.Time { return first.Add(time.Hour) }
	loop.pass(context.Background())

	ds, err := NewService(database, testEncryption(t)).ByID(t.Context(), "ds-drift")
	if err != nil || ds == nil || ds.LastReport == nil || len(ds.LastReport.Diffs) != 1 {
		t.Fatalf("stored report: %+v, %v", ds, err)
	}
	d := ds.LastReport.Diffs[0]
	if d.Class != ClassState || d.Field != "status" {
		t.Fatalf("stored diff %+v, want a state-class status diff", d)
	}
	if want := first.Format(time.RFC3339); d.FirstSeenAt != want {
		t.Fatalf("first seen %q, want %q from the first pass", d.FirstSeenAt, want)
	}
	if want := first.Add(time.Hour).Format(time.RFC3339); ds.LastReport.CheckedAt != want {
		t.Fatalf("checked at %q, want %q", ds.LastReport.CheckedAt, want)
	}
}
