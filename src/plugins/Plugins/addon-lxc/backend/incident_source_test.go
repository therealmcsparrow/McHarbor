// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"
	"time"
)

func TestIncidentClassify(t *testing.T) {
	recorded := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)
	unknown := liveObservation{}
	stopped := liveObservation{Known: true, State: StateStopped}
	running := liveObservation{Known: true, State: StateRunning}

	cases := []struct {
		name       string
		ct         Container
		live       liveObservation
		wantSignal string
		wantAt     time.Time
	}{
		{"start refused", Container{State: StateCrashed, LastError: "no delegated cgroup", UpdatedAt: recorded}, unknown, signalStartFailed, recorded},
		{"crash with exit reason", Container{State: StateCrashed, ExitReason: "oom-kill", UpdatedAt: recorded}, stopped, signalCrashed, recorded},
		{"host reports crashed", Container{State: StateRunning, UpdatedAt: recorded}, liveObservation{Known: true, State: StateCrashed}, signalCrashed, now},
		{"stopped behind our back", Container{State: StateRunning, UpdatedAt: recorded}, stopped, signalStoppedUnexpectedly, now},
		{"frozen then stopped", Container{State: StateFrozen, UpdatedAt: recorded}, stopped, signalStoppedUnexpectedly, now},
		{"unreachable host is not a stop", Container{State: StateRunning, UpdatedAt: recorded}, unknown, "", time.Time{}},
		{"deliberately stopped", Container{State: StateStopped, UpdatedAt: recorded}, stopped, "", time.Time{}},
		{"never started", Container{State: StateDefined, UpdatedAt: recorded}, unknown, "", time.Time{}},
		{"running normally", Container{State: StateRunning, UpdatedAt: recorded}, running, "", time.Time{}},
		{"crashed row but started by hand", Container{State: StateCrashed, LastError: "x", UpdatedAt: recorded}, running, "", time.Time{}},
		{"template ignored", Container{State: StateCrashed, IsTemplate: true, UpdatedAt: recorded}, unknown, "", time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			signal, detail, at := classifyIncident(tc.ct, tc.live, now)
			if signal != tc.wantSignal {
				t.Fatalf("signal = %q, want %q", signal, tc.wantSignal)
			}
			if !at.Equal(tc.wantAt) {
				t.Fatalf("at = %v, want %v", at, tc.wantAt)
			}
			if signal != "" && detail == "" {
				t.Fatalf("signal %q carries no detail", signal)
			}
		})
	}
}

func TestIncidentSettled(t *testing.T) {
	cases := []struct {
		name string
		ct   Container
		live liveObservation
		want bool
	}{
		{"stopped through McHarbor", Container{State: StateStopped}, liveObservation{}, true},
		{"defined", Container{State: StateDefined}, liveObservation{}, true},
		{"running again", Container{State: StateRunning}, liveObservation{Known: true, State: StateRunning}, true},
		{"still stopped on host", Container{State: StateRunning}, liveObservation{Known: true, State: StateStopped}, false},
		{"host unreachable", Container{State: StateRunning}, liveObservation{}, false},
		{"still crashed", Container{State: StateCrashed}, liveObservation{Known: true, State: StateStopped}, false},
		{"crashed then started by hand", Container{State: StateCrashed}, liveObservation{Known: true, State: StateRunning}, true},
		{"became a template", Container{State: StateCrashed, IsTemplate: true}, liveObservation{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := incidentSettled(tc.ct, tc.live); got != tc.want {
				t.Fatalf("settled = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestIncidentNeedsLiveCheck(t *testing.T) {
	for state, want := range map[string]bool{
		StateRunning: true, StateFrozen: true, StateCrashed: true,
		StateStopped: false, StateDefined: false,
	} {
		if got := needsLiveCheck(Container{State: state}); got != want {
			t.Errorf("needsLiveCheck(%s) = %t, want %t", state, got, want)
		}
	}
	if needsLiveCheck(Container{State: StateRunning, IsTemplate: true}) {
		t.Error("a template must never cost an agent query")
	}
}

func TestIncidentLifecycleRows(t *testing.T) {
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	started := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	ct := Container{
		State: StateCrashed, LastError: "refused", ExitReason: "oom-kill",
		CreatedAt: created, LastStartedAt: &started, UpdatedAt: updated,
	}

	all := lifecycleRows(ct, time.Time{}, time.Time{})
	if len(all) != 3 {
		t.Fatalf("rows = %d, want 3: %v", len(all), all)
	}
	if all[0][1] != "created" || all[2][1] != "state recorded: crashed" {
		t.Fatalf("rows out of order: %v", all)
	}
	if all[2][2] != "oom-kill; refused" {
		t.Fatalf("detail = %q", all[2][2])
	}

	window := lifecycleRows(ct, started, updated)
	if len(window) != 2 || window[0][1] != "last started" {
		t.Fatalf("windowed rows = %v", window)
	}
}
