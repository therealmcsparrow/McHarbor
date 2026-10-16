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
	"runtime"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// mountsAt returns the optional-fields column of every mountinfo entry mounted at
// path, one string per stacked mount.
func mountsAt(t *testing.T, path string) []string {
	t.Helper()
	// Not /proc/self: runc's mount-source goroutine setns(2)es a locked thread
	// into a container's mount namespace and never unlocks it, and when that is
	// the main thread Go parks it rather than killing it, so the thread group
	// leader's view is a dead container's namespace.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	raw, err := os.ReadFile("/proc/thread-self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 || fields[4] != path {
			continue
		}
		var optional []string
		for _, field := range fields[6:] {
			if field == "-" {
				break
			}
			optional = append(optional, field)
		}
		out = append(out, strings.Join(optional, " "))
	}
	return out
}

// searchableStageManager gives the manager a staging base whose ancestors the
// test itself owns and can open, since t.TempDir's own directories are 0700.
func searchableStageManager(t *testing.T) *Manager {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("staging needs root to bind-mount")
	}
	m := testManager(t)
	base := t.TempDir()
	for dir := base; dir != os.TempDir() && dir != "/"; dir = filepath.Dir(dir) {
		if err := os.Chmod(dir, 0o711); err != nil {
			t.Fatal(err)
		}
	}
	m.stageBase = filepath.Join(base, "stage")
	return m
}

func TestStageRootfsBindsPrivatelyAndReleaseLeavesTheSourceIntact(t *testing.T) {
	m := searchableStageManager(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "marker"), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A pool on a systemd host sits on a shared mount; a bind of it joins that
	// peer group unless it is made private, so the source is made shared here.
	if err := unix.Mount(source, source, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { errlog.Log(unix.Unmount(source, unix.MNT_DETACH)) })
	if err := unix.Mount("", source, "", unix.MS_SHARED, ""); err != nil {
		t.Fatal(err)
	}
	if shared := mountsAt(t, source); len(shared) != 1 || !strings.Contains(shared[0], "shared:") {
		t.Fatalf("could not make the source shared: %v", shared)
	}
	spec := Spec{ID: "ctstage1", Name: "stage", RootfsPath: source}
	layout, err := stageLayoutFor(m.stageBase, spec.ID)
	if err != nil {
		t.Fatal(err)
	}

	staged, err := m.stageRootfs(spec)
	if err != nil {
		t.Fatalf("stageRootfs: %v", err)
	}
	t.Cleanup(func() { errlog.Log(releaseStage(layout)) })
	if staged != layout.Target {
		t.Fatalf("staged at %q, want %q", staged, layout.Target)
	}
	if data, err := os.ReadFile(filepath.Join(staged, "marker")); err != nil || string(data) != "kept" {
		t.Fatalf("the staged path does not show the rootfs: %q, %v", data, err)
	}
	for _, dir := range layout.Owned {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != stageDirMode {
			t.Fatalf("%s is %v, want %v so the mapped root can search it", dir, info.Mode().Perm(), stageDirMode)
		}
	}
	mounts := mountsAt(t, staged)
	if len(mounts) != 1 {
		t.Fatalf("mounts at the staging target = %d, want 1", len(mounts))
	}
	if strings.Contains(mounts[0], "shared:") || strings.Contains(mounts[0], "master:") {
		t.Fatalf("the staged rootfs propagates (%q); it must be private", mounts[0])
	}

	if _, err := m.stageRootfs(spec); err != nil {
		t.Fatalf("restage: %v", err)
	}
	if n := len(mountsAt(t, staged)); n != 1 {
		t.Fatalf("a second start stacked %d mounts at the staging target, want 1", n)
	}

	if err := m.releaseStaged(spec.ID); err != nil {
		t.Fatalf("releaseStaged: %v", err)
	}
	if n := len(mountsAt(t, staged)); n != 0 {
		t.Fatalf("%d mounts survived the release", n)
	}
	if _, err := os.Stat(layout.Dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the per-container staging directory survived: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(source, "marker")); err != nil || string(data) != "kept" {
		t.Fatalf("the release touched the pool's rootfs: %q, %v", data, err)
	}
	if err := m.releaseStaged(spec.ID); err != nil {
		t.Fatalf("a second release is not a no-op: %v", err)
	}
}

func TestPrivilegedContainersAreHandedTheirPoolPath(t *testing.T) {
	m := searchableStageManager(t)
	source := t.TempDir()
	staged, err := m.stageRootfs(Spec{ID: "ctpriv1", Name: "priv", RootfsPath: source, Privileged: true})
	if err != nil {
		t.Fatalf("stageRootfs: %v", err)
	}
	if staged != source {
		t.Fatalf("privileged rootfs = %q, want the pool path %q", staged, source)
	}
	if _, err := os.Stat(m.stageBase); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a privileged start created staging directories: %v", err)
	}
}

func TestSweepStagingReleasesWhatNoLiveContainerUses(t *testing.T) {
	m := searchableStageManager(t)
	source := t.TempDir()
	spec := Spec{ID: "ctorphan1", Name: "orphan", RootfsPath: source}
	staged, err := m.stageRootfs(spec)
	if err != nil {
		t.Fatalf("stageRootfs: %v", err)
	}
	t.Cleanup(func() { errlog.Log(m.releaseStaged(spec.ID)) })

	if err := m.SweepStaging(context.Background()); err != nil {
		t.Fatalf("SweepStaging: %v", err)
	}
	if n := len(mountsAt(t, staged)); n != 0 {
		t.Fatalf("the sweep left %d mounts of a container with no state", n)
	}
	if _, err := os.Stat(filepath.Dir(staged)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the sweep left the staging directory: %v", err)
	}
}
