// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/imagefetch"
)

// seedCachedImage writes one cache entry with a chosen age, which is all the
// LRU reads.
func seedCachedImage(t *testing.T, root, name string, size int, age time.Duration) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnImageBackingAGuestDiskIsNeverEvicted(t *testing.T) {
	cache := t.TempDir()
	pool := t.TempDir()
	fetcher, err := imagefetch.New(cache, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The oldest image is the one a guest disk backs onto, through an overlay:
	// the LRU would pick it first, and must not.
	baseImage := seedCachedImage(t, cache, "debian-12.qcow2", 1000, 48*time.Hour)
	seedCachedImage(t, cache, "alpine-3.qcow2", 1000, 24*time.Hour)

	disk := filepath.Join(pool, "vm1", "disk-0")
	overlay := filepath.Join(pool, "vm1", "disk-0.snap-1")
	chains := map[string]string{
		canonicalPath(overlay): "disk-0", // relative, as a movable guest folder writes it
		canonicalPath(disk):    baseImage,
	}
	if err := os.MkdirAll(filepath.Dir(disk), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{disk, overlay} {
		if err := os.WriteFile(file, []byte("qcow2"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	backingOf := func(path string) string { return chains[canonicalPath(path)] }

	cached, err := fetcher.List(nil)
	if err != nil {
		t.Fatal(err)
	}
	inUse := backedImages([]string{overlay}, backingOf, cached)
	if !inUse["debian-12"] || inUse["alpine-3"] {
		t.Fatalf("in use = %v, want only debian-12", inUse)
	}

	evicted, err := fetcher.Evict(1000, inUse)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(baseImage); err != nil {
		t.Fatalf("the image a guest disk backs onto was evicted (evicted %v)", evicted)
	}
	if len(evicted) != 1 || evicted[0] != "alpine-3" {
		t.Fatalf("evicted %v, want [alpine-3]", evicted)
	}
	if err := fetcher.Remove("debian-12", inUse); !errors.Is(err, imagefetch.ErrInUse) {
		t.Fatalf("Remove of an in-use image = %v, want ErrInUse", err)
	}
}

func TestBackingChainWalkStopsOnALoop(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	backingOf := func(path string) string {
		if filepath.Base(path) == "a" {
			return "b"
		}
		return "a"
	}
	done := make(chan map[string]bool, 1)
	go func() { done <- backedImages([]string{a, b}, backingOf, nil) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a looping backing chain never ended")
	}
}

func TestDiskFilesAreReadFromTheQEMUCommandLine(t *testing.T) {
	argv := []string{
		"qemu-system-x86_64",
		"-blockdev", "driver=file,node-name=blk0-file,filename=/pool/vm1/disk,,0,cache.direct=on",
		"-blockdev", "driver=qcow2,node-name=blk0,file=blk0-file",
		"-drive", "if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE.fd",
	}
	got := diskFilesFromArgv(argv)
	if len(got) != 2 || got[0] != "/pool/vm1/disk,0" || got[1] != "/usr/share/OVMF/OVMF_CODE.fd" {
		t.Fatalf("diskFilesFromArgv = %q", got)
	}
}

func TestBackedVolumeLedgerDropsVolumesThatAreGone(t *testing.T) {
	dir := t.TempDir()
	ledger := &backedVolumeLedger{path: filepath.Join(dir, "ledger.json")}
	kept := filepath.Join(dir, "kept")
	if err := os.WriteFile(kept, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, volume := range []string{kept, filepath.Join(dir, "gone"), kept} {
		if err := ledger.record(volume); err != nil {
			t.Fatal(err)
		}
	}
	if got := ledger.volumes(); len(got) != 1 || got[0] != kept {
		t.Fatalf("volumes = %v, want [%s]", got, kept)
	}
}

func TestGuestArgvFilesReadsEveryStartedGuest(t *testing.T) {
	root := t.TempDir()
	for vmID, file := range map[string]string{"vm1": "/pool/vm1/disk-0", "vm2": "/pool/vm2/disk-0"} {
		dir := filepath.Join(root, vmID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		body := `{"argv":["qemu","-blockdev","driver=file,node-name=blk0-file,filename=` + file + `"]}`
		if err := os.WriteFile(filepath.Join(dir, "args.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A guest that was defined but never started has a directory and no args.
	if err := os.MkdirAll(filepath.Join(root, "vm3"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := guestArgvFiles(root)
	if len(got) != 2 {
		t.Fatalf("guestArgvFiles = %v, want the two started guests' disks", got)
	}
}
