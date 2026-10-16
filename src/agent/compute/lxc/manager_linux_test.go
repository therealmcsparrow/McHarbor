// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// libcontainer re-executes this test binary as the container's init, so the
	// e2e test below needs the same first-statement branch the agent's main has.
	if IsInitInvocation(os.Args) {
		RunInit()
	}
	os.Exit(m.Run())
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(nil)
	m.root = t.TempDir()
	m.stageBase = filepath.Join(t.TempDir(), "stage")
	previous := RuntimeRoot()
	SetRuntimeRoot(t.TempDir())
	t.Cleanup(func() { SetRuntimeRoot(previous) })
	return m
}

// neverStarted lays down exactly what libcontainer.Create leaves in runc v1.5.1:
// the state directory and no state.json.
func neverStarted(t *testing.T, m *Manager, ctID string) string {
	t.Helper()
	dir := m.stateDir(ctID)
	if err := os.Mkdir(dir, 0o711); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStatusOfANeverStartedContainerIsStopped(t *testing.T) {
	m := testManager(t)
	neverStarted(t, m, "ct-created")

	state, err := m.Status(context.Background(), "ct-created")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if state.State != StateStopped {
		t.Fatalf("state = %q, want %q", state.State, StateStopped)
	}
}

func TestLiveOperationsOnANeverStartedContainerSayNotRunning(t *testing.T) {
	m := testManager(t)
	neverStarted(t, m, "ct-created")
	ctx := context.Background()

	if err := m.Freeze(ctx, "ct-created"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Freeze = %v, want ErrNotRunning", err)
	}
	if _, err := m.OpenExec(ctx, "ct-created", ExecOptions{Argv: []string{"/bin/true"}}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("OpenExec = %v, want ErrNotRunning", err)
	}
	if _, err := m.AttachConsole("ct-created"); !errors.Is(err, ErrConsoleUnavailable) {
		t.Fatalf("AttachConsole = %v, want ErrConsoleUnavailable", err)
	}
	if _, err := m.Sample(ctx, "ct-created"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Sample = %v, want ErrNotRunning", err)
	}
	if err := m.Stop(ctx, "ct-created"); err != nil {
		t.Fatalf("Stop of a never-started container = %v, want nil", err)
	}
	if _, err := m.Shutdown(ctx, "ct-created", InitOpenRC, 0); err != nil {
		t.Fatalf("Shutdown of a never-started container = %v, want nil", err)
	}
	spec := Spec{ID: "ct-created", Limits: Limits{PidsMax: 64}}
	applied, err := m.UpdateLimits(ctx, spec)
	if err != nil || applied.PidsMax != 64 {
		t.Fatalf("UpdateLimits = %+v, %v; want the limits kept for the next start", applied, err)
	}
}

func TestClearLeftoverReclaimsANeverStartedStateDirectory(t *testing.T) {
	m := testManager(t)
	dir := neverStarted(t, m, "ct-created")

	if err := m.clearLeftover("ct-created"); err != nil {
		t.Fatalf("clearLeftover: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the never-started state directory survived, so a second Create would hit ErrExist: %v", err)
	}
}

func TestDeleteRemovesANeverStartedStateDirectory(t *testing.T) {
	m := testManager(t)
	dir := neverStarted(t, m, "ct-created")

	if err := m.Delete(context.Background(), Spec{ID: "ct-created", Name: "created"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the never-started state directory survived the delete as an orphan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(RuntimeRoot(), "ct-created")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the runtime directory survived the delete: %v", err)
	}
}

func TestIsInitInvocationMatchesLibcontainersReexec(t *testing.T) {
	t.Setenv(initPipeEnv, "3")
	if !IsInitInvocation([]string{"/usr/local/bin/mcharbor-agent", "init"}) {
		t.Fatal("libcontainer's `<exe> init` re-exec was not recognised; the container's init would boot the agent")
	}
	if IsInitInvocation([]string{"/usr/local/bin/mcharbor-agent", "container-init"}) {
		t.Fatal("an argv libcontainer never uses was taken for a container init")
	}

	t.Setenv(initPipeEnv, "")
	if IsInitInvocation([]string{"/usr/local/bin/mcharbor-agent", "init"}) {
		t.Fatal("`init` without libcontainer's init pipe was taken for a container init")
	}
}
