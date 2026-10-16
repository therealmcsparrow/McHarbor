// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// poolRootfsAt copies the e2e rootfs under a 0700 pool with its owners shifted
// to base, the layout a McHarbor-created unprivileged container has.
func poolRootfsAt(t *testing.T, source, ctID string, base int) string {
	t.Helper()
	pool := filepath.Join(t.TempDir(), "pool")
	rootfs := filepath.Join(pool, "containers", ctID, "rootfs")
	must(t, os.MkdirAll(rootfs, 0o755))
	must(t, os.Chmod(pool, 0o700))
	if out, err := exec.Command("cp", "-a", source+"/.", rootfs).CombinedOutput(); err != nil {
		t.Fatalf("copy rootfs: %v: %s", err, out)
	}
	must(t, filepath.WalkDir(rootfs, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		return os.Lchown(path, int(stat.Uid)+base, int(stat.Gid)+base)
	}))
	return rootfs
}

// guestRun runs a shell script in the container and returns what it printed
// and its exit code.
func guestRun(t *testing.T, m *Manager, ctID, script string) (string, int) {
	t.Helper()
	session, err := m.OpenExec(context.Background(), ctID, ExecOptions{Argv: []string{"/bin/sh", "-c", script}})
	if err != nil {
		t.Fatalf("OpenExec: %v", err)
	}
	defer errlog.Close(session)
	out := make(chan string, 1)
	go func() {
		body := errlog.ReadAll(session)
		out <- string(body)
	}()
	select {
	case <-session.Exited():
	case <-time.After(20 * time.Second):
		t.Fatalf("%q never exited", script)
	}
	select {
	case text := <-out:
		return text, session.ExitCode()
	case <-time.After(5 * time.Second):
		return "", session.ExitCode()
	}
}

func e2eSpec(id, name string, base int64, rootfs string) Spec {
	return Spec{
		ID: id, Name: name, InitSystem: InitNone, InitArgv: []string{"/bin/sleep", "600"},
		UIDMapStart: base, UIDMapCount: 65536, GIDMapStart: base, GIDMapCount: 65536,
		Limits: Limits{PidsMax: 64}, RootfsPath: rootfs,
	}
}

func startE2E(t *testing.T, m *Manager, spec Spec) {
	t.Helper()
	t.Cleanup(func() {
		if err := m.Delete(context.Background(), spec); err != nil {
			t.Errorf("cleanup Delete %s: %v", spec.ID, err)
		}
	})
	if err := m.Create(context.Background(), spec); err != nil {
		t.Fatalf("Create %s: %v", spec.ID, err)
	}
	if _, err := m.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start %s: %v", spec.ID, err)
	}
	waitForState(t, m, spec.ID, StateRunning)
}

func stopE2E(t *testing.T, m *Manager, id string) {
	t.Helper()
	if err := m.Stop(context.Background(), id); err != nil {
		t.Fatalf("Stop %s: %v", id, err)
	}
	waitForState(t, m, id, StateStopped)
}

// TestE2ERestoreIntoAnotherIDRange is the seventh live pass in miniature: a
// full and an incremental of container A (range 1000000), restored into
// container B (range 1065536), which then has to boot with root owning its
// own files.
func TestE2ERestoreIntoAnotherIDRange(t *testing.T) {
	requireGNUTar(t)
	m, source := e2eManager(t)
	ctx := context.Background()
	const srcRange, dstRange = 1000000, 1065536

	a := e2eSpec("e2ect00000000000000j1", "e2e-range-a", srcRange, poolRootfsAt(t, source, "e2ect00000000000000j1", srcRange))
	startE2E(t, m, a)
	out, code := guestRun(t, m, a.ID, `set -e
echo marker > /root/e2e-marker.txt
mkdir -p /home/e2e && echo mine > /home/e2e/owned-by-1000.txt && chown -R 1000:1000 /home/e2e
cp /bin/busybox /root/suid-bb && chmod 4755 /root/suid-bb
ln -sf e2e-marker.txt /root/marker-link
chmod 700 /root
echo ok-a`)
	if code != 0 || !strings.Contains(out, "ok-a") {
		t.Fatalf("seeding container A: %d %q", code, out)
	}

	scratch := filepath.Join(t.TempDir(), "scratch")
	backup := func(runID, kind string) BackupResult {
		result, err := m.Backup(ctx, BackupRequest{
			CTID: a.ID, RunID: runID, Kind: kind, Manifest: []byte("{}"),
			RootfsPath: a.RootfsPath, ListingName: "mcharbor-ct-e2e-range", ScratchDir: scratch,
		})
		if err != nil {
			t.Fatalf("Backup %s: %v", runID, err)
		}
		return result
	}
	full := backup("run1", "full")
	time.Sleep(1100 * time.Millisecond)
	if out, code := guestRun(t, m, a.ID, "echo second > /root/second.txt && rm /etc/motd; echo ok"); code != 0 {
		t.Fatalf("changing container A: %d %q", code, out)
	}
	incremental := backup("run2", "incremental")
	if incremental.Kind != "incremental" {
		t.Fatalf("the second backup = %+v, want an incremental", incremental)
	}

	// A is left running, as it was on the live host: its range stays taken, so
	// the restore target genuinely has another one.
	bRootfs := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(a.RootfsPath))), "containers",
		"e2ect00000000000000j2", "rootfs")
	src := IDMap{UIDStart: srcRange, UIDCount: 65536, GIDStart: srcRange, GIDCount: 65536}
	dst := IDMap{UIDStart: dstRange, UIDCount: 65536, GIDStart: dstRange, GIDCount: 65536}
	result, err := m.RestoreRootfs(ctx, RestoreRequest{
		CTID: "e2ect00000000000000j2", RunID: "restore1", TargetPath: bRootfs,
		Links: []string{full.RootfsArchive, incremental.RootfsArchive}, ScratchDir: scratch,
		SourceIDMap: &src, TargetIDMap: &dst,
	})
	if err != nil {
		t.Fatalf("RestoreRootfs into B: %v", err)
	}
	t.Logf("restore: %+v", result)

	hostOwners := map[string][2]uint32{
		"root/e2e-marker.txt":        {dstRange, dstRange},
		"root/second.txt":            {dstRange, dstRange},
		"home/e2e/owned-by-1000.txt": {dstRange + 1000, dstRange + 1000},
		"root/marker-link":           {dstRange, dstRange},
		".":                          {dstRange, dstRange},
	}
	for rel, want := range hostOwners {
		uid, gid, _ := ownerMode(t, filepath.Join(bRootfs, rel))
		if uid != want[0] || gid != want[1] {
			t.Errorf("host owner of B's %s = %d:%d, want %d:%d", rel, uid, gid, want[0], want[1])
		}
	}
	if _, _, mode := ownerMode(t, filepath.Join(bRootfs, "root", "suid-bb")); mode != 0o4755 {
		t.Errorf("suid-bb lost its setuid bit: %04o", mode)
	}
	if link := errlog.Value(os.Readlink(filepath.Join(bRootfs, "root", "marker-link"))); link != "e2e-marker.txt" {
		t.Errorf("the symlink reads %q", link)
	}
	if _, err := os.Lstat(filepath.Join(bRootfs, "etc", "motd")); err == nil {
		t.Error("the incremental's deletion was not applied")
	}

	b := e2eSpec("e2ect00000000000000j2", "e2e-range-b", dstRange, bRootfs)
	startE2E(t, m, b)
	out, code = guestRun(t, m, b.ID, `cat /root/e2e-marker.txt /root/second.txt
stat -c 'owner=%u:%g' /home/e2e/owned-by-1000.txt /root
stat -c 'mode=%a' /root/suid-bb
echo written > /etc/e2e-write && cat /etc/e2e-write
cat /root/marker-link`)
	t.Logf("guest B: %q", out)
	for _, want := range []string{"marker\nsecond\n", "owner=1000:1000", "owner=0:0", "mode=4755", "written\n", "marker\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("guest B output lacks %q (exit %d)", want, code)
		}
	}
	if strings.Contains(out, "65534") || strings.Contains(out, "Permission denied") {
		t.Errorf("guest B still sees the old range's owners: %q", out)
	}
	if code != 0 {
		t.Errorf("guest B's checks exited %d", code)
	}
	stopE2E(t, m, b.ID)
	stopE2E(t, m, a.ID)
	for _, path := range []string{full.RootfsArchive, full.ManifestFile, incremental.RootfsArchive, incremental.ManifestFile} {
		errlog.Log(os.Remove(path))
	}
}
