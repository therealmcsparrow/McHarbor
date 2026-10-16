// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"
	"time"
)

var stabilityNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func stabilityAt(minutesAgo int) time.Time {
	return stabilityNow.Add(-time.Duration(minutesAgo) * time.Minute)
}

func shutdownEvent(minutesAgo int, reason string) vmEvent {
	return vmEvent{at: stabilityAt(minutesAgo), kind: qemuEventShutdown, reason: reason}
}

func TestClassifyStabilityEvent(t *testing.T) {
	panicked := vmEvent{at: stabilityAt(60), kind: qemuEventGuestPanic}
	shutdownIntent := lifecycleIntent{at: stabilityAt(121), finishedAt: stabilityAt(120),
		action: intentShutdown, outcome: intentOK}

	cases := []struct {
		name    string
		event   vmEvent
		events  []vmEvent
		intents []lifecycleIntent
		want    string
	}{
		{"panic is a crash", panicked, nil, nil, stabilityCrash},
		{"panic poweroff after a panic event is the same crash",
			shutdownEvent(59, shutdownReasonGuestPanic), []vmEvent{panicked}, nil, ""},
		{"panic poweroff without a panic event is a crash",
			shutdownEvent(59, shutdownReasonGuestPanic), nil, nil, stabilityCrash},
		{"host signal with no action is unexpected",
			shutdownEvent(30, shutdownReasonHostSignal), nil, nil, stabilityUnexpectedStop},
		{"stop with no reason is unexpected", shutdownEvent(30, ""), nil, nil, stabilityUnexpectedStop},
		{"stop McHarbor asked for is routine",
			shutdownEvent(120, shutdownReasonHostQMPQuit), nil, []lifecycleIntent{shutdownIntent}, ""},
		{"guest powering itself off is its own kind",
			shutdownEvent(30, shutdownReasonGuestShutdown), nil, nil, stabilityGuestShutdown},
		{"unrequested reset", vmEvent{at: stabilityAt(10), kind: qemuEventReset}, nil, nil, stabilityReset},
		{"requested reset is routine", vmEvent{at: stabilityAt(120), kind: qemuEventReset}, nil,
			[]lifecycleIntent{shutdownIntent}, ""},
		{"I/O error", vmEvent{at: stabilityAt(5), kind: qemuEventBlockIOError}, nil, nil, stabilityIOError},
		{"pause is routine", vmEvent{at: stabilityAt(5), kind: qemuEventStop}, nil, nil, ""},
	}
	for _, c := range cases {
		if got := classifyStabilityEvent(c.event, c.events, c.intents); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIsFlapping(t *testing.T) {
	loop := []time.Time{stabilityAt(40), stabilityAt(30), stabilityAt(15)}
	if !isFlapping(loop) {
		t.Fatal("three downs within 30 minutes did not flap")
	}
	spread := []time.Time{stabilityAt(3000), stabilityAt(2000), stabilityAt(1000)}
	if isFlapping(spread) {
		t.Fatal("three downs days apart flapped")
	}
	many := []time.Time{stabilityAt(9000), stabilityAt(7000), stabilityAt(5000), stabilityAt(3000), stabilityAt(1000)}
	if !isFlapping(many) {
		t.Fatalf("%d downs in one period did not flap", flapPeriodEvents)
	}
	if got := maxBurst(loop, restartLoopWindow); got != 3 {
		t.Fatalf("maxBurst = %d, want 3", got)
	}
}

func TestIntentStuck(t *testing.T) {
	pending := lifecycleIntent{at: stabilityNow.Add(-intentPendingCover - time.Minute), outcome: intentPending}
	if !intentStuck(pending, stabilityNow) {
		t.Fatal("a pending action older than the longest legitimate one is not stuck")
	}
	recent := lifecycleIntent{at: stabilityNow.Add(-intentPendingCover + time.Minute), outcome: intentPending}
	if intentStuck(recent, stabilityNow) {
		t.Fatal("a pending action inside its cover is stuck")
	}
	failed := pending
	failed.outcome = intentFailed
	if intentStuck(failed, stabilityNow) {
		t.Fatal("a failed action is counted as stuck")
	}
}

func TestGuestAgentLabel(t *testing.T) {
	running := VM{State: StateRunning}
	cases := []struct {
		name   string
		health *guestHealth
		want   string
		status string
	}{
		{"no record", nil, "not checked", ""},
		{"answering", &guestHealth{reachable: true, lastOKAt: stabilityNow}, "answering", "success"},
		{"never answered", &guestHealth{failures: 9}, "never answered", "warning"},
		{"unresponsive", &guestHealth{failures: unresponsiveFailures, lastOKAt: stabilityAt(60)},
			"unresponsive (3 missed)", "danger"},
		{"one miss", &guestHealth{failures: 1, lastOKAt: stabilityAt(5)}, "1 missed check", "warning"},
		{"old agent", &guestHealth{agentTooOld: true}, "host agent too old", "warning"},
	}
	for _, c := range cases {
		text, status := guestAgentLabel(running, c.health)
		if text != c.want || status != c.status {
			t.Errorf("%s: got %q/%q, want %q/%q", c.name, text, status, c.want, c.status)
		}
	}
}

func TestAssessStabilityWindowAndTotals(t *testing.T) {
	from, to := stabilityNow.Add(-24*time.Hour), stabilityNow
	vm := VM{ID: "1", Name: "db", State: StateRunning}
	a := assessStability([]stabilityVMInput{{
		vm: vm,
		health: &guestHealth{failures: unresponsiveFailures, lastOKAt: stabilityAt(90),
			diskAlert: true},
		events: []vmEvent{
			shutdownEvent(3000, shutdownReasonHostSignal),
			{at: stabilityAt(50), kind: qemuEventGuestPanic},
			shutdownEvent(40, shutdownReasonHostSignal),
			shutdownEvent(30, shutdownReasonGuestShutdown),
			{at: stabilityAt(20), kind: qemuEventReset},
		},
		intents: []lifecycleIntent{
			{at: stabilityAt(600), finishedAt: stabilityAt(600), action: intentStart, outcome: intentFailed},
			{at: stabilityAt(300), action: intentShutdown, outcome: intentPending},
			{at: stabilityAt(5000), action: intentStart, outcome: intentFailed},
		},
	}, {vm: VM{ID: "2", Name: "quiet", State: StateStopped}}}, from, to, stabilityNow)

	if a.crashes != 1 || a.stops != 1 || a.resets != 1 {
		t.Fatalf("crashes/stops/resets = %d/%d/%d, want 1/1/1: the stop before the window must not count",
			a.crashes, a.stops, a.resets)
	}
	if len(a.flapping) != 1 || a.flapping[0] != "db" {
		t.Fatalf("flapping = %v, want [db]", a.flapping)
	}
	if a.failed != 1 || a.stuck != 1 {
		t.Fatalf("failed/stuck = %d/%d, want 1/1", a.failed, a.stuck)
	}
	if len(a.unresponsive) != 1 || len(a.diskAlerts) != 1 {
		t.Fatalf("unresponsive/disk = %v/%v, want db in both", a.unresponsive, a.diskAlerts)
	}
	if a.rows[0].vm.Name != "db" || len(a.incidents) != 4 {
		t.Fatalf("rows[0] = %s, incidents = %d; want db first and 4 classified events", a.rows[0].vm.Name, len(a.incidents))
	}
	if len(stabilityFindings(a)) < 5 {
		t.Fatalf("findings = %s, want crash, flapping, stop, agent, actions and more", findingTitles(stabilityFindings(a)))
	}
	if len(stabilityFindings(stabilityAssessment{guests: 3})) != 0 {
		t.Fatal("a quiet estate raised findings")
	}
}
