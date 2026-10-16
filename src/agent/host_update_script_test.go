// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func planFor(t *testing.T, opts hostUpgradeOptions, manager string) hostUpgradePlan {
	t.Helper()
	for _, plan := range hostUpgradePlans(opts) {
		if plan.manager == manager {
			return plan
		}
	}
	t.Fatalf("no plan for %s", manager)
	return hostUpgradePlan{}
}

func TestHostUpgradePlansFlags(t *testing.T) {
	cases := []struct {
		manager string
		opts    hostUpgradeOptions
		want    []string
		absent  []string
	}{
		{"apt", hostUpgradeOptions{}, []string{"apt-get -y upgrade"}, []string{"--fix-missing", "--with-new-pkgs"}},
		{"apt", hostUpgradeOptions{SkipUnavailable: true}, []string{"apt-get -y --fix-missing upgrade"}, []string{"--with-new-pkgs"}},
		{"apt", hostUpgradeOptions{WithNewPackages: true, SkipUnavailable: true}, []string{"--with-new-pkgs --fix-missing upgrade"}, nil},
		{"dnf", hostUpgradeOptions{}, []string{"dnf -y upgrade"}, []string{"--skip-broken", "skip_if_unavailable"}},
		{"dnf", hostUpgradeOptions{SkipUnavailable: true}, []string{"dnf -y --skip-broken --setopt=skip_if_unavailable=True upgrade"}, nil},
		{"yum", hostUpgradeOptions{}, []string{"yum -y update"}, []string{"--skip-broken"}},
		{"yum", hostUpgradeOptions{SkipUnavailable: true}, []string{"yum -y --skip-broken update"}, []string{"skip_if_unavailable"}},
		{"apk", hostUpgradeOptions{SkipUnavailable: true, WithNewPackages: true}, []string{"apk upgrade"}, []string{"--"}},
		{"pacman", hostUpgradeOptions{SkipUnavailable: true}, []string{"pacman -Syu --noconfirm"}, []string{"skip"}},
		{"zypper", hostUpgradeOptions{SkipUnavailable: true, WithNewPackages: true}, []string{"zypper --non-interactive update"}, []string{"skip", "dist-upgrade"}},
	}
	for _, tc := range cases {
		plan := planFor(t, tc.opts, tc.manager)
		for _, want := range tc.want {
			if !strings.Contains(plan.upgrade, want) {
				t.Errorf("%s %+v: upgrade %q lacks %q", tc.manager, tc.opts, plan.upgrade, want)
			}
		}
		for _, absent := range tc.absent {
			if strings.Contains(plan.upgrade, absent) || strings.Contains(plan.retryUpgrade, absent) {
				t.Errorf("%s %+v: upgrade carries %q", tc.manager, tc.opts, absent)
			}
		}
	}
}

func TestHostUpgradeBodyRemoveUnused(t *testing.T) {
	cleanup := map[string]string{
		"apt":    "env DEBIAN_FRONTEND=noninteractive apt-get -y autoremove",
		"dnf":    "dnf -y autoremove",
		"yum":    "yum -y autoremove",
		"pacman": "pacman -Rns --noconfirm $mch_orphans",
	}
	on := hostUpgradeBody(hostUpgradeOptions{RemoveUnused: true})
	off := hostUpgradeBody(hostUpgradeOptions{WithNewPackages: true, SkipUnavailable: true})
	for manager, want := range cleanup {
		if !strings.Contains(on, want) {
			t.Errorf("%s: body with the option lacks %q", manager, want)
		}
		if strings.Contains(off, want) {
			t.Errorf("%s: body without the option carries %q", manager, want)
		}
		if planFor(t, hostUpgradeOptions{}, manager).autoremove == "" {
			t.Errorf("%s: plan has no autoremove", manager)
		}
	}
	for _, manager := range []string{"apk", "zypper"} {
		if got := planFor(t, hostUpgradeOptions{RemoveUnused: true}, manager).autoremove; got != "" {
			t.Errorf("%s: autoremove %q, want none", manager, got)
		}
	}
	if got := strings.Count(on, hostRemoveUnusedNotice); got != 4 {
		t.Errorf("want a cleanup in 4 managers, got %d", got)
	}
	if got := strings.Count(on, hostRemoveUnusedSkipNotice); got != 2 {
		t.Errorf("want apk and zypper to skip with a notice, got %d", got)
	}
	if strings.Contains(off, hostRemoveUnusedNotice) || strings.Contains(off, hostRemoveUnusedSkipNotice) {
		t.Error("the option is off, so no cleanup line may appear")
	}
	if strings.Contains(on, "'") {
		t.Error("the body is wrapped in single quotes and must not contain one")
	}
}

func TestHostUpgradeBodyHasNoSingleQuote(t *testing.T) {
	body := hostUpgradeBody(hostUpgradeOptions{WithNewPackages: true, SkipUnavailable: true})
	if strings.Contains(body, "'") {
		t.Fatal("the body is wrapped in single quotes and must not contain one")
	}
	if got := strings.Count(body, hostUpgradeRetryNotice); got != 6 {
		t.Fatalf("want one retry per manager (6), got %d", got)
	}
}

func TestHostUpdateApplyScriptWrapsBody(t *testing.T) {
	script := hostUpdateApplyScript(hostUpgradeOptions{SkipUnavailable: true})
	if !strings.HasPrefix(script, "sh -lc '") || !strings.HasSuffix(script, "' 2>&1") {
		t.Fatalf("unexpected wrapper: %.40q", script)
	}
	if !strings.Contains(script, "--fix-missing") {
		t.Fatal("skipUnavailable did not reach the apt upgrade")
	}
	if strings.Contains(script, "autoremove") {
		t.Fatal("autoremove must not appear unless removeUnused is set")
	}
	if !strings.Contains(hostUpdateApplyScript(hostUpgradeOptions{RemoveUnused: true}), "apt-get -y autoremove") {
		t.Fatal("removeUnused did not reach the apt cleanup")
	}
}

func TestHostUpdatePayloadToleratesUnknownFields(t *testing.T) {
	var msg WSMessage
	raw := `{"type":"host_update_apply","id":"x","hostUpdate":{"withNewPackages":true,"skipUnavailable":true,"removeUnused":true,"someFutureOption":1}}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("an agent must ignore fields it does not know: %v", err)
	}
	if msg.HostUpdate == nil || !msg.HostUpdate.SkipUnavailable || !msg.HostUpdate.WithNewPackages || !msg.HostUpdate.RemoveUnused {
		t.Fatalf("options lost in decoding: %+v", msg.HostUpdate)
	}
}

// fakeManager is a package-manager stand-in: it records every call, and fails
// the first failTimes upgrade calls with failText.
const fakeManager = `#!/bin/sh
echo "$(basename "$0") $*" >> "$MCH_CALLS"
case " $* " in
  *" -Qdtq "*) echo orphan-pkg; exit 0 ;;
  *" autoremove "*|*" -Rns "*) exit "${MCH_CLEANUP_RC:-0}" ;;
esac
if echo " $* " | grep -Eq "$MCH_UPGRADE"; then
  n=$(cat "$MCH_COUNT" 2>/dev/null || echo 0)
  n=$((n + 1))
  echo "$n" > "$MCH_COUNT"
  if [ "$n" -le "$MCH_FAIL_TIMES" ]; then
    echo "$MCH_FAIL_TEXT"
    exit 100
  fi
  echo upgraded
fi
exit 0
`

func TestHostUpgradeBodyRetriesOnceOnDownloadFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the generated POSIX script against stub binaries; linux only")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}

	managers := []struct {
		manager, binary, upgrade, downloadErr, otherErr string
	}{
		{"apt", "apt-get", " upgrade ", "E: Unable to fetch some archives, maybe run apt-get update", "E: Could not get lock /var/lib/dpkg/lock-frontend"},
		{"dnf", "dnf", " upgrade ", "Error downloading packages:", "Problem: conflicting requests"},
		{"yum", "yum", " update ", "Cannot download repomd.xml", "Error: Package: foo requires bar"},
		{"apk", "apk", " upgrade ", "fetch http://mirror/APKINDEX.tar.gz failed", "ERROR: unable to select packages:"},
		{"pacman", "pacman", " -Syu | -Su ", "error: failed retrieving file x.pkg.tar.zst", "error: failed to commit transaction (conflicting files)"},
		{"zypper", "zypper", " update ", "Download (curl) error for https://mirror/repo", "Problem: nothing provides libfoo"},
	}
	scenarios := []struct {
		name       string
		failTimes  int
		useOther   bool
		wantRC     int
		wantCalls  int
		wantNotice bool
	}{
		{"succeeds first time", 0, false, 0, 1, false},
		{"download failure then success", 1, false, 0, 2, true},
		{"download failure twice retries only once", 5, false, 100, 2, true},
		{"other failure never retries", 5, true, 100, 1, false},
	}

	for _, m := range managers {
		for _, sc := range scenarios {
			t.Run(m.manager+"/"+sc.name, func(t *testing.T) {
				dir := t.TempDir()
				bin := filepath.Join(dir, "bin")
				if err := os.Mkdir(bin, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, tool := range []string{"mktemp", "rm", "tee", "cat", "grep", "env", "basename"} {
					realPath, err := exec.LookPath(tool)
					if err != nil {
						t.Skipf("%s missing", tool)
					}
					if err := os.Symlink(realPath, filepath.Join(bin, tool)); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(bin, m.binary), []byte(fakeManager), 0o755); err != nil {
					t.Fatal(err)
				}
				failText := m.downloadErr
				if sc.useOther {
					failText = m.otherErr
				}
				calls := filepath.Join(dir, "calls")
				cmd := exec.Command(shell, "-c", hostUpgradeBody(hostUpgradeOptions{SkipUnavailable: true}))
				cmd.Env = []string{
					"PATH=" + bin,
					"TMPDIR=" + dir,
					"MCH_CALLS=" + calls,
					"MCH_COUNT=" + filepath.Join(dir, "count"),
					"MCH_UPGRADE=" + m.upgrade,
					"MCH_FAIL_TIMES=" + string(rune('0'+sc.failTimes)),
					"MCH_FAIL_TEXT=" + failText,
				}
				out, runErr := cmd.CombinedOutput()
				rc := 0
				var exitErr *exec.ExitError
				if errors.As(runErr, &exitErr) {
					rc = exitErr.ExitCode()
				} else if runErr != nil {
					t.Fatalf("running script: %v", runErr)
				}
				if rc != sc.wantRC {
					t.Errorf("exit code %d, want %d\n%s", rc, sc.wantRC, out)
				}
				if !strings.Contains(string(out), "MCHARBOR_MANAGER="+m.manager) {
					t.Errorf("wrong branch taken:\n%s", out)
				}
				if got := strings.Contains(string(out), hostUpgradeRetryNotice); got != sc.wantNotice {
					t.Errorf("retry notice present=%v, want %v\n%s", got, sc.wantNotice, out)
				}
				count, readErr := os.ReadFile(filepath.Join(dir, "count"))
				if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("reading upgrade count: %v", readErr)
				}
				if got := strings.TrimSpace(string(count)); got != string(rune('0'+sc.wantCalls)) {
					t.Errorf("upgrade ran %s times, want %d\n%s", got, sc.wantCalls, out)
				}
			})
		}
	}
}

func TestHostUpgradeBodyRemovesUnusedOnlyAfterSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the generated POSIX script against stub binaries; linux only")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}

	managers := []struct {
		manager, binary, upgrade, cleanup string
	}{
		{"apt", "apt-get", " upgrade ", "apt-get -y autoremove"},
		{"dnf", "dnf", " upgrade ", "dnf -y autoremove"},
		{"yum", "yum", " update ", "yum -y autoremove"},
		{"pacman", "pacman", " -Syu | -Su ", "pacman -Rns --noconfirm orphan-pkg"},
		{"apk", "apk", " upgrade ", ""},
		{"zypper", "zypper", " update ", ""},
	}
	scenarios := []struct {
		name      string
		failTimes int
		cleanupRC int
		withOpt   bool
		wantRC    int
		wantRan   bool
	}{
		{"upgrade ok, option on", 0, 0, true, 0, true},
		{"upgrade ok, option off", 0, 0, false, 0, false},
		{"download failure then ok, option on", 1, 0, true, 0, true},
		{"upgrade fails, option on", 5, 0, true, 100, false},
		{"cleanup fails, its rc is returned", 0, 7, true, 7, true},
	}

	for _, m := range managers {
		for _, sc := range scenarios {
			t.Run(m.manager+"/"+sc.name, func(t *testing.T) {
				dir := t.TempDir()
				bin := filepath.Join(dir, "bin")
				if err := os.Mkdir(bin, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, tool := range []string{"mktemp", "rm", "tee", "cat", "grep", "env", "basename"} {
					realPath, err := exec.LookPath(tool)
					if err != nil {
						t.Skipf("%s missing", tool)
					}
					if err := os.Symlink(realPath, filepath.Join(bin, tool)); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(bin, m.binary), []byte(fakeManager), 0o755); err != nil {
					t.Fatal(err)
				}
				calls := filepath.Join(dir, "calls")
				cmd := exec.Command(shell, "-c", hostUpgradeBody(hostUpgradeOptions{RemoveUnused: sc.withOpt}))
				cmd.Env = []string{
					"PATH=" + bin,
					"TMPDIR=" + dir,
					"MCH_CALLS=" + calls,
					"MCH_COUNT=" + filepath.Join(dir, "count"),
					"MCH_UPGRADE=" + m.upgrade,
					"MCH_FAIL_TIMES=" + string(rune('0'+sc.failTimes)),
					"MCH_FAIL_TEXT=Problem: not a download failure",
					"MCH_CLEANUP_RC=" + string(rune('0'+sc.cleanupRC)),
				}
				if sc.failTimes == 1 {
					cmd.Env[6] = "MCH_FAIL_TEXT=" + downloadFailureText(m.manager)
				}
				out, runErr := cmd.CombinedOutput()
				rc := 0
				var exitErr *exec.ExitError
				if errors.As(runErr, &exitErr) {
					rc = exitErr.ExitCode()
				} else if runErr != nil {
					t.Fatalf("running script: %v", runErr)
				}
				wantRC := sc.wantRC
				if sc.cleanupRC != 0 && m.cleanup == "" {
					wantRC = 0
				}
				if rc != wantRC {
					t.Errorf("exit code %d, want %d\n%s", rc, wantRC, out)
				}
				log, readErr := os.ReadFile(calls)
				if readErr != nil {
					t.Fatalf("reading call log: %v", readErr)
				}
				wantRan := sc.wantRan && m.cleanup != ""
				if got := strings.Contains(string(log), m.cleanup) && m.cleanup != ""; got != wantRan {
					t.Errorf("cleanup ran=%v, want %v\ncalls:\n%s\n%s", got, wantRan, log, out)
				}
				if m.cleanup == "" && (strings.Contains(string(log), "autoremove") || strings.Contains(string(log), "-Rns")) {
					t.Errorf("a manager with no cleanup ran one:\n%s", log)
				}
				if got := strings.Contains(string(out), hostRemoveUnusedNotice); got != wantRan {
					t.Errorf("cleanup notice present=%v, want %v\n%s", got, wantRan, out)
				}
				wantSkip := sc.wantRan && sc.withOpt && m.cleanup == ""
				if got := strings.Contains(string(out), hostRemoveUnusedSkipNotice); got != wantSkip {
					t.Errorf("skip notice present=%v, want %v\n%s", got, wantSkip, out)
				}
			})
		}
	}
}

// downloadFailureText is output the manager's retry pattern matches, so the
// script refreshes and upgrades a second time.
func downloadFailureText(manager string) string {
	switch manager {
	case "apt":
		return "E: Unable to fetch some archives"
	case "dnf", "yum":
		return "Error downloading packages:"
	case "apk":
		return "fetch http://mirror/APKINDEX.tar.gz failed"
	case "pacman":
		return "error: failed retrieving file x.pkg.tar.zst"
	}
	return "Download (curl) error for https://mirror/repo"
}
