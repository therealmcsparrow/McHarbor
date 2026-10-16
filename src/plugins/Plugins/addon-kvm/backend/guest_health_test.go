// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const gib = int64(1) << 30

func healthVM(startedMinute int) VM {
	vm := startedVM(StateRunning, startedMinute)
	vm.RunState = "running"
	vm.GuestAgentState = "connected"
	return vm
}

func okProbe(fss []agentGuestFilesystem, skewMs *int64) healthProbe {
	return healthProbe{outcome: healthOK, guest: &agentGuest{AgentState: "connected", Filesystems: fss, ClockSkewMs: skewMs}}
}

func fsUsed(mount string, used, total int64) agentGuestFilesystem {
	return agentGuestFilesystem{Mountpoint: mount, Type: "ext4", UsedBytes: used, TotalBytes: total}
}

func int64Ptr(v int64) *int64 { return &v }

func foldProbes(vm VM, start int, probes ...healthProbe) guestHealth {
	var h guestHealth
	for i, probe := range probes {
		h, _ = nextGuestHealth(h, vm, probe, incidentAt(start+2*i))
	}
	return h
}

func TestIncidentGuestHealthProbeMapping(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		reply *agentPayload
		err   error
		want  healthOutcome
	}{
		{"host unreachable", nil, fmt.Errorf("%w: agent_offline", ErrHostUnavailable), healthSkipped},
		{"agent too old", nil, errors.New(`agent failed vm_guest_query: compute: "health" is not a guest query this agent answers`), healthTooOld},
		{"ping failed", nil, errors.New("agent failed vm_guest_query: guest-ping timed out"), healthFailed},
		{"empty reply", &agentPayload{}, nil, healthFailed},
		{"socket absent", &agentPayload{Guest: &agentGuest{AgentState: "absent"}}, nil, healthAbsent},
		{"answered", &agentPayload{Guest: &agentGuest{AgentState: "connected"}}, nil, healthOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyProbe(ctx, tc.reply, tc.err).outcome; got != tc.want {
				t.Fatalf("outcome = %v, want %v", got, tc.want)
			}
		})
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if got := classifyProbe(cancelled, nil, errors.New("context canceled")).outcome; got != healthSkipped {
		t.Fatalf("a cancelled sweep is a skip, got %v", got)
	}
}

func TestIncidentGuestHealthFailureStreak(t *testing.T) {
	vm := healthVM(0)
	failed := healthProbe{outcome: healthFailed, err: "timeout"}
	h := foldProbes(vm, 10, okProbe(nil, nil), failed, failed, failed)
	if h.failures != 3 || !h.failingSince.Equal(incidentAt(12)) || h.lastOKAt != incidentAt(10) {
		t.Fatalf("streak = %d since %v, last ok %v", h.failures, h.failingSince, h.lastOKAt)
	}
	h, _ = nextGuestHealth(h, vm, okProbe(nil, nil), incidentAt(20))
	if h.failures != 0 || !h.failingSince.IsZero() {
		t.Fatalf("an answer must end the streak, got %d", h.failures)
	}

	h = foldProbes(vm, 10, okProbe(nil, nil), failed, failed)
	h, _ = nextGuestHealth(h, vm, healthProbe{outcome: healthAbsent}, incidentAt(20))
	if h.failures != 0 || h.agentState != "absent" {
		t.Fatalf("an absent socket must not count as a miss, got %+v", h)
	}

	restarted := healthVM(30)
	h = foldProbes(vm, 10, okProbe(nil, nil), failed, failed)
	h, _ = nextGuestHealth(h, restarted, failed, incidentAt(32))
	if h.failures != 1 {
		t.Fatalf("a new run starts a new streak, got %d", h.failures)
	}
}

func TestIncidentGuestHealthSkippedWritesNothing(t *testing.T) {
	if _, write := nextGuestHealth(guestHealth{}, healthVM(0), healthProbe{outcome: healthSkipped}, incidentAt(1)); write {
		t.Fatal("a skipped probe must not be stored")
	}
}

func TestIncidentGuestDiskLatch(t *testing.T) {
	vm := healthVM(0)
	step := func(prev guestHealth, used int64, minute int) guestHealth {
		next, _ := nextGuestHealth(prev, vm, okProbe([]agentGuestFilesystem{fsUsed("/", used, 100)}, nil), incidentAt(minute))
		return next
	}
	h := step(guestHealth{}, 92, 10)
	if h.diskAlert {
		t.Fatal("92% must not raise on its own")
	}
	h = step(h, 95, 12)
	if !h.diskAlert {
		t.Fatal("95% must raise")
	}
	h = step(h, 91, 14)
	if !h.diskAlert {
		t.Fatal("91% must hold a raised alert")
	}
	h = step(h, 89, 16)
	if h.diskAlert {
		t.Fatal("89% must settle")
	}

	small := []agentGuestFilesystem{fsUsed("/data", 15*gib-gib*8/10, 15*gib)}
	if !diskLatched(false, toGuestFilesystems(small)) {
		t.Fatal("under 1 GiB free on a 15 GiB filesystem must raise")
	}
	tiny := []agentGuestFilesystem{fsUsed("/boot", 470<<20, 512<<20)}
	if diskLatched(false, toGuestFilesystems(tiny)) {
		t.Fatal("a small filesystem below 95% must not raise on free space alone")
	}
}

func toGuestFilesystems(in []agentGuestFilesystem) []guestFilesystem {
	out := make([]guestFilesystem, 0, len(in))
	for _, fs := range in {
		out = append(out, guestFilesystem{Mountpoint: fs.Mountpoint, Type: fs.Type, UsedBytes: fs.UsedBytes, TotalBytes: fs.TotalBytes})
	}
	return out
}

func TestIncidentGuestClockSkewLatch(t *testing.T) {
	vm := healthVM(0)
	step := func(prev guestHealth, skewSeconds int64, minute int) guestHealth {
		next, _ := nextGuestHealth(prev, vm, okProbe(nil, int64Ptr(skewSeconds*1000)), incidentAt(minute))
		return next
	}
	h := step(guestHealth{}, 90, 10)
	if h.skewAlert {
		t.Fatal("one skewed check must not raise")
	}
	h = step(h, -90, 12)
	if !h.skewAlert {
		t.Fatal("two skewed checks must raise")
	}
	h = step(h, 45, 14)
	if !h.skewAlert {
		t.Fatal("45 s must hold a raised skew")
	}
	h = step(h, 20, 16)
	if h.skewAlert {
		t.Fatal("20 s must settle")
	}
}

func unresponsiveHealth(vm VM, checkedMinute int) *guestHealth {
	return &guestHealth{
		vmID: vm.ID, runStartedAt: startedAt(vm), checkedAt: incidentAt(checkedMinute),
		failures: 3, failingSince: incidentAt(checkedMinute - 4), lastOKAt: incidentAt(checkedMinute - 6),
	}
}

func TestIncidentGuestUnresponsiveRules(t *testing.T) {
	vm := healthVM(0)
	requireSignals(t, classifyGuestHealth(vm, unresponsiveHealth(vm, 20), nil, incidentAt(21)), signalVMGuestUnresponsive)

	booting := healthVM(18)
	requireSignals(t, classifyGuestHealth(booting, unresponsiveHealth(booting, 20), nil, incidentAt(21)))

	neverAnswered := unresponsiveHealth(vm, 20)
	neverAnswered.lastOKAt = time.Time{}
	requireSignals(t, classifyGuestHealth(vm, neverAnswered, nil, incidentAt(21)))

	shuttingDown := []lifecycleIntent{{at: incidentAt(19), action: intentShutdown, outcome: intentPending}}
	requireSignals(t, classifyGuestHealth(vm, unresponsiveHealth(vm, 20), shuttingDown, incidentAt(21)))

	tooOld := unresponsiveHealth(vm, 20)
	tooOld.agentTooOld = true
	requireSignals(t, classifyGuestHealth(vm, tooOld, nil, incidentAt(21)))

	requireSignals(t, classifyGuestHealth(vm, unresponsiveHealth(vm, 20), nil, incidentAt(20).Add(healthStale+time.Minute)))

	unreachable := vm
	unreachable.HostAvailable = false
	requireSignals(t, classifyGuestHealth(unreachable, unresponsiveHealth(vm, 20), nil, incidentAt(21)))

	paused := vm
	paused.State, paused.RunState = StatePaused, "paused"
	requireSignals(t, classifyGuestHealth(paused, unresponsiveHealth(vm, 20), nil, incidentAt(21)))

	previousRun := unresponsiveHealth(vm, 20)
	previousRun.runStartedAt = incidentAt(-60)
	requireSignals(t, classifyGuestHealth(vm, previousRun, nil, incidentAt(21)))
}

func TestIncidentGuestDiskAndSkewSignals(t *testing.T) {
	vm := healthVM(0)
	h := foldProbes(vm, 10,
		okProbe([]agentGuestFilesystem{fsUsed("/", 40*gib, 100*gib), fsUsed("/var", 97*gib, 100*gib)}, int64Ptr(-75_000)),
		okProbe([]agentGuestFilesystem{fsUsed("/", 40*gib, 100*gib), fsUsed("/var", 97*gib, 100*gib)}, int64Ptr(-75_000)),
	)
	found := classifyGuestHealth(vm, &h, nil, incidentAt(13))
	requireSignals(t, found, signalVMGuestDiskFull, signalVMGuestClockSkew)
	if !strings.Contains(found[0].detail, "/var is 97% full") {
		t.Fatalf("disk detail = %q", found[0].detail)
	}
	if !strings.Contains(found[1].detail, "-75 s") {
		t.Fatalf("skew detail = %q", found[1].detail)
	}

	in := incidentInputs{vm: vm, now: incidentAt(13), intentsOK: true, health: &h}
	if incidentSettled(in) {
		t.Fatal("a full disk is not settled")
	}
	h, _ = nextGuestHealth(h, vm, okProbe([]agentGuestFilesystem{fsUsed("/var", 92*gib, 100*gib)}, int64Ptr(1000)), incidentAt(14))
	in.health, in.now = &h, incidentAt(15)
	if incidentSettled(in) {
		t.Fatal("92% is above the clear line and must not settle")
	}
	h, _ = nextGuestHealth(h, vm, okProbe([]agentGuestFilesystem{fsUsed("/var", 80*gib, 100*gib)}, int64Ptr(1000)), incidentAt(16))
	in.health, in.now = &h, incidentAt(17)
	if !incidentSettled(in) {
		t.Fatalf("80%% with a sane clock is settled, got %v", signalKeys(classifyIncident(in)))
	}

	stopped := vm
	stopped.State = StateStopped
	full := foldProbes(vm, 10, okProbe([]agentGuestFilesystem{fsUsed("/", 99, 100)}, nil))
	if !incidentSettled(incidentInputs{vm: stopped, now: incidentAt(12), intentsOK: true, health: &full}) {
		t.Fatal("a guest stopped with a full disk raises nothing about the disk")
	}
}

func TestIncidentGuestHealthPollerStops(t *testing.T) {
	p := newHealthPoller(nil, quietLogger{})
	p.interval = 5 * time.Millisecond
	var calls atomic.Int64
	blocked := make(chan struct{}, 1)
	p.sweep = func(ctx context.Context) {
		calls.Add(1)
		select {
		case blocked <- struct{}{}:
		default:
		}
		<-ctx.Done()
	}
	p.Start()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("the poller never swept")
	}

	stopped := make(chan struct{})
	go func() {
		p.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not cancel the in-flight sweep")
	}
	after := calls.Load()
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != after {
		t.Fatal("the poller kept sweeping after Stop")
	}
	p.Stop()

	never := newHealthPoller(nil, quietLogger{})
	done := make(chan struct{})
	go func() {
		never.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop on a poller that never started must return")
	}
}

func TestIncidentGuestHealthStoreRoundTrip(t *testing.T) {
	svc, _, _ := newIntentTestService(t)
	ctx := context.Background()
	load := 1.5
	want := guestHealth{
		vmID: "vm1", envID: "env1", runStartedAt: incidentAt(0), checkedAt: incidentAt(10),
		reachable: false, agentState: "connected", failures: 2, failingSince: incidentAt(8),
		lastOKAt: incidentAt(6), lastError: "timeout",
		filesystems: []guestFilesystem{{Mountpoint: "/", Type: "xfs", UsedBytes: 5, TotalBytes: 10}},
		load1:       &load, load5: &load, load15: &load, clockSkewMs: int64Ptr(-1200),
		skewOver: 1, diskAlert: true, unsupported: []string{"guest-get-load"},
	}
	if err := svc.saveGuestHealth(ctx, want); err != nil {
		t.Fatal(err)
	}
	want.failures = 3
	if err := svc.saveGuestHealth(ctx, want); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.guestHealthRows(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := rows["vm1"]
	if len(rows) != 1 || !ok {
		t.Fatalf("rows = %d, want one upserted row", len(rows))
	}
	if got.failures != 3 || !got.runStartedAt.Equal(want.runStartedAt) || !got.failingSince.Equal(want.failingSince) ||
		!got.lastOKAt.Equal(want.lastOKAt) || got.lastError != "timeout" || !got.diskAlert || got.skewOver != 1 ||
		got.load5 == nil || *got.load5 != load || got.clockSkewMs == nil || *got.clockSkewMs != -1200 ||
		len(got.filesystems) != 1 || got.filesystems[0] != want.filesystems[0] ||
		len(got.unsupported) != 1 || got.unsupported[0] != "guest-get-load" {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestIncidentGuestHealthSweepStoresResult(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	insertIntentTestVM(t, db, "vm2", StateRunning)
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_vms SET guest_agent_state = 'connected' WHERE id = 'vm1'`); err != nil {
		t.Fatal(err)
	}
	rpc.reply = []byte(`{"guest":{"agentState":"connected","filesystems":[{"mountpoint":"/","type":"ext4","usedBytes":96,"totalBytes":100}],"clockSkewMs":5}}`)

	p := newHealthPoller(svc, quietLogger{})
	p.sweepAll(context.Background())

	rows, err := svc.guestHealthRows(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, polled := rows["vm2"]; polled {
		t.Fatal("a guest whose agent was never seen must not be polled")
	}
	got, ok := rows["vm1"]
	if !ok || !got.reachable || !got.diskAlert || len(got.filesystems) != 1 {
		t.Fatalf("stored = %+v", got)
	}
}
