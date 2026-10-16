// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// tpmSocketTimeout bounds waiting for swtpm to create its control socket. QEMU
// connects to it immediately and fails outright if it is not there yet.
const tpmSocketTimeout = 10 * time.Second

// TPMUnitName returns the transient unit a guest's software TPM runs under.
//
// The name deliberately avoids the mcharbor-vm- prefix: Query reports any unit
// matching that glob without a backend row as an orphan, and a vTPM would be
// one on every reconcile.
func TPMUnitName(vmID string) string { return supervisor.UnitPrefix + "tpm-" + vmID + ".service" }

// startTPM runs swtpm for a guest and waits for its control socket.
//
// The state directory persists across restarts because the guest's keys live in
// it; only Delete removes it, with the rest of the guest's runtime tree.
func (m *Manager) startTPM(ctx context.Context, spec Spec, paths Paths) error {
	binary, err := exec.LookPath("swtpm")
	if err != nil {
		return fmt.Errorf("vm: %s needs a TPM but swtpm is not installed on this host: %w", spec.Name, err)
	}
	// The agent is unconfined, so it creates these; only swtpm's own writes are
	// mediated, and those all land under TPMRoot.
	if err := os.MkdirAll(paths.TPMState, 0o700); err != nil {
		return fmt.Errorf("vm: create TPM state directory for %s: %w", spec.Name, err)
	}

	unitName := TPMUnitName(spec.ID)
	// A vTPM left behind by a previous boot holds the socket, and swtpm refuses
	// to bind a path that already exists.
	errlog.Log(m.sup.Stop(ctx, unitName))
	errlog.Log(m.sup.Reset(ctx, unitName))
	errlog.Log(os.Remove(paths.TPMSocket))

	if err := m.sup.Start(ctx, supervisor.UnitSpec{
		Name:        unitName,
		Description: "McHarbor vTPM " + spec.Name,
		Argv: []string{
			binary, "socket",
			"--tpm2",
			"--tpmstate", "dir=" + paths.TPMState,
			"--ctrl", "type=unixio,path=" + paths.TPMSocket,
		},
		WorkingDirectory: paths.Dir,
		KillMode:         "mixed",
		TimeoutStopSec:   10,
	}); err != nil {
		return fmt.Errorf("vm: start vTPM for %s: %w", spec.Name, err)
	}

	if err := waitForSocket(ctx, paths.TPMSocket, tpmSocketTimeout); err != nil {
		// The unit's own verdict is worth more than the timeout: swtpm that was
		// refused by a security policy exits immediately, and saying only that
		// the socket never appeared sends the reader looking in the wrong place.
		detail := ""
		if status, statusErr := m.sup.Status(ctx, unitName); statusErr == nil {
			detail = fmt.Sprintf(" (unit %s/%s, result %q)", status.ActiveState, status.SubState, status.Result)
		}
		errlog.Log(m.stopTPM(ctx, spec.ID))
		return fmt.Errorf("vm: vTPM for %s never came up%s: %w", spec.Name, detail, err)
	}
	return nil
}

// removeTPMState deletes a guest's vTPM directory, key material included.
func removeTPMState(vmID string) error {
	if err := os.RemoveAll(PathsFor(vmID).TPMDir); err != nil {
		return fmt.Errorf("vm: remove vTPM state for %s: %w", vmID, err)
	}
	return nil
}

// stopTPM stops a guest's vTPM, leaving its state on disk.
func (m *Manager) stopTPM(ctx context.Context, vmID string) error {
	unitName := TPMUnitName(vmID)
	if err := m.sup.Stop(ctx, unitName); err != nil && !errors.Is(err, supervisor.ErrNoUnit) {
		return fmt.Errorf("vm: stop vTPM for %s: %w", vmID, err)
	}
	errlog.Log(m.sup.Reset(ctx, unitName))
	errlog.Log(os.Remove(PathsFor(vmID).TPMSocket))
	return nil
}

// waitForSocket polls for a unix socket to appear, because a unit that has
// started is not yet a process that has bound its path.
func waitForSocket(ctx context.Context, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not appear within %s", path, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
