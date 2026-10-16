// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package hostmnt

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/hostmnt/hostmnttest"
	"golang.org/x/sys/unix"
)

func init() {
	runtime.LockOSThread()
}

var marker string

func TestMain(m *testing.M) {
	if os.Geteuid() != 0 {
		os.Exit(m.Run())
	}
	var release func()
	var err error
	marker, release, err = hostmnttest.StrandLeader()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	release()
	os.Exit(code)
}

func needsStrandedLeader(t *testing.T) {
	t.Helper()
	if marker == "" {
		t.Skip("needs root to strand the thread-group leader in another mount namespace")
	}
}

func mountsAt(table, path string) int {
	n := 0
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 4 && fields[4] == path {
			n++
		}
	}
	return n
}

func TestProcSelfShowsTheLeadersNamespaceNotTheCallers(t *testing.T) {
	needsStrandedLeader(t)
	raw, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	if mountsAt(string(raw), marker) != 1 {
		t.Fatalf("/proc/self/mountinfo read from a thread in the agent's namespace does not show the leader's marker at %s; the reproduction is not reproducing", marker)
	}
}

func TestReadReturnsTheAgentsTableWhileTheLeaderIsStranded(t *testing.T) {
	needsStrandedLeader(t)
	for _, name := range []string{"mountinfo", "mounts"} {
		raw, err := Read(name)
		if err != nil {
			t.Fatalf("Read(%s): %v", name, err)
		}
		table := string(raw)
		field := 4
		if name == "mounts" {
			field = 1
		}
		for _, line := range strings.Split(table, "\n") {
			fields := strings.Fields(line)
			if len(fields) > field && fields[field] == marker {
				t.Fatalf("Read(%s) returned the stranded leader's mount at %s", name, marker)
			}
		}
		if !strings.Contains(table, " / ") {
			t.Fatalf("Read(%s) has no root mount:\n%s", name, table)
		}
	}
}

func TestAThreadInAnotherNamespaceIsRefused(t *testing.T) {
	needsStrandedLeader(t)
	result := make(chan error, 1)
	go func() {
		// Never unlocked: the thread leaves the agent's namespace and must die
		// with this goroutine rather than go back to the scheduler.
		runtime.LockOSThread()
		if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
			result <- err
			return
		}
		_, err := readLocked("mountinfo")
		result <- err
	}()
	if err := <-result; !errors.Is(err, ErrForeignNamespace) {
		t.Fatalf("readLocked on a thread in another mount namespace = %v, want ErrForeignNamespace", err)
	}
}

func TestReadRefusesAnythingButAMountTable(t *testing.T) {
	if _, err := Read("../environ"); err == nil {
		t.Fatal("Read accepted a name that is not a mount table")
	}
}
