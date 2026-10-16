// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"strings"
	"testing"
)

func fileBlockdev(t *testing.T, disk Disk) string {
	t.Helper()
	argv, err := buildDiskArgs(Spec{Disks: []Disk{disk}}, false)
	if err != nil {
		t.Fatalf("buildDiskArgs: %v", err)
	}
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "-blockdev" && strings.Contains(argv[i+1], "node-name=blk0-file") {
			return argv[i+1]
		}
	}
	t.Fatalf("no file blockdev in %v", argv)
	return ""
}

func TestShareRWDiskFileBlockdevDisablesLocking(t *testing.T) {
	shared := fileBlockdev(t, Disk{Slot: 0, Path: "/img/a.qcow2", ShareRW: true})
	if !strings.Contains(shared, ",locking=off") {
		t.Fatalf("shared disk file blockdev lacks locking=off: %s", shared)
	}
	plain := fileBlockdev(t, Disk{Slot: 0, Path: "/img/a.qcow2"})
	if strings.Contains(plain, "locking=") {
		t.Fatalf("non-shared disk file blockdev must keep default locking: %s", plain)
	}
}

func TestShareRWLockingStaysOffTheFormatNode(t *testing.T) {
	argv, err := buildDiskArgs(Spec{Disks: []Disk{{Slot: 0, Path: "/img/a.qcow2", ShareRW: true}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range argv {
		if strings.Contains(a, "driver=qcow2") && strings.Contains(a, "locking=") {
			t.Fatalf("locking belongs on the file node, found on format node: %s", a)
		}
	}
}
