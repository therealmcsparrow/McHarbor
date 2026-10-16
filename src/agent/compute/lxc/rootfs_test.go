// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"strings"
	"testing"
)

func TestEntryPathRefusesTraversal(t *testing.T) {
	for _, name := range []string{
		"/etc/passwd", "../evil", "etc/../../evil", "a/..", "..", "", "a\x00b",
	} {
		if _, err := EntryPath(name); !errors.Is(err, ErrUnsafeArchive) {
			t.Errorf("EntryPath(%q) = %v, want ErrUnsafeArchive", name, err)
		}
	}
	cases := map[string]string{
		"./":               "",
		".":                "",
		"etc/":             "etc",
		"./etc/os-release": "etc/os-release",
		"usr//bin/./sh":    "usr/bin/sh",
	}
	for name, want := range cases {
		got, err := EntryPath(name)
		if err != nil || got != want {
			t.Errorf("EntryPath(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestSymlinkEscapes(t *testing.T) {
	escapes := map[string]string{
		"etc/shadow-link": "../../etc/shadow",
		"a":               "..",
		"usr/lib/x":       "../../../x",
		"empty":           "",
	}
	for rel, target := range escapes {
		if !SymlinkEscapes(rel, target) {
			t.Errorf("SymlinkEscapes(%q, %q) = false, want true", rel, target)
		}
	}
	inside := map[string]string{
		"bin/sh":          "/bin/busybox",
		"usr/lib/libc.so": "../../lib/libc.so.6",
		"etc/localtime":   "zoneinfo/UTC",
	}
	for rel, target := range inside {
		if SymlinkEscapes(rel, target) {
			t.Errorf("SymlinkEscapes(%q, %q) = true, want false", rel, target)
		}
	}
}

func TestCheckRootfsRemovableNeverPoolRoot(t *testing.T) {
	refused := [][2]string{
		{"/mcharbor/compute", "/mcharbor/compute"},
		{"/mcharbor/compute/", "/mcharbor/compute"},
		{"/", "/mcharbor/compute"},
		{"/mcharbor/computex/containers/a/rootfs", "/mcharbor/compute"},
		{"/etc", "/mcharbor/compute"},
		{"/mcharbor/compute/../../etc", "/mcharbor/compute"},
		{"/anything/rootfs", "/"},
		{"containers/a/rootfs", "/mcharbor/compute"},
		{"/mcharbor/compute/containers/a/rootfs", "vg/thin"},
		{"", "/mcharbor/compute"},
		{"/mcharbor/compute/containers/a/rootfs", ""},
	}
	for _, pair := range refused {
		if err := CheckRootfsRemovable(pair[0], pair[1]); !errors.Is(err, ErrRootfsGuard) {
			t.Errorf("CheckRootfsRemovable(%q, %q) = %v, want ErrRootfsGuard", pair[0], pair[1], err)
		}
	}
	if err := CheckRootfsRemovable("/mcharbor/compute/containers/a/rootfs", "/mcharbor/compute/"); err != nil {
		t.Errorf("a rootfs inside its pool was refused: %v", err)
	}
}

func TestIDShift(t *testing.T) {
	shift := ShiftFor(Spec{UIDMapStart: 100000, UIDMapCount: 65536, GIDMapStart: 100000, GIDMapCount: 65536})
	if uid, err := shift.UID(0); err != nil || uid != 100000 {
		t.Fatalf("UID(0) = %d, %v", uid, err)
	}
	if gid, err := shift.GID(65535); err != nil || gid != 165535 {
		t.Fatalf("GID(65535) = %d, %v", gid, err)
	}
	if _, err := shift.UID(65536); !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("UID(65536) = %v, want ErrUnsafeArchive", err)
	}
	privileged := ShiftFor(Spec{Privileged: true, UIDMapStart: 100000, UIDMapCount: 65536})
	if uid, err := privileged.UID(1000); err != nil || uid != 1000 {
		t.Fatalf("privileged UID(1000) = %d, %v", uid, err)
	}
}

func TestCheckTemplatePath(t *testing.T) {
	if err := CheckTemplatePath("/pool/containers/abc/rootfs"); err != nil {
		t.Fatalf("a container rootfs was refused as a template: %v", err)
	}
	for _, bad := range []string{"/etc", "/root", "pool/containers/abc/rootfs", "/pool/containers/../etc/rootfs", "/pool/x/abc/rootfs"} {
		if err := CheckTemplatePath(bad); !errors.Is(err, ErrUnsafeArchive) {
			t.Errorf("CheckTemplatePath(%q) = %v, want ErrUnsafeArchive", bad, err)
		}
	}
}

func TestEmptyDirsAfterRootfsRemovalNamesOnlyTheContainerAndContainersDirs(t *testing.T) {
	got := EmptyDirsAfterRootfsRemoval("ct1", "/pool/containers/ct1/rootfs", "/pool")
	want := []string{"/pool/containers/ct1", "/pool/containers"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("dirs = %v, want %v", got, want)
	}
	if got := EmptyDirsAfterRootfsRemoval("ct1", "/pool/containers/ct1/rootfs", "/pool/"); len(got) != 2 {
		t.Fatalf("a trailing slash on the pool target changed the answer: %v", got)
	}
}

func TestEmptyDirsAfterRootfsRemovalNeverNamesThePoolRootOrAStrangeShape(t *testing.T) {
	cases := []struct{ ctID, rootfs, pool string }{
		{"ct1", "/containers/ct1/rootfs", "/"},
		{"ct1", "/pool/ct1/rootfs", "/pool"},
		{"ct1", "/pool/other/containers/ct1/rootfs", "/pool"},
		{"ct1", "/pool/containers/ct2/rootfs", "/pool"},
		{"", "/pool/containers//rootfs", "/pool"},
		{"ct1", "pool/containers/ct1/rootfs", "pool"},
		{"containers", "/pool/containers/rootfs", "/pool/containers"},
	}
	for _, c := range cases {
		for _, dir := range EmptyDirsAfterRootfsRemoval(c.ctID, c.rootfs, c.pool) {
			if dir == c.pool || dir == "/" || !strings.HasPrefix(dir, "/pool/containers") {
				t.Fatalf("%+v: listed %q", c, dir)
			}
		}
		if got := EmptyDirsAfterRootfsRemoval(c.ctID, c.rootfs, c.pool); got != nil {
			t.Fatalf("%+v: listed %v, want nothing for this shape", c, got)
		}
	}
}
