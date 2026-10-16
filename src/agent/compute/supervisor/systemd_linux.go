// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	sd "github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// systemdSupervisor drives transient units over the system bus.
type systemdSupervisor struct {
	mu   sync.Mutex
	conn *sd.Conn
}

var _ Supervisor = (*systemdSupervisor)(nil)

// New connects to the systemd system bus.
//
// A failure here is not fatal to compute: the caller falls back to the
// systemd-run path and the capability probe reports the downgrade, so the
// operator sees which mechanism is in use rather than discovering it from
// different failure behaviour.
func New(ctx context.Context) (Supervisor, error) {
	conn, err := sd.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &systemdSupervisor{conn: conn}, nil
}

func (s *systemdSupervisor) Mode() string { return "dbus" }

func (s *systemdSupervisor) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	return nil
}

// properties turns a UnitSpec into the D-Bus property list systemd expects.
func (s *systemdSupervisor) properties(spec UnitSpec) []sd.Property {
	props := []sd.Property{
		sd.PropExecStart(spec.Argv, false),
		sd.PropDescription(spec.Description),
		{Name: "Type", Value: godbus.MakeVariant("exec")},
		// CollectMode inactive-or-failed lets systemd garbage-collect the unit
		// once it stops, so a stopped guest does not leave a failed unit
		// occupying its own name.
		{Name: "CollectMode", Value: godbus.MakeVariant("inactive-or-failed")},
		// Accounting is what makes CPUUsageNSec and MemoryCurrent readable as
		// unit properties, which is how stats are sampled without building
		// cgroup paths by hand.
		{Name: "CPUAccounting", Value: godbus.MakeVariant(true)},
		{Name: "MemoryAccounting", Value: godbus.MakeVariant(true)},
		{Name: "TasksAccounting", Value: godbus.MakeVariant(true)},
		{Name: "IOAccounting", Value: godbus.MakeVariant(true)},
	}

	killMode := spec.KillMode
	if killMode == "" {
		killMode = "mixed"
	}
	props = append(props, sd.Property{Name: "KillMode", Value: godbus.MakeVariant(killMode)})

	if spec.KillSignal != 0 {
		// A container running systemd needs SIGRTMIN+3; anything else leaves
		// it to time out and then be killed.
		props = append(props, sd.Property{Name: "KillSignal", Value: godbus.MakeVariant(int32(spec.KillSignal))})
	}
	if spec.TimeoutStopSec > 0 {
		props = append(props, sd.Property{
			Name:  "TimeoutStopUSec",
			Value: godbus.MakeVariant(spec.TimeoutStopSec * 1_000_000),
		})
	}
	if spec.WorkingDirectory != "" {
		props = append(props, sd.Property{Name: "WorkingDirectory", Value: godbus.MakeVariant(spec.WorkingDirectory)})
	}
	if len(spec.Environment) > 0 {
		props = append(props, sd.Property{Name: "Environment", Value: godbus.MakeVariant(spec.Environment)})
	}
	if spec.Slice != "" {
		props = append(props, sd.Property{Name: "Slice", Value: godbus.MakeVariant(spec.Slice)})
	}
	if spec.Delegate {
		props = append(props, sd.Property{Name: "Delegate", Value: godbus.MakeVariant(true)})
	}
	if spec.CPUQuotaPercent > 0 {
		// CPUQuotaPerSecUSec is microseconds of CPU per wall second, so 100%
		// of one core is 1_000_000.
		props = append(props, sd.Property{
			Name:  "CPUQuotaPerSecUSec",
			Value: godbus.MakeVariant(spec.CPUQuotaPercent * 10_000),
		})
	}
	if spec.MemoryMax > 0 {
		props = append(props, sd.Property{Name: "MemoryMax", Value: godbus.MakeVariant(spec.MemoryMax)})
	}
	if spec.MemoryHigh > 0 {
		props = append(props, sd.Property{Name: "MemoryHigh", Value: godbus.MakeVariant(spec.MemoryHigh)})
	}
	if spec.TasksMax > 0 {
		props = append(props, sd.Property{Name: "TasksMax", Value: godbus.MakeVariant(spec.TasksMax)})
	}
	return props
}

func (s *systemdSupervisor) Start(ctx context.Context, spec UnitSpec) error {
	if err := ValidateUnitName(spec.Name); err != nil {
		return err
	}
	if len(spec.Argv) == 0 {
		return fmt.Errorf("supervisor: %s has no command", spec.Name)
	}

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return ErrUnavailable
	}

	// A unit left in the failed state holds its name, so a restart after a
	// crash would fail with "unit already exists" rather than starting. Clear
	// it first; a unit that was never there answers with an error we ignore.
	errlog.Log(conn.ResetFailedUnitContext(ctx, spec.Name))

	// Buffered so systemd's completion send never blocks, even if the wait
	// below has already returned through ctx.
	done := make(chan string, 1)
	if _, err := conn.StartTransientUnitContext(ctx, spec.Name, "replace", s.properties(spec), done); err != nil {
		return fmt.Errorf("supervisor: start %s: %w", spec.Name, err)
	}

	timer := time.NewTimer(startTimeout)
	defer timer.Stop()

	select {
	case result := <-done:
		if result != "done" {
			return fmt.Errorf("supervisor: start %s: job %s", spec.Name, result)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("supervisor: start %s: %w", spec.Name, ctx.Err())
	case <-timer.C:
		return fmt.Errorf("supervisor: start %s: timed out after %s", spec.Name, startTimeout)
	}
}

func (s *systemdSupervisor) Stop(ctx context.Context, unitName string) error {
	if err := ValidateUnitName(unitName); err != nil {
		return err
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return ErrUnavailable
	}

	done := make(chan string, 1)
	if _, err := conn.StopUnitContext(ctx, unitName, "replace", done); err != nil {
		// Stopping something that is already gone reached the desired state.
		if isNoSuchUnit(err) {
			return nil
		}
		return fmt.Errorf("supervisor: stop %s: %w", unitName, err)
	}

	timer := time.NewTimer(startTimeout)
	defer timer.Stop()

	select {
	case result := <-done:
		if result != "done" && result != "canceled" {
			return fmt.Errorf("supervisor: stop %s: job %s", unitName, result)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("supervisor: stop %s: %w", unitName, ctx.Err())
	case <-timer.C:
		return fmt.Errorf("supervisor: stop %s: timed out after %s", unitName, startTimeout)
	}
}

func (s *systemdSupervisor) Kill(ctx context.Context, unitName string, signal int32) error {
	if err := ValidateUnitName(unitName); err != nil {
		return err
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return ErrUnavailable
	}
	// "all" signals every process in the unit's cgroup, which for a container
	// is the whole guest rather than only its init.
	errlog.Log(conn.KillUnitWithTarget(ctx, unitName, sd.All, signal))
	return nil
}

func (s *systemdSupervisor) Reset(ctx context.Context, unitName string) error {
	if err := ValidateUnitName(unitName); err != nil {
		return err
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return ErrUnavailable
	}
	if err := conn.ResetFailedUnitContext(ctx, unitName); err != nil {
		if isNoSuchUnit(err) {
			return nil
		}
		return fmt.Errorf("supervisor: reset %s: %w", unitName, err)
	}
	return nil
}

func (s *systemdSupervisor) Status(ctx context.Context, unitName string) (UnitStatus, error) {
	if err := ValidateUnitName(unitName); err != nil {
		return UnitStatus{}, err
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return UnitStatus{}, ErrUnavailable
	}

	props, err := conn.GetUnitPropertiesContext(ctx, unitName)
	if err != nil {
		if isNoSuchUnit(err) {
			return UnitStatus{}, ErrNoUnit
		}
		return UnitStatus{}, fmt.Errorf("supervisor: properties %s: %w", unitName, err)
	}
	status := statusFromProps(unitName, props)

	// Service-type properties live on the Service interface rather than the
	// Unit one, so they are a second read.
	svc, err := conn.GetUnitTypePropertiesContext(ctx, unitName, "Service")
	if err == nil {
		applyServiceProps(&status, svc)
	}

	if status.LoadState == "not-found" {
		return status, ErrNoUnit
	}
	return status, nil
}

func (s *systemdSupervisor) List(ctx context.Context, pattern string) ([]UnitStatus, error) {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return nil, ErrUnavailable
	}

	units, err := conn.ListUnitsByPatternsContext(ctx, nil, []string{pattern})
	if err != nil {
		return nil, fmt.Errorf("supervisor: list %s: %w", pattern, err)
	}

	out := make([]UnitStatus, 0, len(units))
	for _, u := range units {
		status := UnitStatus{
			Name:        u.Name,
			LoadState:   u.LoadState,
			ActiveState: u.ActiveState,
			SubState:    u.SubState,
		}
		// Enrich with the accounting properties. A unit that vanished between
		// the list and the read is skipped rather than reported with zeroes,
		// because a zeroed sample reads as "idle" and would be a lie.
		if full, err := s.Status(ctx, u.Name); err == nil {
			status = full
		}
		out = append(out, status)
	}
	return out, nil
}

func (s *systemdSupervisor) Subscribe(ctx context.Context, pattern string) (<-chan UnitStatus, error) {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return nil, ErrUnavailable
	}

	if err := conn.Subscribe(); err != nil {
		return nil, fmt.Errorf("supervisor: subscribe: %w", err)
	}

	updates, errs := conn.SubscribeUnitsCustomContext(
		ctx,
		2*time.Second,
		0,
		func(a, b *sd.UnitStatus) bool {
			// Report a change when any state field moved.
			return a.ActiveState == b.ActiveState && a.SubState == b.SubState && a.LoadState == b.LoadState
		},
		func(name string) bool {
			// Filter *out* names we do not care about.
			_, _, ok := GuestIDFromUnit(strings.TrimSuffix(name, ".service"))
			return !ok
		},
	)

	out := make(chan UnitStatus, 32)
	// ctx cancellation is this goroutine's stop path, per the lifecycle rule:
	// the caller cancels, the goroutine returns, the channel closes.
	go func() {
		defer close(out)
		defer func() { errlog.Log(conn.Unsubscribe()) }()
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errs:
				if err != nil {
					// The bus went away. Closing the channel tells the caller
					// to re-establish rather than trust stale state.
					return
				}
			case changes, ok := <-updates:
				if !ok {
					return
				}
				for name, u := range changes {
					status := UnitStatus{Name: name}
					if u != nil {
						status.LoadState = u.LoadState
						status.ActiveState = u.ActiveState
						status.SubState = u.SubState
					} else {
						// A nil value means the unit went away entirely.
						status.LoadState = "not-found"
						status.ActiveState = "inactive"
					}
					select {
					case out <- status:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, nil
}

// statusFromProps reads the Unit-interface properties.
func statusFromProps(name string, props map[string]interface{}) UnitStatus {
	status := UnitStatus{Name: name}
	if v, ok := props["LoadState"].(string); ok {
		status.LoadState = v
	}
	if v, ok := props["ActiveState"].(string); ok {
		status.ActiveState = v
	}
	if v, ok := props["SubState"].(string); ok {
		status.SubState = v
	}
	if v, ok := props["ActiveEnterTimestamp"].(uint64); ok {
		status.ActiveEnterTimestamp = v
	}
	if v, ok := props["CPUUsageNSec"].(uint64); ok {
		status.CPUUsageNSec = v
	}
	if v, ok := props["MemoryCurrent"].(uint64); ok {
		// systemd reports an unset limit as the maximum uint64 rather than 0.
		if v != ^uint64(0) {
			status.MemoryCurrent = v
		}
	}
	return status
}

// applyServiceProps reads the Service-interface properties: the main PID and,
// critically, Result — which is what distinguishes an OOM kill from a clean
// shutdown in the UI.
func applyServiceProps(status *UnitStatus, props map[string]interface{}) {
	if v, ok := props["MainPID"].(uint32); ok {
		status.MainPID = int(v)
	}
	if v, ok := props["Result"].(string); ok {
		status.Result = v
	}
	if status.CPUUsageNSec == 0 {
		if v, ok := props["CPUUsageNSec"].(uint64); ok {
			status.CPUUsageNSec = v
		}
	}
	if status.MemoryCurrent == 0 {
		if v, ok := props["MemoryCurrent"].(uint64); ok && v != ^uint64(0) {
			status.MemoryCurrent = v
		}
	}
}

// isNoSuchUnit reports whether a D-Bus error means the unit does not exist,
// which several callers treat as the desired state rather than a failure.
func isNoSuchUnit(err error) bool {
	if err == nil {
		return false
	}
	var derr godbus.Error
	if ok := asDBusError(err, &derr); ok {
		return derr.Name == "org.freedesktop.systemd1.NoSuchUnit" ||
			derr.Name == "org.freedesktop.DBus.Error.UnknownObject"
	}
	msg := err.Error()
	return strings.Contains(msg, "not loaded") || strings.Contains(msg, "NoSuchUnit")
}

func asDBusError(err error, target *godbus.Error) bool {
	var value godbus.Error
	if errors.As(err, &value) {
		*target = value
		return true
	}
	var pointer *godbus.Error
	if errors.As(err, &pointer) {
		*target = *pointer
		return true
	}
	return false
}

// startTimeout bounds waiting for the unit's start job to finish. systemd
// answers quickly; a unit that does not is a host problem worth surfacing
// rather than waiting out.
const startTimeout = 60 * time.Second
