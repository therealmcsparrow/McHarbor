// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// copyRootfs gives a test its own copy of the e2e rootfs to change.
func copyRootfs(t *testing.T, source string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "containers", "ct", "rootfs")
	must(t, os.MkdirAll(target, 0o755))
	if out, err := exec.Command("cp", "-a", source+"/.", target).CombinedOutput(); err != nil {
		t.Fatalf("copy rootfs: %v: %s", err, out)
	}
	return target
}

// e2eBusyboxRootfs is the e2e rootfs when it is a busybox-init image, the
// alpine minirootfs this test is about.
func e2eBusyboxRootfs(t *testing.T) (*Manager, string) {
	t.Helper()
	m, rootfs := e2eManager(t)
	if !DetectInitAt(rootfs).BusyboxInit {
		t.Skipf("%s is not a busybox-init rootfs", rootfs)
	}
	return m, rootfs
}

// readFor collects what r produces for d; the console never ends on its own.
func readFor(r interface{ Read([]byte) (int, error) }, d time.Duration) string {
	out := make(chan string, 1)
	go func() {
		var got strings.Builder
		buf := make([]byte, 4096)
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			n, err := r.Read(buf)
			got.Write(buf[:n])
			if err != nil {
				break
			}
		}
		out <- got.String()
	}()
	select {
	case s := <-out:
		return s
	case <-time.After(d + 2*time.Second):
		return ""
	}
}

func TestE2EBusyboxConsoleReachesAShell(t *testing.T) {
	for _, privileged := range []bool{true, false} {
		name := "unprivileged"
		if privileged {
			name = "privileged"
		}
		t.Run(name, func(t *testing.T) { busyboxConsole(t, privileged) })
	}
}

func busyboxConsole(t *testing.T, privileged bool) {
	m, source := e2eBusyboxRootfs(t)
	ctx := context.Background()
	stock := copyRootfs(t, source)
	done, err := PrepareConsoleInit(stock, IDShift{})
	if err != nil || done == InittabUntouched {
		t.Fatalf("PrepareConsoleInit on the minirootfs = %q, %v", done, err)
	}

	spec := Spec{
		ID:         "e2ect00000000000000b1",
		Name:       "e2e-busybox",
		InitSystem: InitOpenRC,
		Privileged: privileged,
		Limits:     Limits{PidsMax: 64, MemoryMaxBytes: 256 << 20, MemoryHighBytes: 241591905},
	}
	spec.RootfsPath = stock
	if !privileged {
		spec.ID = "e2ect00000000000000b2"
		spec.UIDMapStart, spec.UIDMapCount = e2eUIDBase, e2eUIDCount
		spec.GIDMapStart, spec.GIDMapCount = e2eUIDBase, e2eUIDCount
		spec.RootfsPath = privatePoolRootfs(t, stock, spec.ID)
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
	waitForGuestInit(t, pid, "init")

	viewer, err := m.AttachConsole(spec.ID)
	if err != nil {
		t.Fatalf("AttachConsole: %v", err)
	}
	defer errlog.Close(viewer)
	if _, err := viewer.Write([]byte("echo con-$((6*7)); tty\n")); err != nil {
		t.Fatal(err)
	}
	answered := e2eRead(t, viewer, "con-42")
	boot := answered + readFor(viewer, time.Second)
	if strings.Contains(boot, "can't open /dev/tty") {
		t.Fatalf("busybox init still respawns gettys on virtual terminals: %q", boot)
	}
	t.Logf("console: %q", boot)

	// A Ctrl-C typed on the console must not reach PID 1: busybox init maps
	// SIGINT to its ctrlaltdel action, which the minirootfs points at reboot.
	if _, err := viewer.Write([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if state := waitForState(t, m, spec.ID, StateRunning); state.InitPID != pid {
		t.Fatalf("Ctrl-C on the console restarted init: pid %d -> %d", pid, state.InitPID)
	}
	if _, err := viewer.Write([]byte("echo after-$((7*7))\n")); err != nil {
		t.Fatal(err)
	}
	e2eRead(t, viewer, "after-49")

	// An exec terminal opened without a size starts at 80x24.
	shell, err := m.OpenExec(ctx, spec.ID, ExecOptions{TTY: true})
	if err != nil {
		t.Fatalf("tty exec: %v", err)
	}
	if _, err := shell.Write([]byte("stty size\n")); err != nil {
		t.Fatal(err)
	}
	e2eRead(t, shell, "24 80")
	errlog.Close(shell)

	stats, err := m.Sample(ctx, spec.ID)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if stats.MemoryHighBytes != 241591905 {
		// The kernel keeps memory.high in whole pages; the product reports the
		// value it set, not the rounded one.
		t.Fatalf("memoryHighBytes = %d, want the configured 241591905", stats.MemoryHighBytes)
	}
	if stats.MemoryBytes == 0 || stats.SwapBytes == stats.MemoryBytes {
		t.Fatalf("swap %d equals memory %d with swap max 0", stats.SwapBytes, stats.MemoryBytes)
	}
	if stats.RootfsUsedBytes < 1<<20 {
		t.Fatalf("rootfsUsedBytes = %d for an alpine rootfs", stats.RootfsUsedBytes)
	}
	t.Logf("stats: memory %d, high %d, swap %d, rootfs %d",
		stats.MemoryBytes, stats.MemoryHighBytes, stats.SwapBytes, stats.RootfsUsedBytes)
}

// dirCopySnapshot is the dir pool's snapshot as the agent takes it: a fresh
// `cp -a` of the rootfs beside it, removed on release.
func dirCopySnapshot(t *testing.T, rootfs string) func(context.Context) (string, func(), error) {
	return func(ctx context.Context) (string, func(), error) {
		copyPath := rootfs + ".snap.mcharbor-ctbk-e2e"
		if out, err := exec.CommandContext(ctx, "cp", "-a", rootfs+"/.", copyPath).CombinedOutput(); err != nil {
			t.Fatalf("dir copy: %v: %s", err, out)
		}
		return copyPath, func() { errlog.Log(os.RemoveAll(copyPath)) }, nil
	}
}

func TestE2EDirCopyBackupOfARunningContainerIsNeverIncremental(t *testing.T) {
	requireGNUTar(t)
	m, source := e2eManager(t)
	ctx := context.Background()
	spec := Spec{
		ID:         "e2ect00000000000000c1",
		Name:       "e2e-dircopy",
		InitSystem: InitNone,
		InitArgv:   []string{"/bin/sleep", "300"},
		Privileged: true,
		Limits:     Limits{PidsMax: 64},
		RootfsPath: copyRootfs(t, source),
	}
	t.Cleanup(func() {
		if err := m.Delete(context.Background(), spec); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})
	if err := m.Create(ctx, spec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := m.Start(ctx, spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForState(t, m, spec.ID, StateRunning)

	run := func(runID, kind string) BackupResult {
		result, err := m.Backup(ctx, BackupRequest{
			CTID: spec.ID, RunID: runID, Kind: kind, Manifest: []byte("{}"),
			RootfsPath: spec.RootfsPath, ListingName: "mcharbor-ct-e2e-dircopy",
			ScratchDir:     filepath.Join(t.TempDir(), "scratch"),
			Snapshot:       dirCopySnapshot(t, spec.RootfsPath),
			SnapshotIsCopy: true,
		})
		if err != nil {
			t.Fatalf("Backup %s: %v", runID, err)
		}
		return result
	}

	full := run("run1", "full")
	if full.Consistency != ConsistencyLiveCopy || full.Kind != "full" {
		t.Fatalf("the first backup = %+v, want a live-copy full", full)
	}
	must(t, os.WriteFile(filepath.Join(spec.RootfsPath, "root", "second.txt"), []byte("second"), 0o644))
	next := run("run2", "incremental")
	if next.Kind != "full" || !next.FellBackToFull || next.FallbackReason != reasonSourceIsCopy {
		t.Fatalf("an incremental of a running dir-pool container = %+v, want a full with the copy reason", next)
	}
	if state := waitForState(t, m, spec.ID, StateRunning); state.InitPID == 0 {
		t.Fatal("the container stopped during a live copy")
	}

	restored, err := restoreChain(t, next.RootfsArchive)
	if err != nil {
		t.Fatalf("the full taken instead does not restore: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(restored, "root", "second.txt"))
	if err != nil || string(body) != "second" {
		t.Fatalf("the full taken instead is missing the second file: %q, %v", body, err)
	}
	for _, path := range []string{full.RootfsArchive, full.ManifestFile, next.RootfsArchive, next.ManifestFile} {
		errlog.Log(os.Remove(path))
	}
}
