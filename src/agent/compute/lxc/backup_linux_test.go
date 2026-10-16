// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

func requireGNUTar(t *testing.T) {
	t.Helper()
	if err := assertGNUTar(context.Background()); err != nil {
		t.Skipf("needs GNU tar: %v", err)
	}
}

// writeTree lays down a small rootfs-shaped tree: nested directories, each with
// a file, which is what gives GNU tar's listing directories to match by inode.
func writeTree(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{"etc/network", "etc/periodic/monthly", "run/tmp", "var/log", "root", "d1", "d2"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "file"), []byte(dir+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "top"), []byte("top\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// treeListing is every path under root with its type and, for files, content,
// so two trees can be compared exactly.
func treeListing(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := errlog.Value(filepath.Rel(root, path))
		line := rel + " " + entry.Type().String()
		if entry.Type().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += " " + strings.TrimSpace(string(body))
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// restoreChain applies archives into a fresh directory the way RestoreRootfs
// does and returns the result, or the first extract error.
func restoreChain(t *testing.T, archives ...string) (string, error) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "restored")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, archive := range archives {
		if err := runTarExtract(context.Background(), target, archive); err != nil {
			return target, err
		}
	}
	return target, nil
}

func TestTarIncrementalRestoresWhenInodesAreStable(t *testing.T) {
	requireGNUTar(t)
	work := t.TempDir()
	tree := filepath.Join(work, "rootfs")
	writeTree(t, tree)
	listing := filepath.Join(work, "chain.snar")
	full := filepath.Join(work, "0.tar.gz")
	if err := runTarCreate(context.Background(), tree, full, listing); err != nil {
		t.Fatal(err)
	}

	// tar compares against the level-0 start time at the fine clock while file
	// times come from the coarse one, so a change in the same tick can be missed.
	// A real chain has hours between runs; the test waits past the tick.
	time.Sleep(1100 * time.Millisecond)

	// The same tree, changed in place: new, modified and deleted files and a
	// genuine directory rename. Every directory keeps its inode, as it does on
	// the live rootfs and in every btrfs, ZFS or LVM snapshot of it.
	must(t, os.WriteFile(filepath.Join(tree, "root/second.txt"), []byte("second\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(tree, "top"), []byte("top changed\n"), 0o644))
	must(t, os.Remove(filepath.Join(tree, "var/log/file")))
	must(t, os.Rename(filepath.Join(tree, "etc/network"), filepath.Join(tree, "etc/network.d")))
	incremental := filepath.Join(work, "1.tar.gz")
	if err := runTarCreate(context.Background(), tree, incremental, listing); err != nil {
		t.Fatal(err)
	}

	restored, err := restoreChain(t, full, incremental)
	if err != nil {
		t.Fatalf("an incremental over stable inodes did not restore: %v", err)
	}
	if got, want := treeListing(t, restored), treeListing(t, tree); got != want {
		t.Fatalf("restored tree differs\n got: %s\nwant: %s", got, want)
	}
}

// moveDirectoryIdentities rebuilds tree with the same paths and contents but
// with the root directory's inode now the one d1 had — what a fresh `cp -a`
// hands out when the filesystem reuses a number the previous copy freed.
func moveDirectoryIdentities(t *testing.T, tree string) {
	t.Helper()
	old := tree + ".old"
	must(t, os.Rename(tree, old))
	must(t, os.Rename(filepath.Join(old, "d1"), tree))
	must(t, os.Rename(filepath.Join(tree, "file"), filepath.Join(old, "d1-file")))
	must(t, os.Mkdir(filepath.Join(tree, "d1"), 0o755))
	must(t, os.Rename(filepath.Join(old, "d1-file"), filepath.Join(tree, "d1", "file")))
	entries, err := os.ReadDir(old)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		must(t, os.Rename(filepath.Join(old, entry.Name()), filepath.Join(tree, entry.Name())))
	}
	must(t, os.Remove(old))
}

func TestTarIncrementalCannotBeRestoredWhenDirectoryInodesMove(t *testing.T) {
	requireGNUTar(t)
	work := t.TempDir()
	tree := filepath.Join(work, "rootfs")
	writeTree(t, tree)
	listing := filepath.Join(work, "chain.snar")
	full := filepath.Join(work, "0.tar.gz")
	if err := runTarCreate(context.Background(), tree, full, listing); err != nil {
		t.Fatal(err)
	}

	moveDirectoryIdentities(t, tree)
	must(t, os.WriteFile(filepath.Join(tree, "root/second.txt"), []byte("second\n"), 0o644))
	incremental := filepath.Join(work, "1.tar.gz")
	if err := runTarCreate(context.Background(), tree, incremental, listing); err != nil {
		t.Fatal(err)
	}

	restored, err := restoreChain(t, full, incremental)
	if err == nil && treeListing(t, restored) == treeListing(t, tree) {
		t.Fatal("tar restored an incremental whose directory inodes moved; the dir-copy guard would be unnecessary")
	}
	t.Logf("as expected, the chain does not restore: %v", err)
}

// TestTarIncrementalFromRecopiedTreesIsNotRestorable is the fifth pass's bug
// itself: each run reads a fresh `cp -a` of the rootfs. Whether a copy reuses a
// freed inode at a different path depends on the filesystem's allocator, so it
// tries up to 60 times and skips if the filesystem never reused one.
func TestTarIncrementalFromRecopiedTreesIsNotRestorable(t *testing.T) {
	requireGNUTar(t)
	work := t.TempDir()
	source := filepath.Join(work, "rootfs")
	writeTree(t, source)
	for a := 0; a < 6; a++ {
		for b := 0; b < 6; b++ {
			dir := filepath.Join(source, fmt.Sprintf("x%d", a), fmt.Sprintf("y%d", b))
			must(t, os.MkdirAll(dir, 0o755))
			must(t, os.WriteFile(filepath.Join(dir, "file"), []byte(dir), 0o644))
		}
	}
	copyTree := func(target string) {
		if out, err := exec.Command("cp", "-a", source+"/.", target).CombinedOutput(); err != nil {
			t.Fatalf("cp: %v: %s", err, out)
		}
	}

	for attempt := 1; attempt <= 60; attempt++ {
		listing := filepath.Join(work, "chain.snar")
		errlog.Log(os.Remove(listing))
		first, second := filepath.Join(work, "copy1"), filepath.Join(work, "copy2")
		copyTree(first)
		full := filepath.Join(work, "0.tar.gz")
		must(t, runTarCreate(context.Background(), first, full, listing))
		must(t, os.RemoveAll(first))

		must(t, os.WriteFile(filepath.Join(source, "root/second.txt"), []byte(fmt.Sprint(attempt)), 0o644))
		copyTree(second)
		incremental := filepath.Join(work, "1.tar.gz")
		must(t, runTarCreate(context.Background(), second, incremental, listing))
		must(t, os.RemoveAll(second))

		restored, err := restoreChain(t, full, incremental)
		if err != nil || treeListing(t, restored) != treeListing(t, source) {
			t.Logf("attempt %d: a chain read from two copies does not restore: %v", attempt, err)
			return
		}
	}
	t.Skip("this filesystem never reused a freed inode at another path in 60 copies; " +
		"TestTarIncrementalCannotBeRestoredWhenDirectoryInodesMove covers the mechanism")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// backupOnce runs one Backup of a stopped container reading tree directly.
func backupOnce(t *testing.T, m *Manager, tree, runID, kind string) BackupResult {
	t.Helper()
	result, err := m.Backup(context.Background(), BackupRequest{
		CTID: "ct-backup", RunID: runID, Kind: kind, Manifest: []byte("{}"),
		RootfsPath: tree, ListingName: "mcharbor-ct-chain1",
		ScratchDir: filepath.Join(t.TempDir(), "scratch"),
	})
	if err != nil {
		t.Fatalf("Backup %s: %v", runID, err)
	}
	return result
}

func TestBackupTakesAnIncrementalOnlyOverAListingFromTheSameTree(t *testing.T) {
	requireGNUTar(t)
	m := testManager(t)
	tree := filepath.Join(t.TempDir(), "rootfs")
	writeTree(t, tree)

	full := backupOnce(t, m, tree, "run1", "full")
	if full.Kind != "full" || full.FellBackToFull {
		t.Fatalf("first run = %+v", full)
	}
	listing := filepath.Join(RuntimeRoot(), ".backup-listings", "mcharbor-ct-chain1.snar")
	if got := readListingOrigin(listing); got != ListingOriginTree {
		t.Fatalf("a run of the rootfs itself recorded origin %q", got)
	}

	next := backupOnce(t, m, tree, "run2", "incremental")
	if next.Kind != "incremental" || next.FellBackToFull {
		t.Fatalf("an incremental over the same tree = %+v", next)
	}

	must(t, os.Remove(listingOriginPath(listing)))
	legacy := backupOnce(t, m, tree, "run3", "incremental")
	if legacy.Kind != "full" || !legacy.FellBackToFull || legacy.FallbackReason != reasonOriginUnknown {
		t.Fatalf("an incremental over a listing of unknown origin = %+v", legacy)
	}

	must(t, writeListingOrigin(listing, ListingOriginCopy))
	afterCopy := backupOnce(t, m, tree, "run4", "incremental")
	if afterCopy.Kind != "full" || !afterCopy.FellBackToFull || afterCopy.FallbackReason != reasonOriginCopy {
		t.Fatalf("an incremental over a listing read from a copy = %+v", afterCopy)
	}
	if got := readListingOrigin(listing); got != ListingOriginTree {
		t.Fatalf("the full copy that replaced a copy's listing recorded origin %q", got)
	}
}

// archiveOf builds a full archive of a small tree for restore tests.
func archiveOf(t *testing.T, content string) string {
	t.Helper()
	work := t.TempDir()
	tree := filepath.Join(work, "src")
	must(t, os.MkdirAll(filepath.Join(tree, "etc"), 0o755))
	must(t, os.WriteFile(filepath.Join(tree, "etc", "marker"), []byte(content), 0o644))
	archive := filepath.Join(work, "full.tar.gz")
	must(t, runTarCreate(context.Background(), tree, archive, filepath.Join(work, "l.snar")))
	return archive
}

func TestRestoreRootfsSwapsTheRebuiltTreeInOnlyOnSuccess(t *testing.T) {
	requireGNUTar(t)
	m := testManager(t)
	target := filepath.Join(t.TempDir(), "containers", "ct1", "rootfs")
	must(t, os.MkdirAll(filepath.Join(target, "etc"), 0o755))
	must(t, os.WriteFile(filepath.Join(target, "etc", "marker"), []byte("before"), 0o644))

	good := archiveOf(t, "restored")
	broken := filepath.Join(t.TempDir(), "broken.tar.gz")
	must(t, os.WriteFile(broken, []byte("this is not a gzip stream"), 0o600))

	_, err := m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct1", RunID: "r1", TargetPath: target, Links: []string{good, broken},
	})
	if err == nil {
		t.Fatal("a chain with a broken link restored")
	}
	if !strings.Contains(err.Error(), "left untouched") {
		t.Fatalf("the error does not say the target is intact: %v", err)
	}
	body, readErr := os.ReadFile(filepath.Join(target, "etc", "marker"))
	if readErr != nil || !bytes.Equal(body, []byte("before")) {
		t.Fatalf("a failed restore changed the target: %q, %v", body, readErr)
	}
	assertOnlyEntry(t, filepath.Dir(target), "rootfs")

	if _, err := m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct1", RunID: "r2", TargetPath: target, Links: []string{good},
	}); err != nil {
		t.Fatalf("a whole chain did not restore: %v", err)
	}
	body = errlog.Value(os.ReadFile(filepath.Join(target, "etc", "marker")))
	if string(body) != "restored" {
		t.Fatalf("the target reads %q after a successful restore", body)
	}
	assertOnlyEntry(t, filepath.Dir(target), "rootfs")
}

func TestAFailedRestoreIntoANewContainerLeavesNoDirectories(t *testing.T) {
	requireGNUTar(t)
	m := testManager(t)
	pool := t.TempDir()
	target := filepath.Join(pool, "containers", "ct2", "rootfs")
	broken := filepath.Join(t.TempDir(), "broken.tar.gz")
	must(t, os.WriteFile(broken, []byte("not gzip"), 0o600))

	if _, err := m.RestoreRootfs(context.Background(), RestoreRequest{
		CTID: "ct2", RunID: "r1", TargetPath: target, Links: []string{broken},
	}); err == nil {
		t.Fatal("a broken archive restored")
	}
	if _, err := os.Stat(filepath.Join(pool, "containers")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the failed recreate left containers/ behind: %v", err)
	}
}

func assertOnlyEntry(t *testing.T, dir, want string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) != 1 || names[0] != want {
		t.Fatalf("%s holds %v, want only %s (a staging or retired tree was left behind)", dir, names, want)
	}
}

func TestMissingAncestorsStopsAtTheFirstThatExists(t *testing.T) {
	base := t.TempDir()
	got := missingAncestors(filepath.Join(base, "a", "b", "c"))
	want := []string{filepath.Join(base, "a", "b", "c"), filepath.Join(base, "a", "b"), filepath.Join(base, "a")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := missingAncestors(base); len(got) != 0 {
		t.Fatalf("an existing directory listed %v", got)
	}
}
