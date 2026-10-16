// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package storage

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A clone lands in the new guest's own directory, which does not exist yet;
// both clone kinds must create it rather than fail with ENOENT.
func TestCloneVolumeCreatesTheTargetDirectory(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not installed")
	}
	root := t.TempDir()
	pool := Pool{ID: "p", Name: "p", Backend: "dir", Target: root}
	source := filepath.Join(root, "src-vm", "disk-0")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("qemu-img", "create", "-f", "qcow2", source, "16M").CombinedOutput(); err != nil {
		t.Fatalf("seeding source: %v: %s", err, out)
	}
	driver := NewDirDriver()
	for _, linked := range []bool{true, false} {
		target := "clone-full/disk-0"
		if linked {
			target = "clone-linked/disk-0"
		}
		volume, err := driver.CloneVolume(context.Background(), pool, source, target, linked)
		if err != nil {
			t.Fatalf("linked=%v: %v", linked, err)
		}
		if _, err := os.Stat(volume.Path); err != nil {
			t.Fatalf("linked=%v: clone not written: %v", linked, err)
		}
	}
}
