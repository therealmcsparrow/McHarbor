// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type sqliteHandle struct{ db *sql.DB }

func (h sqliteHandle) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := h.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (h sqliteHandle) Query(ctx context.Context, query string, args ...any) (plugin.Rows, error) {
	return h.db.QueryContext(ctx, query, args...)
}

func (h sqliteHandle) KV() plugin.KVStore { return nil }

type fakeGuestRPC struct {
	mu    sync.Mutex
	calls []string
	fail  error
	reply []byte
	down  bool
	sink  func(envID, msgType string, frame []byte)
}

func (f *fakeGuestRPC) Available(context.Context, string) (bool, string) {
	if f.down {
		return false, "agent_offline"
	}
	return true, ""
}

func (f *fakeGuestRPC) Call(_ context.Context, _, messageType string, _ []byte, _ func(string, []byte)) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, messageType)
	if f.fail != nil {
		return nil, f.fail
	}
	if f.reply == nil && messageType == msgComputeProbe {
		return []byte(probeReply), nil
	}
	if f.reply != nil {
		return f.reply, nil
	}
	return []byte(`{}`), nil
}

// Subscribe keeps the sink the bridge installs, so a test can deliver frames as the host would.
func (f *fakeGuestRPC) Subscribe(sink func(envID, msgType string, frame []byte)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sink = sink
}

// emit delivers one observed frame through the installed sink.
func (f *fakeGuestRPC) emit(envID, msgType, frame string) {
	f.mu.Lock()
	sink := f.sink
	f.mu.Unlock()
	if sink != nil {
		sink(envID, msgType, []byte(frame))
	}
}

// probeReply is the capability report the fake agent answers a probe with.
const probeReply = `{"capability":{"canRunVms":true,"qemuBinaries":[{"kvmAccel":true,"machineTypes":["pc-q35-8.2"]}]}}`

type quietLogger struct{}

func (quietLogger) Info(string, ...any)  {}
func (quietLogger) Warn(string, ...any)  {}
func (quietLogger) Error(string, ...any) {}
func (quietLogger) Debug(string, ...any) {}

func newIntentTestService(t *testing.T) (*Service, *fakeGuestRPC, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kvm.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}

	rpc := &fakeGuestRPC{}
	svc := NewService(sqliteHandle{db}, rpc, nil, nil, nil, nil, nil, t.TempDir(), quietLogger{})
	ctx := context.Background()
	for _, ensure := range []func(context.Context) error{
		svc.ensureScheduleSchema, svc.ensureImportSchema, svc.ensureNICSchema,
		svc.ensureSnapshotTreeSchema, svc.ensureIncidentSchema, svc.ensureGuestHealthSchema,
		svc.ensureDisplayProfileSchema,
	} {
		if err := ensure(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return svc, rpc, db
}

func insertIntentTestVM(t *testing.T, db *sql.DB, id, state string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_vms
			(id, environment_id, name, state, vcpus, memory_mb, machine_type, uuid, created_at, updated_at)
		VALUES (?, 'env1', ?, ?, 2, 2048, 'pc-q35-8.2', ?, ?, ?)`,
		id, "guest-"+id, state, "uuid-"+id, now, now); err != nil {
		t.Fatal(err)
	}
}

type intentRow struct {
	action, actor, source, outcome, errorText string
}

func intentRows(t *testing.T, db *sql.DB, vmID string) []intentRow {
	t.Helper()
	rows, err := db.Query(`
		SELECT action, actor, source, outcome, COALESCE(error, '')
		  FROM plugin_addon_kvm_lifecycle_intents WHERE vm_id = ? ORDER BY rowid`, vmID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []intentRow
	for rows.Next() {
		var row intentRow
		if err := rows.Scan(&row.action, &row.actor, &row.source, &row.outcome, &row.errorText); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	return out
}

func TestIncidentIntentRecordedForEveryServiceAction(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  string
		run   func(ctx context.Context, svc *Service, id string) error
	}{
		{"start", StateStopped, intentStart, func(ctx context.Context, svc *Service, id string) error { return svc.Start(ctx, id) }},
		{"shutdown", StateRunning, intentShutdown, func(ctx context.Context, svc *Service, id string) error {
			_, err := svc.Shutdown(ctx, id, 30)
			return err
		}},
		{"kill", StateRunning, intentKill, func(ctx context.Context, svc *Service, id string) error { return svc.Stop(ctx, id) }},
		{"reset", StateRunning, intentReset, func(ctx context.Context, svc *Service, id string) error { return svc.Reset(ctx, id) }},
		{"pause", StateRunning, intentPause, func(ctx context.Context, svc *Service, id string) error { return svc.Pause(ctx, id) }},
		{"resume", StatePaused, intentResume, func(ctx context.Context, svc *Service, id string) error { return svc.Resume(ctx, id) }},
		{"snapshot pause", StateRunning, intentSnapshotPause, func(ctx context.Context, svc *Service, id string) error {
			_, err := svc.CreateSnapshot(ctx, id, SnapshotRequest{Name: "before-upgrade", PauseGuest: true})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, db := newIntentTestService(t)
			insertIntentTestVM(t, db, "vm1", tc.state)
			ctx := withIntentOrigin(context.Background(), userActor("u1"), "api")
			if err := tc.run(ctx, svc, "vm1"); err != nil {
				t.Fatalf("action failed: %v", err)
			}
			rows := intentRows(t, db, "vm1")
			if len(rows) != 1 {
				t.Fatalf("rows = %+v, want exactly one", rows)
			}
			want := intentRow{action: tc.want, actor: "user:u1", source: "api", outcome: intentOK}
			if rows[0] != want {
				t.Fatalf("row = %+v, want %+v", rows[0], want)
			}
		})
	}
}

func TestIncidentFailedActionIsRecordedAsFailed(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	rpc.fail = errors.New("agent went away")

	if err := svc.Stop(context.Background(), "vm1"); err == nil {
		t.Fatal("the stop should have failed")
	}
	rows := intentRows(t, db, "vm1")
	if len(rows) != 1 || rows[0].action != intentKill || rows[0].outcome != intentFailed ||
		!strings.Contains(rows[0].errorText, "agent went away") || rows[0].actor != "system" {
		t.Fatalf("rows = %+v, want one failed system kill", rows)
	}
}

func TestIncidentDeleteIntentFinishesInBackground(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateStopped)

	if err := svc.Delete(withIntentOrigin(context.Background(), userActor("u1"), "api"), "vm1", DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows := intentRows(t, db, "vm1")
		if len(rows) == 1 && rows[0].action == intentDelete && rows[0].outcome == intentOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("rows = %+v, want one finished delete", rows)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestIncidentWorkflowNodeRecordsWorkflowOrigin(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	p := &KVMPlugin{svc: svc, logger: quietLogger{}}

	vm, err := svc.Get(context.Background(), "vm1")
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.execLifecycle(context.Background(), plugin.NodeExecInput{
		WorkflowID: "wf1", Config: map[string]any{"action": "stop"}, Msg: map[string]any{},
	}, vm)
	if err != nil || out.Port != "output" {
		t.Fatalf("node = %+v, %v", out, err)
	}
	rows := intentRows(t, db, "vm1")
	want := intentRow{action: intentKill, actor: "workflow:wf1", source: "workflow", outcome: intentOK}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
}

func TestIncidentQEMUEventsPersistedAndBounded(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	svc.recordVMEvent(GuestEvent{
		EnvID: "env1", GuestKind: guestKindVM, GuestID: "vm1", Kind: qemuEventShutdown,
		Detail: map[string]string{"reason": "host-signal\n" + strings.Repeat("y", 600)},
		At:     time.Now().Add(-time.Minute),
	})
	svc.recordVMEvent(GuestEvent{
		EnvID: "env1", GuestKind: guestKindVM, GuestID: "vm1", Kind: "rtc_change",
	})

	events, err := svc.readEvents(context.Background(), "", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := events["vm1"]
	if len(got) != 1 || got[0].kind != qemuEventShutdown {
		t.Fatalf("events = %+v, want only the shutdown", got)
	}
	if strings.Contains(got[0].reason, "\n") || len([]rune(got[0].reason)) != maxIntentText {
		t.Fatalf("reason was not bounded: %q", got[0].reason)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plugin_addon_kvm_vm_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("stored %d events, %v", count, err)
	}
}
