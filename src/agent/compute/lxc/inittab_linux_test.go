// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// busyboxRootfs lays down the parts of an alpine minirootfs init detection and
// the console setup look at.
func busyboxRootfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "bin"), 0o755))
	must(t, os.MkdirAll(filepath.Join(root, "sbin"), 0o755))
	must(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "bin", "busybox"), []byte("elf"), 0o755))
	must(t, os.Symlink("/bin/busybox", filepath.Join(root, "sbin", "init")))
	return root
}

func TestPrepareConsoleInitAdaptsTheImagesInittabAndKeepsTheOriginal(t *testing.T) {
	root := busyboxRootfs(t)
	inittab := filepath.Join(root, "etc", "inittab")
	must(t, os.WriteFile(inittab, []byte(alpineInittab), 0o644))

	done, err := PrepareConsoleInit(root, IDShift{})
	if err != nil || done != InittabAdapted {
		t.Fatalf("PrepareConsoleInit = %q, %v", done, err)
	}
	body := errlog.Value(os.ReadFile(inittab))
	if !inittabHasConsole(string(body)) {
		t.Fatalf("no console entry after adapting:\n%s", body)
	}
	original, err := os.ReadFile(filepath.Join(root, "etc", inittabOriginalName))
	if err != nil || string(original) != alpineInittab {
		t.Fatalf("the image's own inittab was not kept verbatim: %v", err)
	}
	if done, err := PrepareConsoleInit(root, IDShift{}); err != nil || done != InittabUntouched {
		t.Fatalf("a second pass = %q, %v", done, err)
	}
}

func TestPrepareConsoleInitWritesADefaultOwnedByTheContainersRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to hand the file to a shifted uid")
	}
	root := busyboxRootfs(t)
	shift := IDShift{UIDStart: 100000, UIDCount: 65536, GIDStart: 100000, GIDCount: 65536}
	done, err := PrepareConsoleInit(root, shift)
	if err != nil || done != InittabGenerated {
		t.Fatalf("PrepareConsoleInit = %q, %v", done, err)
	}
	info, err := os.Lstat(filepath.Join(root, "etc", "inittab"))
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Uid != 100000 || stat.Gid != 100000 {
		t.Fatalf("the generated inittab is owned by %d:%d, want the container's root 100000", stat.Uid, stat.Gid)
	}
}

func TestPrepareConsoleInitNeverFollowsALinkOutOfTheRootfs(t *testing.T) {
	root := busyboxRootfs(t)
	host := filepath.Join(t.TempDir(), "host-inittab")
	must(t, os.WriteFile(host, []byte(alpineInittab), 0o644))
	must(t, os.Symlink(host, filepath.Join(root, "etc", "inittab")))

	if done, err := PrepareConsoleInit(root, IDShift{}); err != nil || done != InittabUntouched {
		t.Fatalf("PrepareConsoleInit through a link = %q, %v", done, err)
	}
	body := errlog.Value(os.ReadFile(host))
	if string(body) != alpineInittab {
		t.Fatal("a link in the image let the console setup rewrite a host file")
	}
}

func TestPrepareConsoleInitLeavesNonBusyboxRootfsAlone(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "lib", "systemd"), 0o755))
	must(t, os.MkdirAll(filepath.Join(root, "sbin"), 0o755))
	must(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "lib", "systemd", "systemd"), []byte("elf"), 0o755))
	must(t, os.Symlink("/lib/systemd/systemd", filepath.Join(root, "sbin", "init")))

	if done, err := PrepareConsoleInit(root, IDShift{}); err != nil || done != InittabUntouched {
		t.Fatalf("a systemd rootfs = %q, %v", done, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "etc", "inittab")); !os.IsNotExist(err) {
		t.Fatalf("an inittab was written into a systemd rootfs: %v", err)
	}
}
