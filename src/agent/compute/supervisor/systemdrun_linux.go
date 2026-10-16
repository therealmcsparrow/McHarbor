// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The systemd-run fallback, used on a host where the agent has systemd but
// cannot reach the system bus — a container with no bus socket bind-mounted, a
// hardened dbus policy, an selinux denial.
//
// It is deliberately a second implementation of the same interface rather than
// a degraded mode inside the D-Bus one. Everything the D-Bus path gets as typed
// fields has to be parsed back out of `systemctl show` output here, and mixing
// the two would make it impossible to tell which behaviour is in play. The
// capability probe reports the mode so the operator can see it too.
//
// What is genuinely lost on this path, and why the D-Bus path is preferred:
// there is no push. Subscribe polls, so a guest the OOM killer takes surfaces
// at the next poll rather than immediately.

// runTimeout bounds each systemctl invocation. Every exec on this path is a
// short query or a job submission; one that hangs is a host problem.
const runTimeout = 30 * time.Second

// pollInterval is how often the fallback Subscribe re-reads unit state.
const pollInterval = 5 * time.Second

type systemdRunSupervisor struct {
	systemdRun string
	systemctl  string
}

var _ Supervisor = (*systemdRunSupervisor)(nil)

// NewSystemdRun builds the CLI fallback, verifying both binaries exist so the
// caller learns now rather than at the first guest start.
func NewSystemdRun() (Supervisor, error) {
	run, err := exec.LookPath("systemd-run")
	if err != nil {
		return nil, fmt.Errorf("%w: systemd-run not on PATH: %w", ErrUnavailable, err)
	}
	ctl, err := exec.LookPath("systemctl")
	if err != nil {
		return nil, fmt.Errorf("%w: systemctl not on PATH: %w", ErrUnavailable, err)
	}
	return &systemdRunSupervisor{systemdRun: run, systemctl: ctl}, nil
}

func (s *systemdRunSupervisor) Mode() string { return "systemd-run" }

func (s *systemdRunSupervisor) Close() error { return nil }

func (s *systemdRunSupervisor) Start(ctx context.Context, spec UnitSpec) error {
	if err := ValidateUnitName(spec.Name); err != nil {
		return err
	}
	if len(spec.Argv) == 0 {
		return fmt.Errorf("supervisor: %s has no command", spec.Name)
	}

	errlog.Log(s.Reset(ctx, spec.Name))

	args := []string{
		"--unit=" + strings.TrimSuffix(spec.Name, ".service"),
		"--collect",
		"--property=Type=exec",
		"--property=CPUAccounting=yes",
		"--property=MemoryAccounting=yes",
		"--property=TasksAccounting=yes",
		"--property=IOAccounting=yes",
	}
	if spec.Description != "" {
		args = append(args, "--description="+spec.Description)
	}
	killMode := spec.KillMode
	if killMode == "" {
		killMode = "mixed"
	}
	args = append(args, "--property=KillMode="+killMode)
	if spec.KillSignal != 0 {
		args = append(args, "--property=KillSignal="+strconv.Itoa(spec.KillSignal))
	}
	if spec.TimeoutStopSec > 0 {
		args = append(args, "--property=TimeoutStopSec="+strconv.FormatUint(spec.TimeoutStopSec, 10))
	}
	if spec.WorkingDirectory != "" {
		args = append(args, "--working-directory="+spec.WorkingDirectory)
	}
	for _, env := range spec.Environment {
		args = append(args, "--setenv="+env)
	}
	if spec.Slice != "" {
		args = append(args, "--slice="+spec.Slice)
	}
	if spec.Delegate {
		args = append(args, "--property=Delegate=yes")
	}
	if spec.CPUQuotaPercent > 0 {
		args = append(args, "--property=CPUQuota="+strconv.FormatUint(spec.CPUQuotaPercent, 10)+"%")
	}
	if spec.MemoryMax > 0 {
		args = append(args, "--property=MemoryMax="+strconv.FormatUint(spec.MemoryMax, 10))
	}
	if spec.MemoryHigh > 0 {
		args = append(args, "--property=MemoryHigh="+strconv.FormatUint(spec.MemoryHigh, 10))
	}
	if spec.TasksMax > 0 {
		args = append(args, "--property=TasksMax="+strconv.FormatUint(spec.TasksMax, 10))
	}

	// The argv is appended after the options and passed as separate exec
	// arguments — never joined into a shell string, so a value containing a
	// space or a semicolon stays one argument.
	args = append(args, "--")
	args = append(args, spec.Argv...)

	if out, err := s.run(ctx, s.systemdRun, args...); err != nil {
		return fmt.Errorf("supervisor: start %s: %w (%s)", spec.Name, err, firstLine(out))
	}
	return nil
}

func (s *systemdRunSupervisor) Stop(ctx context.Context, unitName string) error {
	if err := ValidateUnitName(unitName); err != nil {
		return err
	}
	out, err := s.run(ctx, s.systemctl, "stop", unitName)
	if err != nil {
		if strings.Contains(out, "not loaded") || strings.Contains(out, "not found") {
			return nil
		}
		return fmt.Errorf("supervisor: stop %s: %w (%s)", unitName, err, firstLine(out))
	}
	return nil
}

func (s *systemdRunSupervisor) Kill(ctx context.Context, unitName string, signal int32) error {
	if err := ValidateUnitName(unitName); err != nil {
		return err
	}
	bestEffort(s.run(ctx, s.systemctl, "kill",
		"--signal="+strconv.Itoa(int(signal)), "--kill-who=all", unitName))
	return nil
}

func (s *systemdRunSupervisor) Reset(ctx context.Context, unitName string) error {
	if err := ValidateUnitName(unitName); err != nil {
		return err
	}
	bestEffort(s.run(ctx, s.systemctl, "reset-failed", unitName))
	return nil
}

// showProperties is the property set parsed back out of `systemctl show`. It
// mirrors exactly what the D-Bus path reads as typed fields.
var showProperties = []string{
	"LoadState", "ActiveState", "SubState", "ActiveEnterTimestampMonotonic",
	"ActiveEnterTimestamp", "MainPID", "Result", "CPUUsageNSec", "MemoryCurrent",
}

func (s *systemdRunSupervisor) Status(ctx context.Context, unitName string) (UnitStatus, error) {
	if err := ValidateUnitName(unitName); err != nil {
		return UnitStatus{}, err
	}
	out, err := s.run(ctx, s.systemctl, "show", unitName,
		"--property="+strings.Join(showProperties, ","))
	if err != nil {
		return UnitStatus{}, fmt.Errorf("supervisor: show %s: %w", unitName, err)
	}
	status := parseShow(unitName, out)
	if status.LoadState == "not-found" || status.LoadState == "" {
		return status, ErrNoUnit
	}
	return status, nil
}

func (s *systemdRunSupervisor) List(ctx context.Context, pattern string) ([]UnitStatus, error) {
	out, err := s.run(ctx, s.systemctl, "list-units", "--all", "--no-legend",
		"--plain", "--output=json", pattern)
	if err != nil {
		return nil, fmt.Errorf("supervisor: list %s: %w", pattern, err)
	}

	// --output=json is available on systemd 246+. Older systemd prints a table
	// instead, so fall back to reading the first column rather than failing.
	var rows []struct {
		Unit   string `json:"unit"`
		Load   string `json:"load"`
		Active string `json:"active"`
		Sub    string `json:"sub"`
	}
	names := make([]string, 0, 8)
	if err := json.Unmarshal([]byte(out), &rows); err == nil {
		for _, r := range rows {
			names = append(names, r.Unit)
		}
	} else {
		sc := bufio.NewScanner(strings.NewReader(out))
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) > 0 && strings.HasPrefix(fields[0], UnitPrefix) {
				names = append(names, fields[0])
			}
		}
	}

	result := make([]UnitStatus, 0, len(names))
	for _, name := range names {
		status, err := s.Status(ctx, name)
		if err != nil {
			continue
		}
		result = append(result, status)
	}
	return result, nil
}

// Subscribe polls, because there is no push on this path. The caller gets the
// same channel shape either way; only the latency differs.
func (s *systemdRunSupervisor) Subscribe(ctx context.Context, pattern string) (<-chan UnitStatus, error) {
	out := make(chan UnitStatus, 32)
	go func() {
		defer close(out)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		previous := map[string]UnitStatus{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			current, err := s.List(ctx, pattern)
			if err != nil {
				continue
			}
			seen := map[string]bool{}
			for _, status := range current {
				seen[status.Name] = true
				prev, had := previous[status.Name]
				if had && prev.ActiveState == status.ActiveState &&
					prev.SubState == status.SubState && prev.LoadState == status.LoadState {
					continue
				}
				previous[status.Name] = status
				select {
				case out <- status:
				case <-ctx.Done():
					return
				}
			}
			// A unit that disappeared between polls is reported as gone, so
			// the caller does not keep showing it as running.
			for name := range previous {
				if seen[name] {
					continue
				}
				delete(previous, name)
				select {
				case out <- UnitStatus{Name: name, LoadState: "not-found", ActiveState: "inactive"}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// run executes one systemctl/systemd-run invocation under a timeout, returning
// combined output so a failure can name the reason.
func (s *systemdRunSupervisor) run(ctx context.Context, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// parseShow turns `systemctl show` KEY=VALUE lines into a UnitStatus.
func parseShow(name, out string) UnitStatus {
	status := UnitStatus{Name: name}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "LoadState":
			status.LoadState = value
		case "ActiveState":
			status.ActiveState = value
		case "SubState":
			status.SubState = value
		case "Result":
			status.Result = value
		case "MainPID":
			if n, err := strconv.Atoi(value); err == nil {
				status.MainPID = n
			}
		case "CPUUsageNSec":
			if n, err := strconv.ParseUint(value, 10, 64); err == nil {
				status.CPUUsageNSec = n
			}
		case "MemoryCurrent":
			// systemd prints "[not set]" rather than a number when the unit
			// has no accounting or is not running.
			if n, err := strconv.ParseUint(value, 10, 64); err == nil && n != ^uint64(0) {
				status.MemoryCurrent = n
			}
		case "ActiveEnterTimestamp":
			if t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", value); err == nil {
				status.ActiveEnterTimestamp = uint64(t.UnixMicro())
			}
		}
	}
	return status
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// bestEffort discards the output and error of a systemctl call whose failure is
// not load-bearing: a kill that races the unit exiting on its own, a
// reset-failed on a unit that was never in a failed state. Taking the two
// results as ignored parameters states at the call site that the discard is
// deliberate, which a blank-blank assignment cannot.
func bestEffort(string, error) {}
