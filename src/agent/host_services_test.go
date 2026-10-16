// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateHostServiceUnit(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"nginx.service", true},
		{"getty@tty1.service", true},
		{"foo@.service", true},
		{"dev-disk-by\\x2duuid.service", false},
		{"a_b-c:d.service", true},
		{"", false},
		{"nginx", false},
		{"nginx.socket", false},
		{"nginx.service.d", false},
		{"../etc/passwd.service", false},
		{"..service", false},
		{"a/../../b.service", false},
		{"a\\b.service", false},
		{"my unit.service", false},
		{"nginx.service ", false},
		{"-H.service", false},
		{"--now.service", false},
		{".hidden.service", false},
		{"nginx.service\n", false},
		{strings.Repeat("a", 260) + ".service", false},
	}
	for _, c := range cases {
		err := validateHostServiceUnit(c.name)
		if (err == nil) != c.ok {
			t.Errorf("validateHostServiceUnit(%q) = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestHostServiceGuardRefusesStopAndDisableOnProtectedUnits(t *testing.T) {
	protected := []string{
		"mcharbor-agent.service", "mcharbor-agent-dev.service", "ssh.service", "sshd.service",
		"systemd-journald.service", "systemd-networkd.service", "dbus.service", "dbus-broker.service",
		"docker.service", "containerd.service",
	}
	for _, unit := range protected {
		for _, action := range []string{HostServiceActionStop, HostServiceActionDisable, "mask"} {
			if err := hostServiceActionAllowed(unit, action); err == nil {
				t.Errorf("%s %s was allowed, want refused", action, unit)
			}
		}
		for _, action := range []string{HostServiceActionStart, HostServiceActionRestart, HostServiceActionEnable} {
			if err := hostServiceActionAllowed(unit, action); err != nil {
				t.Errorf("%s %s was refused: %v", action, unit, err)
			}
		}
	}
	for _, unit := range []string{"nginx.service", "cron.service", "sshguard.service", "dockerd-rootless.service"} {
		if err := hostServiceActionAllowed(unit, HostServiceActionStop); err != nil {
			t.Errorf("stop %s was refused: %v", unit, err)
		}
	}
}

// fakeRunner records every command and answers from a script keyed by the
// joined argv, so a test proves what reached systemctl and what never did.
type fakeRunner struct {
	calls   []string
	answers map[string]fakeAnswer
}

type fakeAnswer struct {
	out string
	err error
}

func (f *fakeRunner) run(_ context.Context, _ time.Duration, name string, args ...string) ([]byte, bool, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	for prefix, answer := range f.answers {
		if strings.HasPrefix(key, prefix) {
			return []byte(answer.out), false, answer.err
		}
	}
	return nil, false, nil
}

func TestExecuteRefusesProtectedStopWithoutRunningSystemctl(t *testing.T) {
	runner := &fakeRunner{}
	e := &hostServiceExecutor{run: runner.run, base: t.TempDir()}
	reply := e.execute(context.Background(), HostServicePayload{Op: HostServiceOpAction, Unit: "sshd.service", Action: HostServiceActionStop})
	if reply.Success || reply.ErrorCode != hostServiceErrProtected {
		t.Fatalf("reply = %+v, want protected refusal", reply)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("a refused stop reached the host: %v", runner.calls)
	}
}

func TestExecuteRefusesInvalidUnitsBeforeAnyCommand(t *testing.T) {
	runner := &fakeRunner{}
	e := &hostServiceExecutor{run: runner.run, base: t.TempDir()}
	for _, op := range []string{HostServiceOpAction, HostServiceOpOverrideGet, HostServiceOpOverridePut, HostServiceOpOverrideDel} {
		reply := e.execute(context.Background(), HostServicePayload{Op: op, Unit: "../../etc/shadow.service", Action: HostServiceActionStart, Content: "[Service]\n"})
		if reply.Success || reply.ErrorCode != hostServiceErrInvalidUnit {
			t.Errorf("%s: reply = %+v, want invalid_unit", op, reply)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("an invalid unit reached the host: %v", runner.calls)
	}
}

func TestExecuteAgentRestartIsDeferredWithAWarning(t *testing.T) {
	runner := &fakeRunner{}
	e := &hostServiceExecutor{run: runner.run, base: t.TempDir()}
	reply := e.execute(context.Background(), HostServicePayload{Op: HostServiceOpAction, Unit: "mcharbor-agent.service", Action: HostServiceActionRestart})
	if !reply.Success || reply.Warning != HostServiceWarningAgentRestart {
		t.Fatalf("reply = %+v, want success with the agent-restart warning", reply)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("the agent restart ran before the reply could be sent: %v", runner.calls)
	}
}

func TestExecuteActionPassesTheUnitAfterDoubleDash(t *testing.T) {
	runner := &fakeRunner{}
	e := &hostServiceExecutor{run: runner.run, base: t.TempDir()}
	reply := e.execute(context.Background(), HostServicePayload{Op: HostServiceOpAction, Unit: "nginx.service", Action: HostServiceActionRestart})
	if !reply.Success {
		t.Fatalf("reply = %+v", reply)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "systemctl restart -- nginx.service" {
		t.Fatalf("calls = %v", runner.calls)
	}
}

func TestExecuteReportsSystemctlOutputOnFailure(t *testing.T) {
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl start": {out: "Job for nginx.service failed because the control process exited with error code.", err: errors.New("exit status 1")},
	}}
	e := &hostServiceExecutor{run: runner.run, base: t.TempDir()}
	reply := e.execute(context.Background(), HostServicePayload{Op: HostServiceOpAction, Unit: "nginx.service", Action: HostServiceActionStart})
	if reply.Success || reply.ErrorCode != hostServiceErrFailed || !strings.Contains(reply.Error, "control process exited") {
		t.Fatalf("reply = %+v", reply)
	}
}

const listUnitsJSON = `[{"unit":"cron.service","load":"loaded","active":"active","sub":"running","description":"Regular background program processing daemon"},
{"unit":"nginx.service","load":"loaded","active":"failed","sub":"failed","description":"A high performance web server"},
{"unit":"dev-sda.device","load":"loaded","active":"active","sub":"plugged","description":"disk"}]`

const listUnitsPlain = `cron.service    loaded active running Regular background program processing daemon
● nginx.service loaded failed failed  A high performance web server
ssh.service     loaded active running OpenBSD Secure Shell server
`

const unitFilesJSON = `[{"unit_file":"cron.service","state":"enabled","preset":"enabled"},
{"unit_file":"nginx.service","state":"disabled","preset":"enabled"},
{"unit_file":"getty@.service","state":"static","preset":null},
{"unit_file":"bluetooth.service","state":"masked","preset":"enabled"}]`

const unitFilesPlain = `cron.service      enabled  enabled
nginx.service     disabled enabled
getty@.service    static   -
ssh.service       enabled
`

const showBlocks = `Id=cron.service
MainPID=611
Result=success
StatusText=
ExecMainStatus=0
ExecMainCode=0

Id=nginx.service
MainPID=0
Result=exit-code
StatusText=
ExecMainStatus=1
ExecMainCode=1
`

func TestParseListUnits(t *testing.T) {
	cases := []struct {
		name  string
		parse func() []HostServiceUnit
	}{
		{"json", func() []HostServiceUnit {
			units, err := parseListUnitsJSON([]byte(listUnitsJSON))
			if err != nil {
				t.Fatal(err)
			}
			return units
		}},
		{"plain", func() []HostServiceUnit { return parseListUnitsPlain([]byte(listUnitsPlain)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			units := c.parse()
			byName := map[string]HostServiceUnit{}
			for _, unit := range units {
				byName[unit.Name] = unit
			}
			if _, ok := byName["dev-sda.device"]; ok {
				t.Error("a non-service unit was listed")
			}
			nginx, ok := byName["nginx.service"]
			if !ok || nginx.ActiveState != "failed" || nginx.SubState != "failed" || nginx.LoadState != "loaded" || nginx.Description != "A high performance web server" {
				t.Errorf("nginx = %+v", nginx)
			}
			cron := byName["cron.service"]
			if cron.ActiveState != "active" || cron.SubState != "running" {
				t.Errorf("cron = %+v", cron)
			}
		})
	}
	if _, err := parseListUnitsJSON([]byte("UNIT LOAD ACTIVE SUB DESCRIPTION\ncron.service loaded active running x\n")); err == nil {
		t.Error("an old systemctl's table parsed as JSON; the plain fallback would never run")
	}
}

func TestParseUnitFiles(t *testing.T) {
	fromJSON, err := parseUnitFilesJSON([]byte(unitFilesJSON))
	if err != nil {
		t.Fatal(err)
	}
	for name, files := range map[string]map[string]string{"json": fromJSON, "plain": parseUnitFilesPlain([]byte(unitFilesPlain))} {
		if files["cron.service"] != "enabled" || files["nginx.service"] != "disabled" || files["getty@.service"] != "static" {
			t.Errorf("%s: files = %v", name, files)
		}
	}
	if fromJSON["bluetooth.service"] != "masked" {
		t.Errorf("masked unit lost: %v", fromJSON)
	}
}

func TestMergeHostServicesCarriesPIDFailureAndInstalledFiles(t *testing.T) {
	units, err := parseListUnitsJSON([]byte(listUnitsJSON))
	if err != nil {
		t.Fatal(err)
	}
	files, err := parseUnitFilesJSON([]byte(unitFilesJSON))
	if err != nil {
		t.Fatal(err)
	}
	merged := mergeHostServices(units, files, parseShowBlocks([]byte(showBlocks)))
	byName := map[string]HostServiceUnit{}
	for _, unit := range merged {
		byName[unit.Name] = unit
	}
	if got := byName["cron.service"]; got.MainPID != 611 || got.UnitFileState != "enabled" || got.FailureReason != "" {
		t.Errorf("cron = %+v", got)
	}
	nginx := byName["nginx.service"]
	if nginx.Result != "exit-code" || !strings.Contains(nginx.FailureReason, "exit-code") || !strings.Contains(nginx.FailureReason, "status 1") {
		t.Errorf("nginx failure = %+v", nginx)
	}
	if getty := byName["getty@.service"]; !getty.Template || getty.ActiveState != "inactive" || getty.UnitFileState != "static" {
		t.Errorf("an installed, unloaded template = %+v", getty)
	}
	if merged[0].Name > merged[len(merged)-1].Name {
		t.Error("units are not sorted by name")
	}
}

func TestListHostServicesFallsBackToPlainOutput(t *testing.T) {
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl list-units --type=service --all --output=json":      {out: "UNIT LOAD ACTIVE SUB DESCRIPTION\n"},
		"systemctl list-units --type=service --all --plain":            {out: listUnitsPlain},
		"systemctl list-unit-files --type=service --output=json":       {err: errors.New("unknown option")},
		"systemctl list-unit-files --type=service --plain --no-legend": {out: unitFilesPlain},
		"systemctl show": {out: showBlocks},
	}}
	units, _, err := listHostServices(context.Background(), runner.run)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]HostServiceUnit{}
	for _, unit := range units {
		names[unit.Name] = unit
	}
	if names["ssh.service"].UnitFileState != "enabled" || names["nginx.service"].ActiveState != "failed" || names["cron.service"].MainPID != 611 {
		t.Fatalf("units = %+v", units)
	}
}

func TestWriteOverrideIsAtomicAndRevertRemovesTheDirectory(t *testing.T) {
	base := t.TempDir()
	var chowned []string
	chown := func(path string) error {
		chowned = append(chowned, path)
		return nil
	}
	path, err := writeHostServiceOverride(base, "nginx.service", "[Service]\nRestart=always\n", chown)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "nginx.service.d", "override.conf"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "[Service]\nRestart=always\n" {
		t.Fatalf("content = %q, %v", data, err)
	}
	if len(chowned) != 1 || filepath.Dir(chowned[0]) != filepath.Dir(path) || chowned[0] == path {
		t.Fatalf("chown must run on the temp file before the rename, got %v", chowned)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("mode = %v, %v; want 0644", info.Mode().Perm(), err)
		}
	}

	if _, err := writeHostServiceOverride(base, "nginx.service", "[Service]\nRestart=no\n", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}

	removed, err := removeHostServiceOverride(base, "nginx.service")
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(base, "nginx.service.d")); !os.IsNotExist(err) {
		t.Fatalf("the empty drop-in directory survived: %v", err)
	}
	if removed, err := removeHostServiceOverride(base, "nginx.service"); err != nil || removed {
		t.Fatalf("second remove = %v, %v; want a quiet no-op", removed, err)
	}
}

func TestRevertKeepsOtherDropIns(t *testing.T) {
	base := t.TempDir()
	if _, err := writeHostServiceOverride(base, "nginx.service", "[Service]\n", nil); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(base, "nginx.service.d", "10-vendor.conf")
	if err := os.WriteFile(other, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := removeHostServiceOverride(base, "nginx.service"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("someone else's drop-in was removed: %v", err)
	}
}

func TestWriteOverrideRefusesBadInput(t *testing.T) {
	base := t.TempDir()
	for _, c := range []struct{ unit, content string }{
		{"../escape.service", "[Service]\n"},
		{"nginx.service", strings.Repeat("x", hostServiceOverrideMaxBytes+1)},
		{"nginx.service", "bad\x00bytes"},
	} {
		if _, err := writeHostServiceOverride(base, c.unit, c.content, nil); err == nil {
			t.Errorf("write(%q, %d bytes) was accepted", c.unit, len(c.content))
		}
	}
	if entries, err := os.ReadDir(base); err != nil || len(entries) != 0 {
		t.Fatalf("a refused write left files: %v", entries)
	}
}

func TestExecuteOverridePutReloadsAndReportsVerify(t *testing.T) {
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl show -p FragmentPath": {out: "/lib/systemd/system/nginx.service\n"},
		"systemd-analyze verify":         {out: "nginx.service: Unknown key name 'Restartt'", err: errors.New("exit status 1")},
	}}
	e := &hostServiceExecutor{run: runner.run, base: t.TempDir()}
	reply := e.execute(context.Background(), HostServicePayload{Op: HostServiceOpOverridePut, Unit: "nginx.service", Content: "[Service]\nRestartt=always\n", Reload: true, Verify: true})
	if !reply.Success || !reply.Reloaded || !strings.Contains(reply.VerifyOutput, "Unknown key") {
		t.Fatalf("reply = %+v; verify findings must warn, not refuse", reply)
	}
	if runner.calls[0] != "systemctl daemon-reload" {
		t.Fatalf("calls = %v", runner.calls)
	}
}
