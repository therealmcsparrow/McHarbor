// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"strings"
	"testing"
)

// goodDaemon is a host that satisfies every daemon control the pack checks. Each
// case below changes exactly one field, so a failure names the field.
func goodDaemon() Inputs {
	return Inputs{
		DaemonKnown: true,
		Daemon: Daemon{
			ServerVersion:   "28.5.2",
			StorageDriver:   "overlay2",
			Debug:           false,
			LoggingDriver:   "syslog",
			LiveRestore:     true,
			Experimental:    false,
			SecurityOptions: []string{"name=seccomp,profile=builtin", "name=userns", "name=apparmor"},
			AuthzPlugins:    []string{"authz-broker"},
			SwarmState:      "inactive",
			BridgeICCKnown:  true,
			BridgeICC:       false,
			ConnectionType:  "socket",
		},
		ContainersKnown: true,
		Containers:      []Container{goodContainer()},
	}
}

// goodContainer satisfies every container control the pack checks.
func goodContainer() Container {
	return Container{
		Name:            "web",
		User:            "app",
		HasHealthcheck:  true,
		Privileged:      false,
		NetworkMode:     "bridge",
		MemoryLimit:     512 << 20,
		CPUShares:       512,
		ReadonlyRootfs:  true,
		RestartPolicy:   "on-failure",
		RestartMaxRetry: 5,
		PIDMode:         "",
		IPCMode:         "private",
		UTSMode:         "",
		SecurityOpt:     []string{"no-new-privileges=true"},
		PIDsLimit:       100,
	}
}

func statusOf(t *testing.T, r Result) Status {
	t.Helper()
	if r.ID == "" {
		t.Fatal("result carries no control ID")
	}
	return r.Status
}

func hasReason(r Result, code string) bool {
	for _, c := range r.Reasons {
		if c == code {
			return true
		}
	}
	return false
}

// TestDaemonChecksPassOnAGoodHost asserts the good fixture really is good: a
// fixture that quietly fails a control would make every "bad" case below
// meaningless.
func TestDaemonChecksPassOnAGoodHost(t *testing.T) {
	in := goodDaemon()
	for _, c := range cisControls() {
		if c.daemonCheck == nil {
			continue
		}
		got := statusOf(t, c.daemonCheck(in))
		if got == StatusFail || got == StatusWarn || got == StatusUnknown {
			t.Errorf("control %s on a good host: got %q, reasons %v", c.ID, got, c.daemonCheck(in).Reasons)
		}
	}
}

func TestContainerChecksPassOnAGoodContainer(t *testing.T) {
	in := goodDaemon()
	for _, c := range cisControls() {
		if c.containerCheck == nil {
			continue
		}
		res := c.containerCheck(in)
		if got := statusOf(t, res); got != StatusPass {
			t.Errorf("control %s on a good container: got %q, reasons %v", c.ID, got, res.Reasons)
		}
	}
}

// TestEveryDaemonCheckIsUnknownWhenTheDaemonIsUnreachable is the assertion that
// matters most in this file: an unreachable daemon must never read as a pass, and
// the unknown must carry a reason so the operator knows why.
func TestEveryDaemonCheckIsUnknownWhenTheDaemonIsUnreachable(t *testing.T) {
	unreachable := Inputs{DaemonKnown: false, ContainersKnown: false}
	for _, c := range cisControls() {
		if c.daemonCheck == nil {
			continue
		}
		res := c.daemonCheck(unreachable)
		if res.Status != StatusUnknown {
			t.Errorf("control %s with an unreachable daemon: got %q, want unknown", c.ID, res.Status)
		}
		if !hasReason(res, "daemonUnavailable") {
			t.Errorf("control %s with an unreachable daemon: reasons %v, want daemonUnavailable", c.ID, res.Reasons)
		}
	}
}

func TestEveryContainerCheckIsUnknownWhenTheListIsUnreachable(t *testing.T) {
	unreachable := Inputs{DaemonKnown: true, ContainersKnown: false}
	for _, c := range cisControls() {
		if c.containerCheck == nil {
			continue
		}
		res := c.containerCheck(unreachable)
		if res.Status != StatusUnknown {
			t.Errorf("control %s with an unreadable container list: got %q, want unknown", c.ID, res.Status)
		}
		if !hasReason(res, "containersUnavailable") {
			t.Errorf("control %s with an unreadable container list: reasons %v, want containersUnavailable", c.ID, res.Reasons)
		}
	}
}

func TestDaemonBadStates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Daemon)
		check  func(Inputs) Result
		want   Status
		reason string
	}{
		{"icc enabled", func(d *Daemon) { d.BridgeICC = true }, checkBridgeICC, StatusFail, "iccEnabled"},
		{"bridge unreadable", func(d *Daemon) { d.BridgeICCKnown = false }, checkBridgeICC, StatusUnknown, "bridgeUnreadable"},
		{"debug logging", func(d *Daemon) { d.Debug = true }, checkDaemonDebug, StatusWarn, "debugLoggingEnabled"},
		{"aufs driver", func(d *Daemon) { d.StorageDriver = "aufs" }, checkStorageDriver, StatusFail, "legacyStorageDriver"},
		{"driver unreported", func(d *Daemon) { d.StorageDriver = "" }, checkStorageDriver, StatusUnknown, "storageDriverUnreported"},
		{"plain tcp daemon", func(d *Daemon) { d.ConnectionType = "tcp" }, checkDaemonTransport, StatusFail, "daemonTCPNoTLS"},
		{"unknown transport", func(d *Daemon) { d.ConnectionType = "" }, checkDaemonTransport, StatusUnknown, "connectionTypeUnknown"},
		{"no userns", func(d *Daemon) { d.SecurityOptions = []string{"name=seccomp,profile=builtin"} }, checkUserNamespace, StatusWarn, "usernsDisabled"},
		{"no authz plugin", func(d *Daemon) { d.AuthzPlugins = nil }, checkAuthorizationPlugin, StatusWarn, "noAuthorizationPlugin"},
		{"local logging", func(d *Daemon) { d.LoggingDriver = "json-file" }, checkRemoteLogging, StatusWarn, "loggingDriverLocal"},
		{"no live restore", func(d *Daemon) { d.LiveRestore = false }, checkLiveRestore, StatusWarn, "liveRestoreDisabled"},
		{"seccomp off", func(d *Daemon) { d.SecurityOptions = []string{"name=seccomp,profile=unconfined"} }, checkDaemonSeccomp, StatusFail, "seccompDisabled"},
		{"seccomp unreported", func(d *Daemon) { d.SecurityOptions = nil }, checkDaemonSeccomp, StatusUnknown, "seccompUnreported"},
		{"experimental build", func(d *Daemon) { d.Experimental = true }, checkExperimental, StatusWarn, "experimentalEnabled"},
		{"version unreported", func(d *Daemon) { d.ServerVersion = "" }, checkEngineVersion, StatusUnknown, "versionUnreported"},
		{"swarm active", func(d *Daemon) { d.SwarmState = "active" }, checkSwarmMode, StatusInfo, "swarmActive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := goodDaemon()
			tc.mutate(&in.Daemon)
			res := tc.check(in)
			if res.Status != tc.want {
				t.Errorf("got %q, want %q (reasons %v)", res.Status, tc.want, res.Reasons)
			}
			if !hasReason(res, tc.reason) {
				t.Errorf("reasons %v, want %s", res.Reasons, tc.reason)
			}
		})
	}
}

func TestContainerBadStates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Container)
		check  func(Inputs) Result
		want   Status
		reason string
	}{
		{"runs as root", func(c *Container) { c.User = "" }, checkContainerUser, StatusWarn, "containersRunAsRoot"},
		{"root by uid", func(c *Container) { c.User = "0:0" }, checkContainerUser, StatusWarn, "containersRunAsRoot"},
		{"no healthcheck", func(c *Container) { c.HasHealthcheck = false }, checkContainerHealthcheck, StatusWarn, "containersWithoutHealthcheck"},
		{"adds capabilities", func(c *Container) { c.CapAdd = []string{"SYS_ADMIN"} }, checkContainerCapAdd, StatusWarn, "containersAddCapabilities"},
		{"privileged", func(c *Container) { c.Privileged = true }, checkContainerPrivileged, StatusFail, "containersPrivileged"},
		{"privileged port", func(c *Container) { c.PrivilegedPorts = []string{"80->80/tcp"} }, checkContainerPrivilegedPorts, StatusWarn, "containersOnPrivilegedPorts"},
		{"host network", func(c *Container) { c.NetworkMode = "host" }, checkContainerHostNetwork, StatusFail, "containersShareHostNetwork"},
		{"no memory limit", func(c *Container) { c.MemoryLimit = 0 }, checkContainerMemory, StatusWarn, "containersWithoutMemoryLimit"},
		{"no cpu limit", func(c *Container) { c.CPUShares, c.NanoCPUs = 0, 0 }, checkContainerCPU, StatusWarn, "containersWithoutCPULimit"},
		{"writable rootfs", func(c *Container) { c.ReadonlyRootfs = false }, checkContainerReadonlyRootfs, StatusWarn, "containersWritableRootfs"},
		{"restart always", func(c *Container) { c.RestartPolicy = "always" }, checkContainerRestartPolicy, StatusWarn, "containersRestartUnbounded"},
		{"restart on-failure unbounded", func(c *Container) { c.RestartMaxRetry = 0 }, checkContainerRestartPolicy, StatusWarn, "containersRestartUnbounded"},
		{"host pid", func(c *Container) { c.PIDMode = "host" }, checkContainerHostPID, StatusFail, "containersShareHostPID"},
		{"host ipc", func(c *Container) { c.IPCMode = "host" }, checkContainerHostIPC, StatusFail, "containersShareHostIPC"},
		{"host device", func(c *Container) { c.DeviceCount = 1 }, checkContainerDevices, StatusWarn, "containersExposeHostDevices"},
		{"shared propagation", func(c *Container) { c.SharedMounts = []string{"/data"} }, checkContainerMountPropagation, StatusWarn, "containersSharedMountPropagation"},
		{"host uts", func(c *Container) { c.UTSMode = "host" }, checkContainerHostUTS, StatusWarn, "containersShareHostUTS"},
		{"seccomp unconfined", func(c *Container) { c.SecurityOpt = []string{"seccomp=unconfined"} }, checkContainerSeccomp, StatusFail, "containersSeccompUnconfined"},
		{"seccomp unconfined colon", func(c *Container) { c.SecurityOpt = []string{"seccomp:unconfined"} }, checkContainerSeccomp, StatusFail, "containersSeccompUnconfined"},
		{"may gain privileges", func(c *Container) { c.SecurityOpt = nil }, checkContainerNoNewPrivileges, StatusWarn, "containersMayGainPrivileges"},
		{"no-new-privileges false", func(c *Container) { c.SecurityOpt = []string{"no-new-privileges=false"} }, checkContainerNoNewPrivileges, StatusWarn, "containersMayGainPrivileges"},
		{"no pids limit", func(c *Container) { c.PIDsLimit = 0 }, checkContainerPIDsLimit, StatusWarn, "containersWithoutPIDsLimit"},
		{"docker socket mounted", func(c *Container) { c.DockerSocketBind = true }, checkContainerDockerSocket, StatusFail, "containersMountDockerSocket"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := goodDaemon()
			c := goodContainer()
			tc.mutate(&c)
			in.Containers = []Container{c}
			res := tc.check(in)
			if res.Status != tc.want {
				t.Errorf("got %q, want %q (reasons %v)", res.Status, tc.want, res.Reasons)
			}
			if !hasReason(res, tc.reason) {
				t.Errorf("reasons %v, want %s", res.Reasons, tc.reason)
			}
			if got := factValue(res, "containersViolating"); got != "1" {
				t.Errorf("containersViolating = %q, want 1", got)
			}
			if names := factValue(res, "violations"); !strings.Contains(names, "web") {
				t.Errorf("violations = %q, want the container name", names)
			}
		})
	}
}

// TestPartialInspectIsNotAPass covers the case that looks most like a pass and is
// not one: the list loaded, no violation was found, and half the inspects failed.
func TestPartialInspectIsNotAPass(t *testing.T) {
	in := goodDaemon()
	in.InspectFailures = 3
	res := checkContainerPrivileged(in)
	if res.Status != StatusUnknown {
		t.Fatalf("got %q, want unknown", res.Status)
	}
	if !hasReason(res, "inspectIncomplete") {
		t.Fatalf("reasons %v, want inspectIncomplete", res.Reasons)
	}
}

func TestNoContainersIsInformationalNotAPass(t *testing.T) {
	in := goodDaemon()
	in.Containers = nil
	res := checkContainerPrivileged(in)
	if res.Status != StatusInfo || !hasReason(res, "noContainers") {
		t.Fatalf("got %q reasons %v, want info/noContainers", res.Status, res.Reasons)
	}
}

// TestWorstRankingKeepsUnknownAboveInfo is the run-level rollup: one control that
// could not be assessed is not a passing run, which is the rule the posture module
// established and every pack inherits.
func TestWorstRankingKeepsUnknownAboveInfo(t *testing.T) {
	results := make([]Result, 0, 10)
	for i := 0; i < 9; i++ {
		results = append(results, Result{ID: "x", Status: StatusPass})
	}
	results = append(results, Result{ID: "y", Status: StatusUnknown})
	if got := Worst(results); got != StatusUnknown {
		t.Fatalf("nine passes and one unknown rolled up to %q, want unknown", got)
	}
	if got := Worst([]Result{{Status: StatusInfo}, {Status: StatusUnknown}}); got != StatusUnknown {
		t.Fatalf("info beside unknown rolled up to %q, want unknown", got)
	}
	if got := Worst([]Result{{Status: StatusUnknown}, {Status: StatusWarn}, {Status: StatusFail}}); got != StatusFail {
		t.Fatalf("fail must win: got %q", got)
	}
}

// TestEvaluateEmitsEveryControl asserts a run lists the controls McHarbor cannot
// check as well as the ones it can. A pack that lists only what it checked reads
// as a clean bill of health.
func TestEvaluateEmitsEveryControl(t *testing.T) {
	f, ok := FrameworkByKey(FrameworkCIS)
	if !ok {
		t.Fatal("the CIS framework is not registered")
	}
	results := Evaluate(f, goodDaemon())
	if len(results) != len(f.Controls) {
		t.Fatalf("got %d results for %d controls", len(results), len(f.Controls))
	}
	byID := map[string]Result{}
	for _, r := range results {
		byID[r.ID] = r
	}
	for _, c := range f.Controls {
		r, ok := byID[c.ID]
		if !ok {
			t.Fatalf("control %s produced no result", c.ID)
		}
		switch c.Coverage {
		case CoverageNotCovered:
			if r.Status != StatusInfo || !hasReason(r, "notCoveredByMcHarbor") {
				t.Errorf("control %s: got %q reasons %v, want info/notCoveredByMcHarbor", c.ID, r.Status, r.Reasons)
			}
		case CoverageEvidenced:
			if r.Status != StatusInfo || !hasReason(r, "evidencedNotChecked") {
				t.Errorf("control %s: got %q reasons %v, want info/evidencedNotChecked", c.ID, r.Status, r.Reasons)
			}
		}
	}
}

// TestEvaluateOrdersControlsNumerically guards the sort: 5.9 before 5.10, which a
// lexical sort gets wrong and which an auditor reading down a table notices.
func TestEvaluateOrdersControlsNumerically(t *testing.T) {
	f, _ := FrameworkByKey(FrameworkCIS)
	results := Evaluate(f, goodDaemon())
	var nine, ten int
	for i, r := range results {
		switch r.ID {
		case "5.9":
			nine = i
		case "5.10":
			ten = i
		}
	}
	if nine >= ten {
		t.Fatalf("5.9 is at %d and 5.10 at %d; control IDs must sort numerically", nine, ten)
	}
}

func factValue(r Result, key string) string {
	for _, f := range r.Facts {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}
