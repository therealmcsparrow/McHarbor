// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

const ctRangeStart = 1000000

func needRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("chown needs root")
	}
}

func poolWithRootfs(t *testing.T, owners map[string]int) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime := t.TempDir()
	lxc.SetRuntimeRoot(runtime)
	t.Cleanup(func() { lxc.SetRuntimeRoot("") })
	for id, owner := range owners {
		rootfs := filepath.Join(root, "containers", id, "rootfs")
		for _, dir := range []string{"root", "tmp"} {
			if err := os.MkdirAll(filepath.Join(rootfs, dir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Lchown(filepath.Join(rootfs, dir), owner, owner); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Lchown(rootfs, owner, owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func ownerOf(t *testing.T, p string) (int, int) {
	t.Helper()
	info, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid)
}

func wantOwner(t *testing.T, p string, uid, gid int) {
	t.Helper()
	if gotUID, gotGID := ownerOf(t, p); gotUID != uid || gotGID != gid {
		t.Fatalf("%s is owned by %d:%d, want %d:%d", p, gotUID, gotGID, uid, gid)
	}
}

func poolWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	if err := poolWriteFile(root, &ComputePoolPayload{Path: rel, Content: []byte(content)}); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestPoolWriteAndMkdirInAnUnprivilegedRootfsBelongToTheContainerRoot(t *testing.T) {
	needRoot(t)
	root := poolWithRootfs(t, map[string]int{"ct1": ctRangeStart})

	poolWrite(t, root, "containers/ct1/rootfs/root/e2e-third.txt", "hello")
	wantOwner(t, filepath.Join(root, "containers/ct1/rootfs/root/e2e-third.txt"), ctRangeStart, ctRangeStart)

	if err := poolMkdir(root, &ComputePoolPayload{Path: "containers/ct1/rootfs/root/made"}); err != nil {
		t.Fatal(err)
	}
	wantOwner(t, filepath.Join(root, "containers/ct1/rootfs/root/made"), ctRangeStart, ctRangeStart)

	poolWrite(t, root, "containers/ct1/rootfs/root/made/inner.conf", "x=1")
	wantOwner(t, filepath.Join(root, "containers/ct1/rootfs/root/made/inner.conf"), ctRangeStart, ctRangeStart)
}

func TestPoolWriteAndMkdirInAPrivilegedRootfsStayHostRoot(t *testing.T) {
	needRoot(t)
	root := poolWithRootfs(t, map[string]int{"priv": 0})
	poolWrite(t, root, "containers/priv/rootfs/root/a.txt", "a")
	wantOwner(t, filepath.Join(root, "containers/priv/rootfs/root/a.txt"), 0, 0)
	if err := poolMkdir(root, &ComputePoolPayload{Path: "containers/priv/rootfs/root/d"}); err != nil {
		t.Fatal(err)
	}
	wantOwner(t, filepath.Join(root, "containers/priv/rootfs/root/d"), 0, 0)
}

func TestPoolOverwriteKeepsAnInRangeOwnerAndRepairsAHostRootOne(t *testing.T) {
	needRoot(t)
	root := poolWithRootfs(t, map[string]int{"ct1": ctRangeStart})
	user := filepath.Join(root, "containers/ct1/rootfs/root/user.txt")
	stray := filepath.Join(root, "containers/ct1/rootfs/root/stray.txt")
	for _, p := range []string{user, stray} {
		if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Lchown(user, ctRangeStart+1000, ctRangeStart+1000); err != nil {
		t.Fatal(err)
	}

	poolWrite(t, root, "containers/ct1/rootfs/root/user.txt", "new")
	wantOwner(t, user, ctRangeStart+1000, ctRangeStart+1000)
	poolWrite(t, root, "containers/ct1/rootfs/root/stray.txt", "new")
	wantOwner(t, stray, ctRangeStart, ctRangeStart)
}

func TestPoolOverwriteSizesTheRangeFromTheContainersConfig(t *testing.T) {
	needRoot(t)
	root := poolWithRootfs(t, map[string]int{"ct1": ctRangeStart})
	paths := lxc.PathsFor("ct1")
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := `{"uid_mappings":[{"container_id":0,"host_id":1000000,"size":1000}],` +
		`"gid_mappings":[{"container_id":0,"host_id":1000000,"size":1000}]}`
	if err := os.WriteFile(paths.ConfigJSON, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "containers/ct1/rootfs/root/beyond.txt")
	if err := os.WriteFile(outside, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Lchown(outside, ctRangeStart+1000, ctRangeStart+1000); err != nil {
		t.Fatal(err)
	}
	poolWrite(t, root, "containers/ct1/rootfs/root/beyond.txt", "new")
	wantOwner(t, outside, ctRangeStart, ctRangeStart)
}

func TestPoolWriteNeverFollowsASymlinkWhenOwning(t *testing.T) {
	needRoot(t)
	root := poolWithRootfs(t, map[string]int{"ct1": ctRangeStart, "ct2": ctRangeStart + 65536})
	outsideDir := t.TempDir()
	victim := filepath.Join(outsideDir, "host-file")
	if err := os.WriteFile(victim, []byte("host"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "containers/ct1/rootfs/root/link")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}

	poolWrite(t, root, "containers/ct1/rootfs/root/link", "guest")
	wantOwner(t, victim, 0, 0)
	if content := errlog.Value(os.ReadFile(victim)); string(content) != "host" {
		t.Fatalf("the symlink's target was rewritten: %q", content)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the write did not replace the link itself: %v %v", info.Mode(), err)
	}
	wantOwner(t, link, ctRangeStart, ctRangeStart)

	dirLink := filepath.Join(root, "containers/ct1/rootfs/root/dirlink")
	if err := os.Symlink(outsideDir, dirLink); err != nil {
		t.Fatal(err)
	}
	if err := poolMkdir(root, &ComputePoolPayload{Path: "containers/ct1/rootfs/root/dirlink"}); !errors.Is(err, storage.ErrExists) {
		t.Fatalf("mkdir over a symlink = %v, want ErrExists", err)
	}
	wantOwner(t, outsideDir, 0, 0)

	if err := os.Symlink("../../ct2/rootfs/tmp", filepath.Join(root, "containers/ct1/rootfs/other")); err != nil {
		t.Fatal(err)
	}
	poolWrite(t, root, "containers/ct1/rootfs/other/landed.txt", "x")
	wantOwner(t, filepath.Join(root, "containers/ct2/rootfs/tmp/landed.txt"), ctRangeStart+65536, ctRangeStart+65536)
}

func TestPoolWriteOutsideAnyRootfsKeepsTheAgentsOwner(t *testing.T) {
	needRoot(t)
	root := poolWithRootfs(t, map[string]int{"ct1": ctRangeStart})
	poolWrite(t, root, "images/seed.cfg", "x")
	wantOwner(t, filepath.Join(root, "images/seed.cfg"), 0, 0)
	poolWrite(t, root, "containers/ct1/notes.txt", "x")
	wantOwner(t, filepath.Join(root, "containers/ct1/notes.txt"), 0, 0)
	if err := poolMkdir(root, &ComputePoolPayload{Path: "containers/ct9"}); err != nil {
		t.Fatal(err)
	}
	wantOwner(t, filepath.Join(root, "containers/ct9"), 0, 0)
}
