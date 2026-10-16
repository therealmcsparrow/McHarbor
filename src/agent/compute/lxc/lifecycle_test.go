// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProbePresenceTellsTheThreeShapesApart(t *testing.T) {
	root := t.TempDir()

	absent, err := probePresence(filepath.Join(root, "ct-absent"))
	if err != nil || absent != presenceAbsent {
		t.Fatalf("no directory: got %v, %v; want absent", absent, err)
	}

	unstarted := filepath.Join(root, "ct-created")
	if err := os.Mkdir(unstarted, 0o711); err != nil {
		t.Fatal(err)
	}
	got, err := probePresence(unstarted)
	if err != nil || got != presenceUnstarted {
		t.Fatalf("directory without state.json (what libcontainer.Create leaves): got %v, %v; want unstarted", got, err)
	}

	if err := os.WriteFile(filepath.Join(unstarted, stateFileName), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = probePresence(unstarted)
	if err != nil || got != presenceLoaded {
		t.Fatalf("directory with state.json: got %v, %v; want loaded", got, err)
	}
}

func TestCreateThenStartFreesTheNeverStartedDirectory(t *testing.T) {
	// Bug E: Create leaves a state directory with no state.json, and the next
	// start or create must reclaim it rather than report the container missing
	// or fail on ErrExist.
	action, err := planLeftover(presenceUnstarted, "")
	if err != nil {
		t.Fatalf("a never-started container was refused: %v", err)
	}
	if action != leftoverRemoveDir {
		t.Fatalf("action = %v, want leftoverRemoveDir", action)
	}
}

func TestPlanLeftover(t *testing.T) {
	cases := []struct {
		name     string
		presence presence
		state    string
		want     leftoverAction
		wantErr  error
	}{
		{"nothing there", presenceAbsent, "", leftoverNone, nil},
		{"stopped after a run", presenceLoaded, StateStopped, leftoverDestroy, nil},
		{"init parked on the exec fifo", presenceLoaded, StateDefined, leftoverDestroy, nil},
		{"running", presenceLoaded, StateRunning, leftoverNone, ErrAlreadyRunning},
		{"frozen", presenceLoaded, StateFrozen, leftoverNone, ErrAlreadyRunning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := planLeftover(c.presence, c.state)
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("error = %v, want %v", err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("action = %v, want %v", got, c.want)
			}
		})
	}

	if _, err := planLeftover(presenceLoaded, StateUnknown); err == nil {
		t.Fatal("an unreadable container state was cleared rather than refused")
	}
}

func TestUnloadedStateReportsANeverStartedContainerAsStopped(t *testing.T) {
	for _, p := range []presence{presenceAbsent, presenceUnstarted} {
		state, ok := unloadedState(p)
		if !ok || state != StateStopped {
			t.Fatalf("presence %v: got %q, %v; want stopped", p, state, ok)
		}
	}
	if _, ok := unloadedState(presenceLoaded); ok {
		t.Fatal("a loadable container was answered without loading it")
	}
}

func TestReportedInitPIDIsZeroUnlessTheContainerIsLive(t *testing.T) {
	for _, state := range []string{StateRunning, StateFrozen} {
		if got := reportedInitPID(state, 4242); got != 4242 {
			t.Fatalf("%s: pid = %d, want 4242", state, got)
		}
	}
	for _, state := range []string{StateStopped, StateDefined, StateUnknown, StateCrashed} {
		if got := reportedInitPID(state, 4242); got != 0 {
			t.Fatalf("%s: pid = %d, want 0 — the old init's pid outlives it in state.json", state, got)
		}
	}
}
