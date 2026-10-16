// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/therealmcsparrow/mcharbor-agent/compute/ctsnap"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

const e2eRootfsEnv = "MCHARBOR_LXC_E2E_ROOTFS"

// e2eManager runs the real libcontainer lifecycle with the cgroupfs driver under
// a throwaway parent, since the privileged build container has no systemd.
func e2eManager(t *testing.T) (*Manager, string) {
	t.Helper()
	rootfs := os.Getenv(e2eRootfsEnv)
	if rootfs == "" {
		t.Skipf("set %s to an unpacked rootfs to run the real lifecycle", e2eRootfsEnv)
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	m := testManager(t)
	// Under /run like production: testManager's temporary base sits below the
	// 0700 test directory, which the mapped root could not search either.
	m.stageBase = filepath.Join("/run", "mcharbor-e2e-"+strconv.Itoa(os.Getpid()))
	t.Cleanup(func() {
		for _, dir := range []string{filepath.Join(m.stageBase, "ct"), m.stageBase} {
			if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("staging directory survived: %v", err)
			}
		}
	})
	m.configFor = func(spec Spec, paths Paths) (*configs.Config, error) {
		config, err := BuildConfig(spec, paths)
		if err != nil {
			return nil, err
		}
		config.Cgroups.Systemd = false
		config.Cgroups.Parent = "mcharbor-e2e"
		return config, nil
	}
	return m, rootfs
}

func waitForState(t *testing.T, m *Manager, ctID, want string) State {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, err := m.Status(context.Background(), ctID)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if state.State == want {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %q after 10s, want %q", state.State, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitForGuestInit polls until pid has exec'd the guest's init. Exec returns once
// runc's init stage has written the exec fifo, a moment before its execve.
func waitForGuestInit(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
		if err == nil && strings.Contains(string(cmdline), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("init never became the guest's %s (still parked on the exec fifo?): %q, %v", want, cmdline, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestE2ECreateThenStartRunsInit(t *testing.T) {
	m, rootfs := e2eManager(t)
	ctx := context.Background()
	spec := Spec{
		ID:         "e2ect0000000000000001",
		Name:       "e2e",
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
	if _, err := os.Stat(filepath.Join(m.stateDir(spec.ID), stateFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected runc v1.5.1 Create to leave no state.json, got %v", err)
	}
	waitForState(t, m, spec.ID, StateStopped)

	if err := m.Create(ctx, spec); err != nil {
		t.Fatalf("second Create over a never-started leftover: %v", err)
	}

	pid, err := m.Start(ctx, spec)
	if err != nil {
		t.Fatalf("Start after Create: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("pid = %d", pid)
	}
	running := waitForState(t, m, spec.ID, StateRunning)
	if running.InitPID != pid {
		t.Fatalf("status pid = %d, start pid = %d", running.InitPID, pid)
	}
	waitForGuestInit(t, pid, "sleep")

	if _, err := m.Start(ctx, spec); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Start of a running container = %v, want ErrAlreadyRunning", err)
	}
	if err := m.Create(ctx, spec); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Create over a running container = %v, want ErrAlreadyRunning", err)
	}

	if err := m.Freeze(ctx, spec.ID); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	waitForState(t, m, spec.ID, StateFrozen)
	if err := m.Unfreeze(ctx, spec.ID); err != nil {
		t.Fatalf("Unfreeze: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)

	runTrue(t, m, spec.ID)

	if err := m.Stop(ctx, spec.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitForState(t, m, spec.ID, StateStopped)
	if err := m.Stop(ctx, spec.ID); err != nil {
		t.Fatalf("Stop of a container that already exited: %v", err)
	}

	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("restart after Stop: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)

	if err := m.Delete(ctx, spec); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(m.stateDir(spec.ID)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("state directory survived Delete: %v", err)
	}
}

const (
	e2eUIDBase  = 100000
	e2eUIDCount = 65536
)

// privatePoolRootfs copies the e2e rootfs under a 0700 pool directory with its
// ownership shifted into the container's range, which is the production layout
// bug F was found on: a pool target only host root can traverse.
func privatePoolRootfs(t *testing.T, source, ctID string) string {
	t.Helper()
	pool := filepath.Join(t.TempDir(), "pool")
	rootfs := filepath.Join(pool, "containers", ctID, "rootfs")
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pool, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-a", source+"/.", rootfs).CombinedOutput(); err != nil {
		t.Fatalf("copy rootfs: %v: %s", err, out)
	}
	err := filepath.WalkDir(rootfs, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		return os.Lchown(path, int(stat.Uid)+e2eUIDBase, int(stat.Gid)+e2eUIDBase)
	})
	if err != nil {
		t.Fatalf("shift rootfs ownership: %v", err)
	}
	return rootfs
}

func TestE2EUnprivilegedStartUnderAPrivatePoolDirectory(t *testing.T) {
	m, source := e2eManager(t)
	ctx := context.Background()
	spec := Spec{
		ID:          "e2ect0000000000000002",
		Name:        "e2e-unpriv",
		InitSystem:  InitNone,
		InitArgv:    []string{"/bin/sleep", "300"},
		UIDMapStart: e2eUIDBase, UIDMapCount: e2eUIDCount,
		GIDMapStart: e2eUIDBase, GIDMapCount: e2eUIDCount,
		Limits: Limits{PidsMax: 64},
	}
	spec.RootfsPath = privatePoolRootfs(t, source, spec.ID)
	t.Cleanup(func() {
		if err := m.Delete(context.Background(), spec); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	if err := m.Create(ctx, spec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	pid, err := m.Start(ctx, spec)
	if err != nil {
		t.Fatalf("Start of an unprivileged container under a 0700 pool directory: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)
	waitForGuestInit(t, pid, "sleep")
	layout, err := stageLayoutFor(m.stageBase, spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(mountsAt(t, layout.Target)); n != 1 {
		t.Fatalf("mounts at the staging target of a running container = %d, want 1", n)
	}
	// The production reader has to see the same table whichever thread runc left
	// in the container's mount namespace, the main thread included.
	hostTable, err := ctsnap.Mounts()
	if err != nil {
		t.Fatalf("ctsnap.Mounts after an unprivileged start: %v", err)
	}
	staged := 0
	for _, entry := range hostTable {
		if entry.MountPoint == layout.Target {
			staged++
		}
	}
	if staged != 1 {
		t.Fatalf("ctsnap.Mounts shows %d mounts at the staging target of a running container, want 1", staged)
	}

	runTrue(t, m, spec.ID)
	shell, err := m.OpenExec(ctx, spec.ID, ExecOptions{TTY: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("tty exec in an unprivileged container: %v", err)
	}
	if _, err := shell.Write([]byte("echo uid-$(id -u)-end\n")); err != nil {
		t.Fatal(err)
	}
	e2eRead(t, shell, "uid-0-end")
	errlog.Close(shell)
	viewer, err := m.AttachConsole(spec.ID)
	if err != nil {
		t.Fatalf("console of an unprivileged container: %v", err)
	}
	errlog.Close(viewer)

	if err := m.SweepStaging(ctx); err != nil {
		t.Fatalf("SweepStaging: %v", err)
	}
	if n := len(mountsAt(t, layout.Target)); n != 1 {
		t.Fatalf("the startup sweep unmounted a running container's rootfs (%d mounts left)", n)
	}

	if err := m.Stop(ctx, spec.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitForState(t, m, spec.ID, StateStopped)
	if n := len(mountsAt(t, layout.Target)); n != 0 {
		t.Fatalf("%d staging mounts survived Stop", n)
	}

	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("restart after Stop: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)

	if _, err := m.Shutdown(ctx, spec.ID, InitNone, time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	waitForState(t, m, spec.ID, StateStopped)
	if n := len(mountsAt(t, layout.Target)); n != 0 {
		t.Fatalf("%d staging mounts survived Shutdown", n)
	}

	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("start after Shutdown: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)

	if err := m.Delete(ctx, spec); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n := len(mountsAt(t, layout.Target)); n != 0 {
		t.Fatalf("%d staging mounts survived Delete", n)
	}
	if _, err := os.Stat(layout.Dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the staging directory survived Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spec.RootfsPath, "etc")); err != nil {
		t.Fatalf("releasing the staging touched the pool's rootfs: %v", err)
	}
}

// runTrue runs /bin/true through a plain exec and checks it exited cleanly.
func runTrue(t *testing.T, m *Manager, ctID string) {
	t.Helper()
	session, err := m.OpenExec(context.Background(), ctID, ExecOptions{Argv: []string{"/bin/true"}})
	if err != nil {
		t.Fatalf("OpenExec: %v", err)
	}
	defer errlog.Close(session)
	select {
	case <-session.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("/bin/true never exited")
	}
	if code := session.ExitCode(); code != 0 {
		t.Fatalf("exec exit code = %d, want 0", code)
	}
}

// e2eRead reads r until the output contains want, failing after 10 seconds.
func e2eRead(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var got strings.Builder
		buf := make([]byte, 4096)
		for !strings.Contains(got.String(), want) {
			n, err := r.Read(buf)
			got.Write(buf[:n])
			if err != nil {
				done <- result{got.String(), err}
				return
			}
		}
		done <- result{got.String(), nil}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("read ended with %v before %q; got %q", res.err, want, res.text)
		}
		return res.text
	case <-time.After(10 * time.Second):
		t.Fatalf("never read %q", want)
		return ""
	}
}

// containerPIDs lists the host pids in a container's cgroup.
func containerPIDs(t *testing.T, m *Manager, ctID string) []int {
	t.Helper()
	container, err := m.load(ctID)
	if err != nil {
		t.Fatal(err)
	}
	pids, err := container.Processes()
	if err != nil {
		t.Fatal(err)
	}
	return pids
}

func waitForPIDCount(t *testing.T, m *Manager, ctID string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		pids := containerPIDs(t, m, ctID)
		if len(pids) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("container holds %d processes %v, want %d", len(pids), pids, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestE2EConsoleExecAndStats(t *testing.T) {
	m, rootfs := e2eManager(t)
	ctx := context.Background()
	spec := Spec{
		ID:         "e2ect0000000000000003",
		Name:       "e2econsole",
		RootfsPath: rootfs,
		InitSystem: InitNone,
		// PID 1 is a shell on the console: it prints a marker, then echoes each
		// line it reads back, so the test can see input reach PID 1.
		InitArgv: []string{"/bin/sh", "-c",
			`echo boot-marker-$((40+2)); while read line; do echo "pid1 got $line"; done; sleep 300`},
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
	pid, err := m.Start(ctx, spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)

	// PID-1 console: boot output, input reaching PID 1, resize.
	viewer, err := m.AttachConsole(spec.ID)
	if err != nil {
		t.Fatalf("AttachConsole: %v", err)
	}
	e2eRead(t, viewer, "boot-marker-42")
	if _, err := viewer.Write([]byte("hello\n")); err != nil {
		t.Fatalf("console write: %v", err)
	}
	e2eRead(t, viewer, "pid1 got hello")
	if err := viewer.Resize(100, 40); err != nil {
		t.Fatalf("console resize: %v", err)
	}
	size, err := m.OpenExec(ctx, spec.ID, ExecOptions{Argv: []string{"/bin/sh", "-c", "stty size </dev/console"}})
	if err != nil {
		t.Fatalf("exec stty: %v", err)
	}
	e2eRead(t, size, "40 100")
	errlog.Close(size)

	// Closing the viewer detaches it; PID 1 lives on and a new viewer replays.
	if err := viewer.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if state := waitForState(t, m, spec.ID, StateRunning); state.InitPID != pid {
		t.Fatalf("PID 1 changed from %d to %d after a viewer closed", pid, state.InitPID)
	}
	again, err := m.AttachConsole(spec.ID)
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	replay := e2eRead(t, again, "pid1 got hello")
	if !strings.Contains(replay, "boot-marker-42") {
		t.Fatalf("reattach replay = %q, want the earlier output", replay)
	}

	// An exec shell with a pty: size, resize, and a job killed on close.
	baseline := len(containerPIDs(t, m, spec.ID))
	shell, err := m.OpenExec(ctx, spec.ID, ExecOptions{TTY: true, Cols: 90, Rows: 30})
	if err != nil {
		t.Fatalf("tty exec: %v", err)
	}
	if _, err := shell.Write([]byte("stty size; echo exec-$((6*7))\n")); err != nil {
		t.Fatal(err)
	}
	e2eRead(t, shell, "exec-42")
	if err := shell.Resize(120, 50); err != nil {
		t.Fatalf("exec resize: %v", err)
	}
	if _, err := shell.Write([]byte("stty size; sleep 1000 & echo job-started\n")); err != nil {
		t.Fatal(err)
	}
	e2eRead(t, shell, "50 120")
	e2eRead(t, shell, "job-started")
	if got := len(containerPIDs(t, m, spec.ID)); got < baseline+2 {
		t.Fatalf("expected the shell and its job in the container, have %d processes over %d", got, baseline)
	}
	if err := shell.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-shell.Exited():
	default:
		t.Fatal("Close returned with the exec shell unreaped")
	}
	waitForPIDCount(t, m, spec.ID, baseline)

	// A plain exec: stdin in, stdout out, killed on close.
	cat, err := m.OpenExec(ctx, spec.ID, ExecOptions{Argv: []string{"/bin/cat"}})
	if err != nil {
		t.Fatalf("pipe exec: %v", err)
	}
	if _, err := cat.Write([]byte("through-the-pipe\n")); err != nil {
		t.Fatal(err)
	}
	e2eRead(t, cat, "through-the-pipe")
	errlog.Close(cat)
	waitForPIDCount(t, m, spec.ID, baseline)

	// Stats: a busy exec makes the CPU share visible.
	burn, err := m.OpenExec(ctx, spec.ID, ExecOptions{Argv: []string{"/bin/sh", "-c", "while :; do :; done"}})
	if err != nil {
		t.Fatalf("burn exec: %v", err)
	}
	stats, err := m.Sample(ctx, spec.ID)
	errlog.Close(burn)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if stats.CPUUsageNSec == 0 || stats.MemoryBytes == 0 || stats.PidsCurrent == 0 {
		t.Fatalf("stats carry zero counters: %+v", stats)
	}
	if stats.PidsMax != 64 {
		t.Fatalf("pids max = %d, want the configured 64", stats.PidsMax)
	}
	if stats.CPUPercent <= 0 || stats.CPUPercent > 100 {
		t.Fatalf("cpu percent = %v with a busy loop running, want (0, 100]", stats.CPUPercent)
	}

	// Stop: the viewer sees the end, no console remains, the pid is cleared.
	if err := m.Stop(ctx, spec.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	stopped := waitForState(t, m, spec.ID, StateStopped)
	if stopped.InitPID != 0 {
		t.Fatalf("a stopped container reports init pid %d", stopped.InitPID)
	}
	buf := make([]byte, 4096)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := again.Read(buf); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the console viewer never saw the container stop")
		}
	}
	if _, err := m.AttachConsole(spec.ID); !errors.Is(err, ErrConsoleUnavailable) {
		t.Fatalf("attach to a stopped container = %v, want ErrConsoleUnavailable", err)
	}
	if _, err := m.Sample(ctx, spec.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Sample of a stopped container = %v, want ErrNotRunning", err)
	}

	// Start again: a fresh console with the new boot's output.
	newPID, err := m.Start(ctx, spec)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if newPID == pid {
		t.Fatal("restart reported the old pid")
	}
	fresh, err := m.AttachConsole(spec.ID)
	if err != nil {
		t.Fatalf("attach after restart: %v", err)
	}
	e2eRead(t, fresh, "boot-marker-42")
	errlog.Close(fresh)
}
