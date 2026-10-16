// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// zombieChildren lists this process's children that have exited and not been
// waited on, read from /proc because a zombie is visible nowhere else.
func zombieChildren(t *testing.T) []int {
	t.Helper()
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatalf("read /proc: %v", err)
	}
	var zombies []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "status"))
		if err != nil {
			continue
		}
		var state string
		ppid := -1
		for _, line := range strings.Split(string(body), "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			switch key {
			case "State":
				state = strings.TrimSpace(value)
			case "PPid":
				ppid = errlog.Value(strconv.Atoi(strings.TrimSpace(value)))
			}
		}
		if ppid == self && strings.HasPrefix(state, "Z") {
			zombies = append(zombies, pid)
		}
	}
	return zombies
}

// requireNoZombies fails when any child of the test process is a zombie. The
// check is immediate: a Stop that returned must already have reaped its init.
func requireNoZombies(t *testing.T, when string) {
	t.Helper()
	if zombies := zombieChildren(t); len(zombies) > 0 {
		t.Fatalf("%s: zombie children of the agent: %v", when, zombies)
	}
}

func TestE2EStoppedAndCrashedInitsAreReaped(t *testing.T) {
	m, rootfs := e2eManager(t)
	ctx := context.Background()
	spec := Spec{
		ID:         "e2ect0000000000000009",
		Name:       "e2e-reap",
		RootfsPath: rootfs,
		InitSystem: InitNone,
		InitArgv:   []string{"/bin/sleep", "300"},
		Privileged: true,
		Limits:     Limits{PidsMax: 64},
	}
	t.Cleanup(func() {
		if err := m.Delete(context.Background(), spec); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})
	if err := m.Create(ctx, spec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const cycles = 5
	for i := 0; i < cycles; i++ {
		pid, err := m.Start(ctx, spec)
		if err != nil {
			t.Fatalf("cycle %d Start: %v", i, err)
		}
		waitForGuestInit(t, pid, "sleep")
		if i%2 == 0 {
			if err := m.Stop(ctx, spec.ID); err != nil {
				t.Fatalf("cycle %d Stop: %v", i, err)
			}
		} else {
			// sleep as PID 1 ignores SIGTERM, so this is the escalation path.
			if _, err := m.Shutdown(ctx, spec.ID, InitNone, time.Second); err != nil {
				t.Fatalf("cycle %d Shutdown: %v", i, err)
			}
		}
		requireNoZombies(t, "after stop cycle "+strconv.Itoa(i))
		if state := waitForState(t, m, spec.ID, StateStopped); state.ExitReason != "" {
			t.Fatalf("cycle %d: a requested stop reported exit reason %q", i, state.ExitReason)
		}
	}

	// An init that exits on its own has nobody stopping it: only the reaper can
	// collect it, and its status is a crash.
	crashing := spec
	crashing.InitArgv = []string{"/bin/sh", "-c", "sleep 1; exit 3"}
	if _, err := m.Start(ctx, crashing); err != nil {
		t.Fatalf("Start of a crashing init: %v", err)
	}
	crashed := waitForState(t, m, spec.ID, StateCrashed)
	if crashed.ExitReason != "init exited with status 3" {
		t.Fatalf("exit reason = %q", crashed.ExitReason)
	}
	requireNoZombies(t, "after the init crashed")

	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("Start after a crash: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)
}
