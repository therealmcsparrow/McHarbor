// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

type member struct {
	name     string
	kind     byte
	body     string
	link     string
	uid, gid int
	mode     int64
}

func writeArchive(t *testing.T, dir string, compress bool, members []member) string {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	for _, m := range members {
		mode := m.mode
		if mode == 0 {
			mode = 0o644
			if m.kind == tar.TypeDir {
				mode = 0o755
			}
		}
		header := &tar.Header{
			Name: m.name, Typeflag: m.kind, Linkname: m.link,
			Uid: m.uid, Gid: m.gid, Mode: mode, Size: int64(len(m.body)),
		}
		if m.kind != tar.TypeReg {
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if m.kind == tar.TypeReg {
			if _, err := writer.Write([]byte(m.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := buf.Bytes()
	name := "image.tar"
	if compress {
		var zipped bytes.Buffer
		gz := gzip.NewWriter(&zipped)
		if _, err := gz.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		payload = zipped.Bytes()
		name = "image.gz"
	}
	archive := filepath.Join(dir, name)
	if err := os.WriteFile(archive, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return archive
}

func owner(t *testing.T, file string) (uint32, uint32) {
	t.Helper()
	info, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	return stat.Uid, stat.Gid
}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("unpacking a rootfs changes ownership, which needs root")
	}
}

var shift = IDShift{UIDStart: 100000, UIDCount: 65536, GIDStart: 100000, GIDCount: 65536}

func TestUnpackRootfsHappyPath(t *testing.T) {
	requireRoot(t)
	dir := t.TempDir()
	archive := writeArchive(t, dir, true, []member{
		{name: "./", kind: tar.TypeDir, mode: 0o755},
		{name: "etc/", kind: tar.TypeDir},
		{name: "etc/os-release", kind: tar.TypeReg, body: "ID=alpine\n"},
		{name: "home/user/notes", kind: tar.TypeReg, body: "hi", uid: 1000, gid: 1000},
		{name: "bin/busybox", kind: tar.TypeReg, body: "#!", mode: 0o4755},
		{name: "bin/sh", kind: tar.TypeSymlink, link: "/bin/busybox"},
		{name: "bin/ash", kind: tar.TypeLink, link: "bin/busybox"},
		{name: "dev/null", kind: tar.TypeChar},
	})
	target := filepath.Join(dir, "pool", "containers", "ct1", "rootfs")

	if err := UnpackRootfs(context.Background(), archive, target, shift); err != nil {
		t.Fatalf("UnpackRootfs: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(target, "etc", "os-release"))
	if err != nil || string(body) != "ID=alpine\n" {
		t.Fatalf("os-release = %q, %v", body, err)
	}
	if uid, gid := owner(t, target); uid != 100000 || gid != 100000 {
		t.Fatalf("rootfs root owned by %d:%d, want the container's root 100000:100000", uid, gid)
	}
	if uid, gid := owner(t, filepath.Join(target, "home", "user", "notes")); uid != 101000 || gid != 101000 {
		t.Fatalf("notes owned by %d:%d, want 101000:101000", uid, gid)
	}
	if uid, _ := owner(t, filepath.Join(target, "home")); uid != 100000 {
		t.Fatalf("an implied parent is owned by %d, want 100000", uid)
	}
	link, err := os.Readlink(filepath.Join(target, "bin", "sh"))
	if err != nil || link != "/bin/busybox" {
		t.Fatalf("bin/sh -> %q, %v", link, err)
	}
	if uid, _ := owner(t, filepath.Join(target, "bin", "sh")); uid != 100000 {
		t.Fatalf("symlink owned by %d, want 100000", uid)
	}
	info, err := os.Stat(filepath.Join(target, "bin", "busybox"))
	if err != nil || info.Mode()&fs.ModeSetuid == 0 {
		t.Fatalf("busybox lost its setuid bit after the chown: %v %v", info.Mode(), err)
	}
	ash := errlog.Value(os.Stat(filepath.Join(target, "bin", "ash")))
	if !os.SameFile(info, ash) {
		t.Fatal("bin/ash is not a hard link to bin/busybox")
	}
	if _, err := os.Lstat(filepath.Join(target, "dev", "null")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a device node was created on the host: %v", err)
	}
	leftovers := errlog.Value(filepath.Glob(target + ".unpack-*"))
	if len(leftovers) != 0 {
		t.Fatalf("staging trees left behind: %v", leftovers)
	}
}

func TestUnpackRootfsPlainTarPrivileged(t *testing.T) {
	requireRoot(t)
	dir := t.TempDir()
	archive := writeArchive(t, dir, false, []member{
		{name: "etc/hostname", kind: tar.TypeReg, body: "x", uid: 5, gid: 6},
	})
	target := filepath.Join(dir, "rootfs")
	if err := UnpackRootfs(context.Background(), archive, target, IDShift{}); err != nil {
		t.Fatalf("UnpackRootfs: %v", err)
	}
	if uid, gid := owner(t, filepath.Join(target, "etc", "hostname")); uid != 5 || gid != 6 {
		t.Fatalf("privileged ownership shifted to %d:%d", uid, gid)
	}
}

func TestUnpackRootfsRefusesUnsafeArchives(t *testing.T) {
	requireRoot(t)
	outside := t.TempDir()
	cases := map[string][]member{
		"parent traversal": {{name: "../escape", kind: tar.TypeReg, body: "x"}},
		"absolute member":  {{name: "/escape", kind: tar.TypeReg, body: "x"}},
		"escaping symlink": {{name: "etc/shadow", kind: tar.TypeSymlink, link: "../../../../escape"}},
		"write through symlink": {
			{name: "etc", kind: tar.TypeSymlink, link: outside},
			{name: "etc/passwd", kind: tar.TypeReg, body: "root::0:0"},
		},
		"hard link outside": {{name: "x", kind: tar.TypeLink, link: "../../etc/passwd"}},
		"uid out of range":  {{name: "x", kind: tar.TypeReg, body: "x", uid: 70000}},
	}
	for name, members := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			// A valid member first, so the refusal happens with a partial tree on disk.
			archive := writeArchive(t, dir, true, append([]member{
				{name: "ok/file", kind: tar.TypeReg, body: "fine"},
			}, members...))
			parent := filepath.Join(dir, "containers", "ct1")
			target := filepath.Join(parent, "rootfs")

			err := UnpackRootfs(context.Background(), archive, target, shift)
			if !errors.Is(err, ErrUnsafeArchive) {
				t.Fatalf("UnpackRootfs = %v, want ErrUnsafeArchive", err)
			}
			if _, statErr := os.Lstat(parent); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("the partial rootfs (or its container directory) was left behind: %v", statErr)
			}
			if _, statErr := os.Lstat(filepath.Join(outside, "passwd")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatal("the archive wrote outside the rootfs")
			}
			if _, statErr := os.Lstat(filepath.Join(dir, "escape")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatal("the archive wrote beside the rootfs")
			}
		})
	}
}

func TestUnpackRootfsRefusesExistingTree(t *testing.T) {
	requireRoot(t)
	dir := t.TempDir()
	archive := writeArchive(t, dir, false, []member{{name: "a", kind: tar.TypeReg, body: "new"}})
	target := filepath.Join(dir, "rootfs")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UnpackRootfs(context.Background(), archive, target, shift); err == nil {
		t.Fatal("an image was unpacked over an existing rootfs")
	}
	if body := errlog.Value(os.ReadFile(filepath.Join(target, "keep"))); string(body) != "old" {
		t.Fatal("the existing rootfs was changed")
	}
}

func TestCopyTemplateRootfsReownsIntoNewRange(t *testing.T) {
	requireRoot(t)
	dir := t.TempDir()
	template := filepath.Join(dir, "pool", "containers", "tmpl", "rootfs")
	if err := os.MkdirAll(filepath.Join(template, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(template, "etc", "os-release")
	if err := os.WriteFile(file, []byte("ID=debian\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/os-release", filepath.Join(template, "release")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{template, filepath.Join(template, "etc"), filepath.Join(template, "release")} {
		if err := os.Lchown(path, 200000, 200000); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Lchown(file, 201000, 201000); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "pool", "containers", "ct2", "rootfs")
	if err := CopyTemplateRootfs(context.Background(), template, target, shift); err != nil {
		t.Fatalf("CopyTemplateRootfs: %v", err)
	}
	if uid, _ := owner(t, target); uid != 100000 {
		t.Fatalf("copied root owned by %d, want 100000", uid)
	}
	if uid, gid := owner(t, filepath.Join(target, "etc", "os-release")); uid != 101000 || gid != 101000 {
		t.Fatalf("copied file owned by %d:%d, want 101000:101000", uid, gid)
	}
	if link, err := os.Readlink(filepath.Join(target, "release")); err != nil || link != "/etc/os-release" {
		t.Fatalf("symlink copied as %q, %v", link, err)
	}
}
