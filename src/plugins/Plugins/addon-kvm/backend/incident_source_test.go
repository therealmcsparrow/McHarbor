// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
	"testing"
	"time"
)

var incidentBase = time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)

func incidentAt(minute int) time.Time {
	return incidentBase.Add(time.Duration(minute) * time.Minute)
}

func incidentVM(state, runState string) VM {
	return VM{ID: "vm1", EnvironmentID: "env1", Name: "db", State: state, RunState: runState, HostAvailable: true}
}

func startedVM(state string, startedMinute int) VM {
	vm := incidentVM(state, "")
	started := incidentAt(startedMinute)
	vm.LastStartedAt = &started
	return vm
}

func signalKeys(found []classifiedSignal) []string {
	keys := make([]string, 0, len(found))
	for _, f := range found {
		keys = append(keys, f.signal)
	}
	return keys
}

func requireSignals(t *testing.T, found []classifiedSignal, want ...string) {
	t.Helper()
	got := signalKeys(found)
	if len(got) != len(want) {
		t.Fatalf("signals = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("signals = %v, want %v", got, want)
		}
	}
}

func finishedIntent(action string, atMinute, finishedMinute int) lifecycleIntent {
	return lifecycleIntent{
		at: incidentAt(atMinute), finishedAt: incidentAt(finishedMinute),
		action: action, actor: "user:u1", source: "api", outcome: intentOK,
	}
}

func stoppedMark(atMinute, startedMinute int) downMark {
	return downMark{at: incidentAt(atMinute), startedAt: incidentAt(startedMinute), state: StateStopped}
}

func TestIncidentClassifyVM(t *testing.T) {
	cases := []struct {
		name string
		vm   VM
		want []string
	}{
		{"running is healthy", incidentVM(StateRunning, "running"), nil},
		{"deliberate pause is healthy", incidentVM(StatePaused, "paused"), nil},
		{"stopped is healthy on state alone", incidentVM(StateStopped, ""), nil},
		{"failed unit is a crash", incidentVM(StateCrashed, ""), []string{signalVMCrashed}},
		{"io-error pause", incidentVM(StatePaused, RunStateIOError), []string{signalVMPausedError}},
		{"guest panic", incidentVM(StateStopping, RunStateGuestPanicked), []string{signalVMGuestPanic}},
		{"monitor unreachable", incidentVM(StateUnknown, ""), []string{signalVMUnresponsive}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireSignals(t, classifyVM(tc.vm), tc.want...)
		})
	}
}

func TestIncidentUnreachableHostRaisesNothing(t *testing.T) {
	vm := startedVM(StateStopped, 0)
	vm.HostAvailable = false
	in := incidentInputs{
		vm: vm, now: incidentAt(20), intentsOK: true,
		downs:  []downMark{stoppedMark(10, 0), {at: incidentAt(12), state: StateCrashed}, {at: incidentAt(14), state: StateCrashed}},
		events: []vmEvent{{at: incidentAt(10), kind: qemuEventShutdown, reason: shutdownReasonHostSignal}},
	}
	if got := classifyIncident(in); len(got) != 0 {
		t.Fatalf("a guest on an unreachable host must not raise %v", signalKeys(got))
	}
	if incidentSettled(in) {
		t.Fatal("an unreachable host must not read as recovery")
	}
}

func TestIncidentTemplatesRaiseNothing(t *testing.T) {
	vm := incidentVM(StateCrashed, "")
	vm.IsTemplate = true
	if got := classifyIncident(incidentInputs{vm: vm, now: incidentAt(1), intentsOK: true}); len(got) != 0 {
		t.Fatalf("a template must not raise %v", signalKeys(got))
	}
}

func TestIncidentCrashDetailCarriesJournalReason(t *testing.T) {
	vm := incidentVM(StateCrashed, "")
	vm.LastError = "oom-kill"
	found := classifyVM(vm)
	if len(found) != 1 || found[0].detail != "oom-kill" {
		t.Fatalf("detail = %+v, want the stored last error", found)
	}
}

func TestIncidentDeliberateShutdownRaisesNothing(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateStopped, 0), now: incidentAt(12), intentsOK: true,
		downs:   []downMark{stoppedMark(10, 0)},
		intents: []lifecycleIntent{finishedIntent(intentShutdown, 8, 9)},
	}
	requireSignals(t, classifyIncident(in))
	if !incidentSettled(in) {
		t.Fatal("a deliberately shut down guest is settled")
	}
}

func TestIncidentKillRaisesNothing(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateStopped, 0), now: incidentAt(12), intentsOK: true,
		intents: []lifecycleIntent{finishedIntent(intentKill, 9, 9)},
		events:  []vmEvent{{at: incidentAt(9), kind: qemuEventShutdown, reason: shutdownReasonHostSignal}},
	}
	requireSignals(t, classifyIncident(in))
}

func TestIncidentStopWithoutIntentIsUnexpected(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateStopped, 0), now: incidentAt(12), intentsOK: true,
		downs: []downMark{stoppedMark(10, 0)},
	}
	found := classifyIncident(in)
	requireSignals(t, found, signalVMStoppedUnexpected)
	if !strings.Contains(found[0].detail, "guest-initiated or external") {
		t.Fatalf("detail = %q, want the honest guest-initiated-or-external wording", found[0].detail)
	}
	if !found[0].at.Equal(incidentAt(10)) {
		t.Fatalf("at = %v, want the observed stop", found[0].at)
	}
	if incidentSettled(in) {
		t.Fatal("an unexplained stop is not settled")
	}
}

func TestIncidentLaterKillSettlesUnexpectedStop(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateStopped, 0), now: incidentAt(40), intentsOK: true,
		downs:   []downMark{stoppedMark(10, 0)},
		intents: []lifecycleIntent{finishedIntent(intentKill, 30, 30)},
	}
	if !incidentSettled(in) {
		t.Fatalf("a kill after the stop settles it, got %v", signalKeys(classifyIncident(in)))
	}
}

func TestIncidentPreviousRunIntentDoesNotExcuse(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateStopped, 3), now: incidentAt(8), intentsOK: true,
		downs: []downMark{stoppedMark(6, 3)},
		intents: []lifecycleIntent{
			finishedIntent(intentShutdown, 0, 2),
			finishedIntent(intentStart, 3, 3),
		},
	}
	requireSignals(t, classifyIncident(in), signalVMStoppedUnexpected)
}

func TestIncidentUnobservedStopRaisesNothing(t *testing.T) {
	in := incidentInputs{vm: startedVM(StateStopped, 0), now: incidentAt(12), intentsOK: true}
	requireSignals(t, classifyIncident(in))
}

func TestIncidentUnreadableIntentsRaiseNoStop(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateStopped, 0), now: incidentAt(12), intentsOK: false,
		downs: []downMark{stoppedMark(10, 0)},
	}
	requireSignals(t, classifyIncident(in))
}

func TestIncidentShutdownReasonMapping(t *testing.T) {
	cases := []struct {
		reason string
		signal string
		detail string
	}{
		{shutdownReasonGuestShutdown, signalVMGuestShutdown, "shut down from inside the guest"},
		{shutdownReasonHostSignal, signalVMStoppedUnexpected, "killed on the host (signal)"},
		{shutdownReasonHostQMPQuit, signalVMStoppedUnexpected, "QEMU's monitor on the host"},
		{shutdownReasonHostUI, signalVMStoppedUnexpected, "QEMU window on the host"},
		{shutdownReasonGuestPanic, signalVMGuestPanic, "panicked"},
		{shutdownReasonSubsystem, signalVMStoppedUnexpected, "stopped by QEMU (subsystem-reset)"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			in := incidentInputs{
				vm: startedVM(StateStopped, 0), now: incidentAt(12), intentsOK: true,
				events: []vmEvent{{at: incidentAt(9), kind: qemuEventShutdown, reason: tc.reason}},
			}
			found := classifyIncident(in)
			requireSignals(t, found, tc.signal)
			if !strings.Contains(found[0].detail, tc.detail) {
				t.Fatalf("detail = %q, want it to contain %q", found[0].detail, tc.detail)
			}
		})
	}
}

func TestIncidentCrashWinsOverStop(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateCrashed, 0), now: incidentAt(12), intentsOK: true,
		events: []vmEvent{{at: incidentAt(9), kind: qemuEventShutdown, reason: shutdownReasonHostSignal}},
	}
	requireSignals(t, classifyIncident(in), signalVMCrashed)
}

func TestIncidentPanicEventRaisesImmediately(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateRunning, 0), now: incidentAt(11), intentsOK: true,
		events: []vmEvent{{at: incidentAt(10), kind: qemuEventGuestPanic}},
	}
	requireSignals(t, classifyIncident(in), signalVMGuestPanic)

	in.events = append(in.events, vmEvent{at: incidentAt(10), kind: qemuEventReset})
	requireSignals(t, classifyIncident(in))
}

func TestIncidentPanicClearedByLaterKill(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StatePaused, 0), now: incidentAt(20), intentsOK: true,
		events:  []vmEvent{{at: incidentAt(10), kind: qemuEventGuestPanic}},
		intents: []lifecycleIntent{finishedIntent(intentKill, 15, 15)},
	}
	requireSignals(t, classifyIncident(in))
}

func TestIncidentBlockIOErrorEvent(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StatePaused, 0), now: incidentAt(11), intentsOK: true,
		events: []vmEvent{{at: incidentAt(10), kind: qemuEventBlockIOError, device: "drive-virtio0", reason: "No space left on device"}},
	}
	found := classifyIncident(in)
	requireSignals(t, found, signalVMPausedError)
	if !strings.Contains(found[0].detail, "drive-virtio0") || !strings.Contains(found[0].detail, "No space left") {
		t.Fatalf("detail = %q, want device and reason", found[0].detail)
	}

	in.events = append(in.events, vmEvent{at: incidentAt(10), kind: qemuEventResume})
	requireSignals(t, classifyIncident(in))
}

func TestIncidentRestartLoopFromCrashes(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateRunning, 20), now: incidentAt(21), intentsOK: true,
		downs: []downMark{
			{at: incidentAt(5), startedAt: incidentAt(0), state: StateCrashed},
			{at: incidentAt(12), startedAt: incidentAt(8), state: StateCrashed},
			{at: incidentAt(19), startedAt: incidentAt(15), state: StateCrashed},
		},
	}
	requireSignals(t, classifyIncident(in), signalVMRestartLoop)
	if incidentSettled(in) {
		t.Fatal("a running guest inside a crash loop is not settled")
	}
}

func TestIncidentUserRestartsAreNotALoop(t *testing.T) {
	in := incidentInputs{
		vm: startedVM(StateRunning, 20), now: incidentAt(21), intentsOK: true,
		downs: []downMark{stoppedMark(5, 0), stoppedMark(12, 6), stoppedMark(19, 13)},
		intents: []lifecycleIntent{
			finishedIntent(intentStart, 0, 0),
			finishedIntent(intentShutdown, 4, 5),
			finishedIntent(intentStart, 6, 6),
			finishedIntent(intentShutdown, 11, 12),
			finishedIntent(intentStart, 13, 13),
			finishedIntent(intentKill, 18, 18),
			finishedIntent(intentStart, 20, 20),
		},
	}
	requireSignals(t, classifyIncident(in))
}

func TestIncidentGuestResetsOnlyLoopWhenRepeated(t *testing.T) {
	resets := func(n int) []vmEvent {
		var out []vmEvent
		for i := 0; i < n; i++ {
			out = append(out, vmEvent{at: incidentAt(10 + i), kind: qemuEventReset})
		}
		return out
	}
	in := incidentInputs{vm: startedVM(StateRunning, 0), now: incidentAt(20), intentsOK: true, events: resets(2)}
	requireSignals(t, classifyIncident(in))

	in.events = resets(restartLoopEvents)
	requireSignals(t, classifyIncident(in), signalVMRestartLoop)

	in.intents = []lifecycleIntent{finishedIntent(intentReset, 9, 15)}
	requireSignals(t, classifyIncident(in))
}

func TestIncidentSettledStates(t *testing.T) {
	cases := []struct {
		name string
		vm   VM
		want bool
	}{
		{"running again", incidentVM(StateRunning, "running"), true},
		{"stopped with no observed stop", incidentVM(StateStopped, ""), true},
		{"paused by a user", incidentVM(StatePaused, "paused"), true},
		{"still crashed", incidentVM(StateCrashed, ""), false},
		{"still paused on io-error", incidentVM(StatePaused, RunStateIOError), false},
		{"shutdown in flight", incidentVM(StateStopping, "shutdown"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := incidentInputs{vm: tc.vm, now: incidentAt(1), intentsOK: true}
			if got := incidentSettled(in); got != tc.want {
				t.Fatalf("incidentSettled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIncidentIntentCovers(t *testing.T) {
	done := finishedIntent(intentShutdown, 10, 12)
	if !intentCovers(done, incidentAt(12).Add(intentSlack)) {
		t.Fatal("a stop seen within the slack after the action finished is covered")
	}
	if intentCovers(done, incidentAt(12).Add(intentSlack+time.Second)) {
		t.Fatal("a stop seen after the slack is not covered")
	}
	if intentCovers(done, incidentAt(10).Add(-intentSkew-time.Second)) {
		t.Fatal("a stop before the action is not covered")
	}
	pending := lifecycleIntent{at: incidentAt(10), action: intentShutdown, outcome: intentPending}
	if !intentCovers(pending, incidentAt(20)) || intentCovers(pending, incidentAt(10).Add(intentPendingCover+intentSlack+time.Second)) {
		t.Fatal("a pending action covers a bounded span")
	}
}

func TestIncidentTrackerMarksObservedStops(t *testing.T) {
	tracker := newIncidentTracker()
	observe := func(minute int, state string, startedMinute int) {
		vm := startedVM(state, startedMinute)
		tracker.observeAll(incidentAt(minute), []VM{vm})
	}

	observe(1, StateStopped, 0)
	if got := len(tracker.downs("vm1")); got != 0 {
		t.Fatalf("a guest first seen stopped produced %d marks", got)
	}

	observe(2, StateRunning, 2)
	observe(3, StateStopped, 2)
	observe(4, StateStopped, 2)
	observe(5, StateRunning, 5)
	observe(6, StateCrashed, 5)
	observe(7, StateCrashed, 5)

	marks := tracker.downs("vm1")
	if len(marks) != 2 || marks[0].state != StateStopped || marks[1].state != StateCrashed {
		t.Fatalf("marks = %+v, want one stop and one crash", marks)
	}
}

func TestIncidentTrackerIgnoresUnreachableAndForgetsDeleted(t *testing.T) {
	tracker := newIncidentTracker()

	vm := incidentVM(StateCrashed, "")
	vm.HostAvailable = false
	tracker.observeAll(incidentAt(0), []VM{vm})
	if got := len(tracker.downs("vm1")); got != 0 {
		t.Fatalf("an unreachable host produced %d marks", got)
	}

	vm.HostAvailable = true
	tracker.observeAll(incidentAt(0), []VM{vm})
	tracker.observeAll(incidentAt(1), nil)
	if got := len(tracker.history("vm1")); got != 0 {
		t.Fatalf("a deleted guest kept %d observations", got)
	}
}

func TestIncidentEvidenceSectionsWindow(t *testing.T) {
	history := []vmObservation{
		{at: incidentAt(0), state: StateRunning},
		{at: incidentAt(10), state: StateCrashed, lastError: "oom-kill"},
		{at: incidentAt(20), state: StateRunning},
	}
	section := transitionSection(history, incidentAt(5), incidentAt(15))
	if len(section.Rows) != 1 || section.Rows[0][1] != StateCrashed {
		t.Fatalf("rows = %v, want only the crash inside the window", section.Rows)
	}

	intents := []lifecycleIntent{finishedIntent(intentKill, 1, 1), finishedIntent(intentStart, 30, 30)}
	intents[0].outcome, intents[0].errorText = intentFailed, "host unreachable"
	actions := intentSection(intents, nil, incidentAt(0), incidentAt(15))
	if len(actions.Rows) != 1 || actions.Rows[0][1] != intentKill || actions.Rows[0][4] != "failed: host unreachable" {
		t.Fatalf("lifecycle rows = %v", actions.Rows)
	}

	events := eventSection([]vmEvent{{at: incidentAt(3), kind: qemuEventShutdown, reason: shutdownReasonHostSignal}}, nil, incidentAt(0), incidentAt(15))
	if len(events.Rows) != 1 || events.Rows[0][2] != shutdownReasonHostSignal {
		t.Fatalf("event rows = %v", events.Rows)
	}
}

func TestIncidentFactsLeaveOutSecrets(t *testing.T) {
	vm := incidentVM(StateCrashed, "")
	vm.GuestReportedIPs = []string{"10.0.0.5"}
	vm.CloudInit = &CloudInit{HasPassword: true}
	facts := vmIncidentFacts(vm)
	for key, value := range facts {
		if value == "10.0.0.5" {
			t.Fatalf("fact %q carries a guest address", key)
		}
	}
	if _, ok := facts["cloudInit"]; ok {
		t.Fatal("facts must not carry cloud-init")
	}
}

func TestIncidentBoundText(t *testing.T) {
	got := boundText("  <b>line\none\x1b[31m</b>  " + strings.Repeat("x", 500))
	if strings.ContainsAny(got, "\n\x1b") {
		t.Fatalf("control characters survived: %q", got)
	}
	if n := len([]rune(got)); n != maxIntentText {
		t.Fatalf("length = %d, want %d", n, maxIntentText)
	}
}

func TestIncidentTailLines(t *testing.T) {
	lines := make([]string, 250)
	for i := range lines {
		lines[i] = time.Duration(i).String()
	}
	got := tailLines(lines, incidentLogTail)
	if len(got) != incidentLogTail || got[0] != lines[50] {
		t.Fatalf("tail kept %d lines starting at %q", len(got), got[0])
	}
}
