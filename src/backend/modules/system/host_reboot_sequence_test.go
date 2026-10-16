// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/demo"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

var testBudget = rebootBudget{Guests: 150 * time.Millisecond, Containers: 150 * time.Millisecond, Poll: 10 * time.Millisecond}

type fakeContainers struct {
	mu       sync.Mutex
	list     []rebootContainer
	stubborn map[string]bool
	stopped  map[string]bool
	signals  []string
	started  []string
}

func newFakeContainers(list []rebootContainer, stubborn ...string) *fakeContainers {
	f := &fakeContainers{list: list, stubborn: map[string]bool{}, stopped: map[string]bool{}}
	for _, id := range stubborn {
		f.stubborn[id] = true
	}
	return f
}

func (f *fakeContainers) RunningContainers(context.Context) ([]rebootContainer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []rebootContainer
	for _, ctr := range f.list {
		if !f.stopped[ctr.ID] {
			out = append(out, ctr)
		}
	}
	return out, nil
}

func (f *fakeContainers) SignalStop(_ context.Context, c rebootContainer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, c.ID+"="+c.StopSignal)
	if !f.stubborn[c.ID] {
		f.stopped[c.ID] = true
	}
	return nil
}

func (f *fakeContainers) StillRunning(_ context.Context, ids []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, id := range ids {
		if !f.stopped[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeContainers) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, id)
	f.stopped[id] = false
	return nil
}

type fakeGuests struct {
	mu        sync.Mutex
	list      []rebootGuest
	stubborn  map[string]bool
	stopped   map[string]bool
	requested []string
}

func (f *fakeGuests) RunningGuests(context.Context) ([]rebootGuest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []rebootGuest
	for _, guest := range f.list {
		if !f.stopped[guest.ID] {
			out = append(out, guest)
		}
	}
	return out, nil
}

func (f *fakeGuests) RequestShutdown(_ context.Context, g rebootGuest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, g.ID)
	if !f.stubborn[g.ID] {
		f.stopped[g.ID] = true
	}
	return nil
}

func (f *fakeGuests) StillRunning(_ context.Context, guests []rebootGuest) ([]rebootGuest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []rebootGuest
	for _, guest := range guests {
		if !f.stopped[guest.ID] {
			out = append(out, guest)
		}
	}
	return out, nil
}

func noProgress(string) {}

func assertNoKill(t *testing.T, signals []string) {
	t.Helper()
	for _, signal := range signals {
		upper := strings.ToUpper(signal)
		if strings.HasSuffix(upper, "=SIGKILL") || strings.HasSuffix(upper, "=KILL") || strings.HasSuffix(upper, "=9") {
			t.Fatalf("a container was killed: %s", signal)
		}
	}
}

func TestStopWorkloadsSendsEachContainersOwnStopSignal(t *testing.T) {
	containers := newFakeContainers([]rebootContainer{
		{ID: "a", Name: "web", StopSignal: "SIGTERM"},
		{ID: "b", Name: "nginx", StopSignal: "SIGQUIT"},
	})
	result, err := stopWorkloads(context.Background(), nil, containers, testBudget, noProgress)
	if err != nil {
		t.Fatalf("stopWorkloads: %v", err)
	}
	if len(result.Containers) != 2 {
		t.Fatalf("want 2 stopped containers, got %d", len(result.Containers))
	}
	joined := strings.Join(containers.signals, ",")
	if !strings.Contains(joined, "a=SIGTERM") || !strings.Contains(joined, "b=SIGQUIT") {
		t.Fatalf("each container must get its own stop signal, got %v", containers.signals)
	}
	assertNoKill(t, containers.signals)
}

func TestStopWorkloadsRefusesAContainerThatWillNotStop(t *testing.T) {
	containers := newFakeContainers([]rebootContainer{
		{ID: "a", Name: "web", StopSignal: "SIGTERM"},
		{ID: "b", Name: "postgres", StopSignal: "SIGINT"},
	}, "b")
	_, err := stopWorkloads(context.Background(), nil, containers, testBudget, noProgress)
	refusal, ok := AsRebootRefusal(err)
	if !ok || refusal.Reason != RebootReasonContainerTimeout {
		t.Fatalf("want a container_timeout refusal, got %v", err)
	}
	if !strings.Contains(refusal.Detail, "postgres") || strings.Contains(refusal.Detail, "web") {
		t.Fatalf("the refusal must name only the container still running, got %q", refusal.Detail)
	}
	if len(containers.signals) != 2 {
		t.Fatalf("each container is signalled exactly once and never escalated, got %v", containers.signals)
	}
	assertNoKill(t, containers.signals)
}

func TestStopWorkloadsRefusesUngracefulGuestsBeforeTouchingAny(t *testing.T) {
	guests := &fakeGuests{
		list: []rebootGuest{
			{Kind: "vm", ID: "vm1", Name: "db", Graceful: true},
			{Kind: "vm", ID: "vm2", Name: "win", Graceful: false},
		},
		stubborn: map[string]bool{}, stopped: map[string]bool{},
	}
	containers := newFakeContainers([]rebootContainer{{ID: "a", Name: "web", StopSignal: "SIGTERM"}})
	_, err := stopWorkloads(context.Background(), guests, containers, testBudget, noProgress)
	refusal, ok := AsRebootRefusal(err)
	if !ok || refusal.Reason != RebootReasonGuestNotGraceful || !strings.Contains(refusal.Detail, "win") {
		t.Fatalf("want guest_not_graceful naming win, got %v", err)
	}
	if len(guests.requested) != 0 || len(containers.signals) != 0 {
		t.Fatalf("nothing may be stopped when one guest cannot be, got guests=%v containers=%v", guests.requested, containers.signals)
	}
}

func TestStopWorkloadsStopsGuestsBeforeContainersAndAbortsOnTimeout(t *testing.T) {
	guests := &fakeGuests{
		list:     []rebootGuest{{Kind: "ct", ID: "ct1", Name: "cache", Graceful: true}},
		stubborn: map[string]bool{"ct1": true}, stopped: map[string]bool{},
	}
	containers := newFakeContainers([]rebootContainer{{ID: "a", Name: "web", StopSignal: "SIGTERM"}})
	_, err := stopWorkloads(context.Background(), guests, containers, testBudget, noProgress)
	refusal, ok := AsRebootRefusal(err)
	if !ok || refusal.Reason != RebootReasonGuestTimeout {
		t.Fatalf("want guest_timeout, got %v", err)
	}
	if len(guests.requested) != 1 {
		t.Fatalf("the guest is asked once, got %v", guests.requested)
	}
	if len(containers.signals) != 0 {
		t.Fatalf("containers must not be touched after the guest phase failed, got %v", containers.signals)
	}
}

type rebootFixture struct {
	svc        *RebootService
	containers *fakeContainers
	commands   []string
	mu         sync.Mutex
}

func newRebootFixture(t *testing.T, containers *fakeContainers) *rebootFixture {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "reboot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type) VALUES ('env-1', 'lab', 'socket')`); err != nil {
		t.Fatal(err)
	}
	if err := ensureRebootSchema(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agentPool := coreagent.NewAgentPool(logger)
	pool := coredocker.NewClientPool(database, agentPool, logger)
	fixture := &rebootFixture{containers: containers}
	svc := NewRebootService(NewServiceWithAgent(pool, agentPool, logger), database, nil, logger)
	svc.budget = testBudget
	svc.hostsFor = func(context.Context, HostTarget, string) (*rebootHosts, error) {
		return &rebootHosts{containers: containers, close: func() {}}, nil
	}
	svc.runCommand = func(_ context.Context, _ HostTarget, script string, _ bool, _ time.Duration) (*commandResult, error) {
		fixture.mu.Lock()
		fixture.commands = append(fixture.commands, script)
		fixture.mu.Unlock()
		return &commandResult{}, nil
	}
	fixture.svc = svc
	return fixture
}

func (f *rebootFixture) rebootIssued() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, command := range f.commands {
		if strings.Contains(command, "systemctl reboot") {
			return true
		}
	}
	return false
}

func (f *rebootFixture) runNow(t *testing.T) *HostReboot {
	t.Helper()
	ctx := context.Background()
	rec, err := f.svc.Schedule(ctx, EnvTarget("env-1"), HostRebootRequest{Confirm: true, Now: true}, "user-1", "alice")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if claimed, err := f.svc.store.claim(ctx, rec.ID); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	stored, err := f.svc.store.get(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}
	f.svc.execute(ctx, *stored)
	after, err := f.svc.store.get(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}
	return after
}

func TestRebootAbortsWithoutRebootingWhenAContainerWillNotStop(t *testing.T) {
	containers := newFakeContainers([]rebootContainer{
		{ID: "a", Name: "web", StopSignal: "SIGTERM", RestartPolicy: "always"},
		{ID: "b", Name: "postgres", StopSignal: "SIGINT", RestartPolicy: "unless-stopped"},
	}, "b")
	fixture := newRebootFixture(t, containers)
	after := fixture.runNow(t)

	if after.Status != RebootStatusFailed || after.FailureReason != RebootReasonContainerTimeout {
		t.Fatalf("want failed/container_timeout, got %s/%s", after.Status, after.FailureReason)
	}
	if !strings.Contains(after.LastError, "postgres") {
		t.Fatalf("the persisted error must name the container, got %q", after.LastError)
	}
	if fixture.rebootIssued() {
		t.Fatal("the reboot command ran although a container was still running")
	}
	if len(containers.started) != 2 {
		t.Fatalf("an aborted reboot starts every container it signalled again, got %v", containers.started)
	}
	assertNoKill(t, containers.signals)
}

func TestRebootIssuesTheRebootOnlyAfterEverythingStopped(t *testing.T) {
	containers := newFakeContainers([]rebootContainer{
		{ID: "a", Name: "web", StopSignal: "SIGTERM", RestartPolicy: "unless-stopped"},
		{ID: "b", Name: "oneshot", StopSignal: "SIGTERM", RestartPolicy: "no"},
		{ID: "c", Name: "task", StopSignal: "SIGTERM", RestartPolicy: "always", SwarmTask: true},
	})
	fixture := newRebootFixture(t, containers)
	after := fixture.runNow(t)

	if after.Status != RebootStatusRebooting || after.RebootIssuedAt == nil {
		t.Fatalf("want rebooting with an issue time, got %s (%s: %s)", after.Status, after.FailureReason, after.LastError)
	}
	if !fixture.rebootIssued() {
		t.Fatal("the reboot command was not run")
	}
	if len(after.plan.Containers) != 1 || after.plan.Containers[0].ID != "a" {
		t.Fatalf("only the containers a normal boot would restart are restored, got %+v", after.plan.Containers)
	}
}

func TestRebootTimeValidation(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	far := now.Add(100 * 24 * time.Hour)
	soon := now.Add(time.Hour)
	for name, req := range map[string]HostRebootRequest{
		"missing": {},
		"past":    {ScheduledAt: &past},
		"far":     {ScheduledAt: &far},
	} {
		if _, err := rebootTime(req, now); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	at, err := rebootTime(HostRebootRequest{ScheduledAt: &soon}, now)
	if err != nil || !at.Equal(soon) {
		t.Fatalf("a time an hour out is accepted as given, got %v %v", at, err)
	}
}

func TestRebootScheduleCancelAndInProgressRefusal(t *testing.T) {
	fixture := newRebootFixture(t, newFakeContainers(nil))
	ctx := context.Background()
	at := time.Now().Add(time.Hour)
	if _, err := fixture.svc.Schedule(ctx, EnvTarget("env-1"), HostRebootRequest{Confirm: true, ScheduledAt: &at}, "u", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.svc.Cancel(ctx, EnvTarget("env-1")); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := fixture.svc.Cancel(ctx, EnvTarget("env-1")); err == nil {
		t.Fatal("cancelling twice must be refused")
	}
	rec, err := fixture.svc.Schedule(ctx, EnvTarget("env-1"), HostRebootRequest{Confirm: true, ScheduledAt: &at}, "u", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.svc.store.claim(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.svc.Schedule(ctx, EnvTarget("env-1"), HostRebootRequest{Confirm: true, Now: true}, "u", "alice")
	if refusal, ok := AsRebootRefusal(err); !ok || refusal.Reason != RebootReasonInProgress {
		t.Fatalf("scheduling over a running reboot must be refused, got %v", err)
	}
	if err := fixture.svc.Cancel(ctx, EnvTarget("env-1")); err == nil {
		t.Fatal("a running reboot cannot be cancelled")
	}
	if _, err := fixture.svc.Schedule(ctx, EnvTarget("missing"), HostRebootRequest{Confirm: true, Now: true}, "u", "alice"); !errors.Is(err, errRebootTargetMissing) {
		t.Fatalf("an unknown environment is refused, got %v", err)
	}
}

func TestRebootRefusedInDemoMode(t *testing.T) {
	dir := t.TempDir()
	marker, err := demo.NewMarker("u")
	if err != nil {
		t.Fatal(err)
	}
	if err := demo.WriteMarker(dir, marker); err != nil {
		t.Fatal(err)
	}
	if _, err := demo.Load(dir); err != nil {
		t.Fatal(err)
	}
	resetDir := t.TempDir()
	t.Cleanup(func() {
		if _, err := demo.Load(resetDir); err != nil {
			t.Error(err)
		}
	})

	fixture := newRebootFixture(t, newFakeContainers(nil))
	_, err = fixture.svc.Schedule(context.Background(), EnvTarget("env-1"), HostRebootRequest{Confirm: true, Now: true}, "u", "alice")
	if refusal, ok := AsRebootRefusal(err); !ok || refusal.Reason != RebootReasonDemoMode {
		t.Fatalf("demo mode must refuse, got %v", err)
	}
	state, err := fixture.svc.State(context.Background(), EnvTarget("env-1"))
	if err != nil || state.BlockedReason != RebootReasonDemoMode {
		t.Fatalf("state must say demo_mode, got %+v %v", state, err)
	}
}
