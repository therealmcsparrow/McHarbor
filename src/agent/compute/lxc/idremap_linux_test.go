// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

const (
	srcBase = 1000000
	dstBase = 1065536
)

func ownerMode(t *testing.T, path string) (uint32, uint32, uint32) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	return stat.Uid, stat.Gid, stat.Mode & 0o7777
}

func chownTo(t *testing.T, path string, uid, gid int) {
	t.Helper()
	must(t, os.Lchown(path, uid, gid))
}

// idTree lays down the shapes a re-own has to get right, owned by the range
// at base: root and a user, setuid and setgid, a symlink out of the tree, a
// hard link, and where the filesystem allows, an ACL and a v3 file capability.
// It returns the host file the symlink points at.
func idTree(t *testing.T, root string, base int) (hostFile string, withACL, withCaps bool) {
	t.Helper()
	for _, dir := range []string{"etc", "root", "home/e2e", "bin", "srv/shared"} {
		must(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
	}
	must(t, os.WriteFile(filepath.Join(root, "etc", "marker"), []byte("marker\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "home", "e2e", "owned-by-1000.txt"), []byte("mine\n"), 0o640))
	must(t, os.WriteFile(filepath.Join(root, "root", "suid-bb"), []byte("#!/bin/true\n"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "bin", "sgid-tool"), []byte("x"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "bin", "a"), []byte("hard"), 0o755))
	must(t, os.Link(filepath.Join(root, "bin", "a"), filepath.Join(root, "bin", "b")))
	must(t, os.WriteFile(filepath.Join(root, "bin", "ping"), []byte("ping"), 0o755))

	hostFile = filepath.Join(t.TempDir(), "host-owned")
	must(t, os.WriteFile(hostFile, []byte("host"), 0o600))
	chownTo(t, hostFile, 0, 0)
	must(t, os.Symlink(hostFile, filepath.Join(root, "root", "host-link")))
	must(t, os.Symlink("../etc/marker", filepath.Join(root, "root", "marker-link")))

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, base, base)
	})
	must(t, err)
	chownTo(t, filepath.Join(root, "home", "e2e"), base+1000, base+1000)
	chownTo(t, filepath.Join(root, "home", "e2e", "owned-by-1000.txt"), base+1000, base+1000)
	chownTo(t, filepath.Join(root, "srv", "shared"), base, base+50)
	chownTo(t, filepath.Join(root, "bin", "sgid-tool"), base, base+50)
	// Modes after the chowns, which would otherwise have dropped them.
	must(t, unix.Chmod(filepath.Join(root, "root", "suid-bb"), 0o4755))
	must(t, unix.Chmod(filepath.Join(root, "bin", "sgid-tool"), 0o2755))
	must(t, unix.Chmod(filepath.Join(root, "srv", "shared"), 0o2775))
	must(t, unix.Chmod(filepath.Join(root, "root"), 0o700))

	acl := aclBytes([3]uint32{0x01, 6, 0xFFFFFFFF}, [3]uint32{aclTagUser, 4, uint32(base + 1000)},
		[3]uint32{0x04, 4, 0xFFFFFFFF}, [3]uint32{0x10, 4, 0xFFFFFFFF}, [3]uint32{0x20, 4, 0xFFFFFFFF})
	withACL = unix.Lsetxattr(filepath.Join(root, "etc", "marker"), xattrACLAccess, acl, 0) == nil
	withCaps = unix.Lsetxattr(filepath.Join(root, "bin", "ping"), xattrFileCaps,
		capBytes(vfsCapRevision3, uint32(base)), 0) == nil
	return hostFile, withACL, withCaps
}

func TestReownTreeMovesOwnersAndKeepsModesLinksAndXattrs(t *testing.T) {
	requireRoot(t)
	root := filepath.Join(t.TempDir(), "rootfs")
	hostFile, withACL, withCaps := idTree(t, root, srcBase)

	remap, err := NewIDRemap(unpriv(srcBase), unpriv(dstBase))
	must(t, err)
	if _, err := ReownTree(context.Background(), root, remap); err != nil {
		t.Fatalf("ReownTree: %v", err)
	}

	want := map[string][3]uint32{
		".":                          {dstBase, dstBase, 0o755},
		"etc/marker":                 {dstBase, dstBase, 0o644},
		"root":                       {dstBase, dstBase, 0o700},
		"home/e2e/owned-by-1000.txt": {dstBase + 1000, dstBase + 1000, 0o640},
		"root/suid-bb":               {dstBase, dstBase, 0o4755},
		"bin/sgid-tool":              {dstBase, dstBase + 50, 0o2755},
		"srv/shared":                 {dstBase, dstBase + 50, 0o2775},
		"bin/a":                      {dstBase, dstBase, 0o755},
		"bin/b":                      {dstBase, dstBase, 0o755},
	}
	for rel, w := range want {
		uid, gid, mode := ownerMode(t, filepath.Join(root, rel))
		if uid != w[0] || gid != w[1] || mode != w[2] {
			t.Errorf("%s = %d:%d %04o, want %d:%d %04o", rel, uid, gid, mode, w[0], w[1], w[2])
		}
	}
	for _, link := range []string{"root/host-link", "root/marker-link"} {
		uid, gid, _ := ownerMode(t, filepath.Join(root, link))
		if uid != dstBase || gid != dstBase {
			t.Errorf("symlink %s = %d:%d, want the link itself re-owned", link, uid, gid)
		}
	}
	if uid, gid, mode := ownerMode(t, hostFile); uid != 0 || gid != 0 || mode != 0o600 {
		t.Fatalf("the symlink was followed: its host target is now %d:%d %04o", uid, gid, mode)
	}
	if target := errlog.Value(os.Readlink(filepath.Join(root, "root", "host-link"))); target != hostFile {
		t.Fatalf("the symlink now points at %q", target)
	}

	if withACL {
		got, present, err := lgetxattr(filepath.Join(root, "etc", "marker"), xattrACLAccess)
		if err != nil || !present {
			t.Fatalf("the ACL is gone: %v", err)
		}
		if !bytes.Contains(got, le32(dstBase+1000)) || bytes.Contains(got, le32(srcBase+1000)) {
			t.Fatalf("the ACL's named user was not moved: %x", got)
		}
	} else {
		t.Log("this filesystem holds no POSIX ACLs; the ACL leg was not exercised")
	}
	if withCaps {
		got, present, err := lgetxattr(filepath.Join(root, "bin", "ping"), xattrFileCaps)
		if err != nil || !present {
			t.Fatalf("the file capability did not survive the chown: %v", err)
		}
		if !bytes.Equal(got, capBytes(vfsCapRevision3, dstBase)) {
			t.Fatalf("the file capability's root id was not moved: %x", got)
		}
	} else {
		t.Log("this filesystem holds no file capabilities; the capability leg was not exercised")
	}
}

func le32(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

func TestReownTreeShiftsPrivilegedUpAndUnprivilegedDown(t *testing.T) {
	requireRoot(t)
	up := filepath.Join(t.TempDir(), "up")
	idTree(t, up, 0)
	remap := errlog.Value(NewIDRemap(privileged, unpriv(dstBase)))
	if _, err := ReownTree(context.Background(), up, remap); err != nil {
		t.Fatalf("privileged into unprivileged: %v", err)
	}
	if uid, gid, mode := ownerMode(t, filepath.Join(up, "root", "suid-bb")); uid != dstBase || gid != dstBase || mode != 0o4755 {
		t.Fatalf("shift up: suid-bb = %d:%d %04o", uid, gid, mode)
	}
	if uid, _, _ := ownerMode(t, filepath.Join(up, "home", "e2e", "owned-by-1000.txt")); uid != dstBase+1000 {
		t.Fatalf("shift up: uid 1000 became %d", uid)
	}

	down := filepath.Join(t.TempDir(), "down")
	idTree(t, down, srcBase)
	remap = errlog.Value(NewIDRemap(unpriv(srcBase), privileged))
	if _, err := ReownTree(context.Background(), down, remap); err != nil {
		t.Fatalf("unprivileged into privileged: %v", err)
	}
	if uid, gid, mode := ownerMode(t, filepath.Join(down, "root", "suid-bb")); uid != 0 || gid != 0 || mode != 0o4755 {
		t.Fatalf("shift down: suid-bb = %d:%d %04o", uid, gid, mode)
	}
	if uid, _, _ := ownerMode(t, filepath.Join(down, "home", "e2e", "owned-by-1000.txt")); uid != 1000 {
		t.Fatalf("shift down: uid 1000 became %d", uid)
	}
}

func TestReownTreeRefusesAnOwnerOutsideTheSourceRangeAndChangesNothing(t *testing.T) {
	requireRoot(t)
	root := filepath.Join(t.TempDir(), "rootfs")
	idTree(t, root, srcBase)
	stray := filepath.Join(root, "etc", "zz-host-owned")
	must(t, os.WriteFile(stray, []byte("x"), 0o644))
	chownTo(t, stray, 0, 0)

	remap := errlog.Value(NewIDRemap(unpriv(srcBase), unpriv(dstBase)))
	_, err := ReownTree(context.Background(), root, remap)
	if !errors.Is(err, ErrIDRangeMismatch) || !strings.Contains(err.Error(), "/etc/zz-host-owned") {
		t.Fatalf("an owner outside the source range was not refused by name: %v", err)
	}
	for _, rel := range []string{".", "bin/a", "etc/marker", "root/suid-bb"} {
		if uid, _, _ := ownerMode(t, filepath.Join(root, rel)); uid != srcBase {
			t.Fatalf("a refused re-own changed %s to %d", rel, uid)
		}
	}
	if _, _, mode := ownerMode(t, filepath.Join(root, "root", "suid-bb")); mode != 0o4755 {
		t.Fatalf("a refused re-own changed the mode of suid-bb to %04o", mode)
	}
}

// chainOf archives tree as a full, changes it in place, and archives the
// change as an incremental on the same listing.
func chainOf(t *testing.T, tree string, change func()) (full, incremental string) {
	t.Helper()
	work := t.TempDir()
	listing := filepath.Join(work, "chain.snar")
	full = filepath.Join(work, "0.tar.gz")
	must(t, runTarCreate(context.Background(), tree, full, listing))
	time.Sleep(1100 * time.Millisecond)
	change()
	incremental = filepath.Join(work, "1.tar.gz")
	must(t, runTarCreate(context.Background(), tree, incremental, listing))
	return full, incremental
}

func TestRestoreRootfsReownsAWholeChainAcrossRanges(t *testing.T) {
	requireGNUTar(t)
	requireRoot(t)
	m := testManager(t)
	tree := filepath.Join(t.TempDir(), "source")
	_, withACL, withCaps := idTree(t, tree, srcBase)
	full, incremental := chainOf(t, tree, func() {
		added := filepath.Join(tree, "home", "e2e", "second.txt")
		must(t, os.WriteFile(added, []byte("second\n"), 0o600))
		chownTo(t, added, srcBase+1000, srcBase+1000)
		must(t, os.Remove(filepath.Join(tree, "bin", "b")))
	})

	target := filepath.Join(t.TempDir(), "containers", "ct-b", "rootfs")
	src, dst := unpriv(srcBase), unpriv(dstBase)
	result, err := m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct-b", RunID: "r1", TargetPath: target, Links: []string{full, incremental},
		SourceIDMap: &src, TargetIDMap: &dst,
	})
	if err != nil {
		t.Fatalf("RestoreRootfs across ranges: %v", err)
	}
	if result.IDShift != IDShiftApplied || result.EntriesMoved == 0 {
		t.Fatalf("result = %+v, want a shift", result)
	}
	if uid, gid, _ := ownerMode(t, target); uid != dstBase || gid != dstBase {
		t.Fatalf("rootfs root = %d:%d", uid, gid)
	}
	if uid, _, mode := ownerMode(t, filepath.Join(target, "home", "e2e", "second.txt")); uid != dstBase+1000 || mode != 0o600 {
		t.Fatalf("the incremental's new file = %d %04o", uid, mode)
	}
	if uid, _, mode := ownerMode(t, filepath.Join(target, "root", "suid-bb")); uid != dstBase || mode != 0o4755 {
		t.Fatalf("suid-bb = %d %04o", uid, mode)
	}
	if _, err := os.Lstat(filepath.Join(target, "bin", "b")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the incremental's deletion was not applied: %v", err)
	}
	if link := errlog.Value(os.Readlink(filepath.Join(target, "root", "marker-link"))); link != "../etc/marker" {
		t.Fatalf("the symlink reads %q", link)
	}
	if withCaps {
		got, present, err := lgetxattr(filepath.Join(target, "bin", "ping"), xattrFileCaps)
		if err != nil || !present || !bytes.Equal(got, capBytes(vfsCapRevision3, dstBase)) {
			t.Fatalf("the archived file capability did not arrive rooted in the target range: %x, %v, %v",
				got, present, err)
		}
	}
	if withACL {
		got, present, err := lgetxattr(filepath.Join(target, "etc", "marker"), xattrACLAccess)
		if err != nil || !present || !bytes.Contains(got, le32(dstBase+1000)) || bytes.Contains(got, le32(srcBase+1000)) {
			t.Fatalf("the archived ACL did not arrive naming the target range: %x, %v, %v", got, present, err)
		}
	}
	assertOnlyEntry(t, filepath.Dir(target), "rootfs")
}

func TestRestoreRootfsRefusalLeavesTheTargetUntouched(t *testing.T) {
	requireGNUTar(t)
	requireRoot(t)
	m := testManager(t)
	tree := filepath.Join(t.TempDir(), "source")
	idTree(t, tree, srcBase)
	stray := filepath.Join(tree, "zz-host-owned")
	must(t, os.WriteFile(stray, []byte("x"), 0o644))
	chownTo(t, stray, 0, 0)
	archive := filepath.Join(t.TempDir(), "full.tar.gz")
	must(t, runTarCreate(context.Background(), tree, archive, filepath.Join(t.TempDir(), "l.snar")))

	target := filepath.Join(t.TempDir(), "containers", "ct-b", "rootfs")
	must(t, os.MkdirAll(filepath.Join(target, "etc"), 0o755))
	must(t, os.WriteFile(filepath.Join(target, "etc", "before"), []byte("before"), 0o644))
	chownTo(t, target, dstBase, dstBase)

	src, dst := unpriv(srcBase), unpriv(dstBase)
	_, err := m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct-b", RunID: "r1", TargetPath: target, Links: []string{archive},
		SourceIDMap: &src, TargetIDMap: &dst,
	})
	if !errors.Is(err, ErrIDRangeMismatch) || !strings.Contains(err.Error(), "left untouched") {
		t.Fatalf("a tree with a host-owned entry restored across ranges: %v", err)
	}
	body, readErr := os.ReadFile(filepath.Join(target, "etc", "before"))
	if readErr != nil || string(body) != "before" {
		t.Fatalf("the refused restore changed the target: %q, %v", body, readErr)
	}
	if uid, _, _ := ownerMode(t, target); uid != dstBase {
		t.Fatalf("the refused restore re-owned the target: %d", uid)
	}
	assertOnlyEntry(t, filepath.Dir(target), "rootfs")
}

func TestRestoreRootfsInfersALegacyRangeOnlyWhenUnambiguous(t *testing.T) {
	requireGNUTar(t)
	requireRoot(t)
	m := testManager(t)
	dst := unpriv(dstBase)

	tree := filepath.Join(t.TempDir(), "source")
	idTree(t, tree, srcBase)
	archive := filepath.Join(t.TempDir(), "full.tar.gz")
	must(t, runTarCreate(context.Background(), tree, archive, filepath.Join(t.TempDir(), "l.snar")))
	target := filepath.Join(t.TempDir(), "containers", "ct-b", "rootfs")
	result, err := m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct-b", RunID: "r1", TargetPath: target, Links: []string{archive}, TargetIDMap: &dst,
	})
	if err != nil || result.IDShift != IDShiftApplied {
		t.Fatalf("an unambiguous legacy tree: %+v, %v", result, err)
	}
	if uid, _, _ := ownerMode(t, filepath.Join(target, "home", "e2e", "owned-by-1000.txt")); uid != dstBase+1000 {
		t.Fatalf("the inferred shift left uid %d", uid)
	}

	ambiguous := filepath.Join(t.TempDir(), "ambiguous")
	idTree(t, ambiguous, srcBase)
	wide := filepath.Join(ambiguous, "etc", "wide")
	must(t, os.WriteFile(wide, []byte("x"), 0o644))
	chownTo(t, wide, srcBase+70000, srcBase)
	archive = filepath.Join(t.TempDir(), "ambiguous.tar.gz")
	must(t, runTarCreate(context.Background(), ambiguous, archive, filepath.Join(t.TempDir(), "l2.snar")))
	target2 := filepath.Join(t.TempDir(), "containers", "ct-c", "rootfs")
	must(t, os.MkdirAll(target2, 0o755))
	must(t, os.WriteFile(filepath.Join(target2, "before"), []byte("before"), 0o644))
	_, err = m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct-c", RunID: "r2", TargetPath: target2, Links: []string{archive}, TargetIDMap: &dst,
	})
	if !errors.Is(err, ErrIDRangeUnknown) {
		t.Fatalf("an ambiguous legacy tree restored: %v", err)
	}
	if body := errlog.Value(os.ReadFile(filepath.Join(target2, "before"))); string(body) != "before" {
		t.Fatal("the refused legacy restore changed the target")
	}
	assertOnlyEntry(t, filepath.Dir(target2), "rootfs")
}

func TestRestoreRootfsIntoTheSameRangeOrWithoutATargetTouchesNoOwner(t *testing.T) {
	requireGNUTar(t)
	requireRoot(t)
	m := testManager(t)
	tree := filepath.Join(t.TempDir(), "source")
	idTree(t, tree, srcBase)
	archive := filepath.Join(t.TempDir(), "full.tar.gz")
	must(t, runTarCreate(context.Background(), tree, archive, filepath.Join(t.TempDir(), "l.snar")))

	same := unpriv(srcBase)
	for name, req := range map[string]RestoreRequest{
		IDShiftNone:    {SourceIDMap: &same, TargetIDMap: &same},
		IDShiftSkipped: {},
	} {
		target := filepath.Join(t.TempDir(), "containers", "ct", "rootfs")
		req.CTID, req.RunID, req.TargetPath, req.Links = "ct", "r-"+name, target, []string{archive}
		result, err := m.RestoreRootfs(context.Background(), req)
		if err != nil || result.IDShift != name {
			t.Fatalf("%s: %+v, %v", name, result, err)
		}
		if uid, _, _ := ownerMode(t, filepath.Join(target, "home", "e2e", "owned-by-1000.txt")); uid != srcBase+1000 {
			t.Fatalf("%s: owner changed to %d", name, uid)
		}
	}
}
