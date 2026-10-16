// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package supervisor runs compute guests as systemd transient units.
//
// This is the single most consequential reliability decision in the compute
// plan. A guest started as a child of the agent process dies when the agent
// dies — including on the agent's own self-update path, which replaces the
// running binary. A guest started as a transient unit is supervised by the
// host's init, survives all of that, and brings its cgroup accounting with it.
//
// Concretely, going through the system bus rather than shelling out to
// `systemd-run` buys three things:
//
//   - Structured results. StartTransientUnit returns a job object and a
//     completion channel; a start failure is a typed D-Bus error rather than a
//     stderr string whose wording changes between systemd versions.
//   - Structured reconciliation. ListUnitsByPatterns returns load state, active
//     state and sub-state as fields, so adopting running guests after an agent
//     restart is not `systemctl list-units` output parsing.
//   - Push, not poll. Unit state changes arrive as they happen, so a QEMU
//     process the OOM killer took surfaces immediately rather than at the next
//     sampling interval.
//
// `systemd-run` remains the documented fallback for a host where the agent
// cannot reach the system bus, and the capability probe reports which path is
// in use so the operator is never guessing.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UnitPrefix namespaces every unit this package creates. Nothing McHarbor puts
// on a host is unnamed or unfindable: an operator can always run
// `systemctl list-units 'mcharbor-*'` and see exactly what is there.
const UnitPrefix = "mcharbor-"

// VMUnitPattern and CTUnitPattern are the reconciliation globs. Units matching
// these that the backend has no row for are reported as orphans in the UI with
// adopt-or-stop actions — never silently killed, because the operator may have
// a very good reason for them.
const (
	VMUnitPattern = UnitPrefix + "vm-*.service"
	CTUnitPattern = UnitPrefix + "ct-*.service"
)

// ErrUnavailable means the system bus could not be reached. The caller falls
// back to the systemd-run path and the capability probe reports the downgrade.
var ErrUnavailable = errors.New("supervisor: systemd system bus unavailable")

// ErrNoUnit means the named unit does not exist.
var ErrNoUnit = errors.New("supervisor: unit not found")

// VMUnitName returns the transient unit name for a VM id.
func VMUnitName(vmID string) string { return UnitPrefix + "vm-" + vmID + ".service" }

// CTUnitName returns the transient unit name for a container id.
func CTUnitName(ctID string) string { return UnitPrefix + "ct-" + ctID + ".service" }

// GuestIDFromUnit recovers the guest id from a unit name, and reports whether
// the name was one of ours at all. Used when reconciling a unit list back
// against the backend's rows.
func GuestIDFromUnit(unit string) (kind string, id string, ok bool) {
	name := strings.TrimSuffix(unit, ".service")
	switch {
	case strings.HasPrefix(name, UnitPrefix+"vm-"):
		return "vm", strings.TrimPrefix(name, UnitPrefix+"vm-"), true
	case strings.HasPrefix(name, UnitPrefix+"ct-"):
		return "ct", strings.TrimPrefix(name, UnitPrefix+"ct-"), true
	default:
		return "", "", false
	}
}

// UnitStatus is one unit's state as systemd reports it.
type UnitStatus struct {
	Name string
	// LoadState is loaded | not-found | error.
	LoadState string
	// ActiveState is active | activating | deactivating | inactive | failed.
	ActiveState string
	// SubState is the type-specific state: running, exited, dead, failed.
	SubState string
	// MainPID is the supervised process, 0 when not running.
	MainPID int
	// Result is systemd's verdict on a unit that stopped: success, oom-kill,
	// timeout, signal, exit-code. This is what turns "the VM died" into "the
	// OOM killer took it", which is the difference between a useful and a
	// useless error in the UI.
	Result string
	// CPUUsageNSec and MemoryCurrent come from the unit's cgroup accounting,
	// read as unit properties rather than by constructing /sys/fs/cgroup
	// paths that differ between distributions.
	CPUUsageNSec  uint64
	MemoryCurrent uint64
	// ActiveEnterTimestamp is when the unit last became active, in
	// microseconds since the epoch.
	ActiveEnterTimestamp uint64
}

// Running reports whether the unit is up.
func (u UnitStatus) Running() bool {
	return u.ActiveState == "active" && u.SubState == "running"
}

// Failed reports whether the unit stopped abnormally.
func (u UnitStatus) Failed() bool {
	return u.ActiveState == "failed" || (u.Result != "" && u.Result != "success")
}

// StartedAt renders ActiveEnterTimestamp as a time, zero when unset.
func (u UnitStatus) StartedAt() time.Time {
	if u.ActiveEnterTimestamp == 0 {
		return time.Time{}
	}
	return time.UnixMicro(int64(u.ActiveEnterTimestamp))
}

// UnitSpec describes a transient unit to create.
type UnitSpec struct {
	Name        string
	Description string
	// Argv is the command and its arguments. It is passed to systemd as an
	// argument vector, never as a shell string, so a guest name containing a
	// space or a semicolon cannot become a second command.
	Argv []string
	// WorkingDirectory the process starts in, usually the guest's runtime dir.
	WorkingDirectory string
	// Environment entries as KEY=VALUE.
	Environment []string
	// KillSignal overrides SIGTERM. A container running systemd needs
	// SIGRTMIN+3, because SIGTERM to systemd's PID 1 does not shut it down
	// cleanly and is what produces the "takes 90 seconds to stop" complaint.
	KillSignal int
	// KillMode is normally "mixed": signal the main process first, then the
	// rest of the cgroup.
	KillMode string
	// TimeoutStopSec bounds the graceful stop before systemd escalates to
	// SIGKILL.
	TimeoutStopSec uint64
	// Slice places the unit under a parent slice, which is how a container's
	// cgroup ends up delegated by systemd rather than by McHarbor writing
	// cgroup.subtree_control and hoping the ordering is right.
	Slice string
	// Delegate hands the unit's cgroup subtree to the payload. Required for a
	// container running systemd inside.
	Delegate bool
	// Extra properties passed through untouched, for the per-guest cgroup
	// limits the caller computes.
	CPUQuotaPercent uint64
	MemoryMax       uint64
	MemoryHigh      uint64
	TasksMax        uint64
}

// Supervisor starts, stops and inspects transient units.
//
// It is an interface so the systemd-run fallback and the non-Linux stub satisfy
// the same shape, and so the guest lifecycle code never branches on which one
// it got.
type Supervisor interface {
	// Start creates and starts a transient unit, waiting for its start job.
	Start(ctx context.Context, spec UnitSpec) error
	// Stop stops a unit, waiting for its stop job. Stopping a unit that is
	// already gone is not an error — the desired state is reached either way.
	Stop(ctx context.Context, unitName string) error
	// Kill sends a signal to a unit's processes without waiting.
	Kill(ctx context.Context, unitName string, signal int32) error
	// Status reads one unit's state. Returns ErrNoUnit when it does not exist.
	Status(ctx context.Context, unitName string) (UnitStatus, error)
	// List returns every unit matching the glob, for reconciliation.
	List(ctx context.Context, pattern string) ([]UnitStatus, error)
	// Reset clears a failed unit's state so the name can be reused.
	Reset(ctx context.Context, unitName string) error
	// Subscribe delivers unit state changes as they happen. The returned
	// channel is closed when ctx is cancelled, which is this goroutine's
	// stop path.
	Subscribe(ctx context.Context, pattern string) (<-chan UnitStatus, error)
	// Mode reports "dbus" or "systemd-run", for the capability probe.
	Mode() string
	// Close releases the bus connection.
	Close() error
}

// ValidateUnitName rejects anything that is not a unit this package created.
//
// Every caller reaches Stop and Kill with an id that ultimately came from the
// backend, and a unit name is passed to systemd, so this is the boundary where
// a crafted id must not become a way to stop an unrelated system service.
func ValidateUnitName(unit string) error {
	if !strings.HasPrefix(unit, UnitPrefix) {
		return fmt.Errorf("supervisor: refusing unit outside the %s namespace: %q", UnitPrefix, unit)
	}
	if !strings.HasSuffix(unit, ".service") {
		return fmt.Errorf("supervisor: refusing non-service unit: %q", unit)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(unit, UnitPrefix), ".service")
	if body == "" {
		return fmt.Errorf("supervisor: empty unit name")
	}
	for _, r := range body {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
		default:
			return fmt.Errorf("supervisor: refusing unit name with %q: %q", r, unit)
		}
	}
	return nil
}
