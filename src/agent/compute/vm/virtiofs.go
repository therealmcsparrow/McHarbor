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
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// virtiofsSocketTimeout bounds waiting for a daemon to bind its socket.
//
// QEMU dials a vhost-user socket at startup and fails outright if nothing is
// listening, so the daemon has to be up before the guest is launched rather
// than merely started alongside it.
const virtiofsSocketTimeout = 10 * time.Second

// virtiofsBinaries are the names the daemon ships under, newest first.
//
// The Rust rewrite installs as `virtiofsd`; older packages ship QEMU's C
// version at a libexec path that is not on PATH at all.
var virtiofsBinaries = []string{
	"virtiofsd",
	"/usr/libexec/virtiofsd",
	"/usr/lib/qemu/virtiofsd",
}

// ShareUnitName returns the transient unit one share's daemon runs under.
//
// Like the vTPM's, the name avoids the mcharbor-vm- prefix that Query treats as
// a guest: a virtiofsd matching that glob would be reported as an orphan on
// every reconcile.
func ShareUnitName(vmID, shareID string) string {
	return supervisor.UnitPrefix + "fs-" + vmID + "-" + shareID + ".service"
}

// ShareSocketPath is where one share's daemon listens.
//
// Under the guest's own runtime directory, so it is removed with the guest and
// so a share cannot outlive the VM it belongs to.
func ShareSocketPath(vmID, shareID string) string {
	return filepath.Join(PathsFor(vmID).Dir, "virtiofs-"+shareID+".sock")
}

// ErrVirtiofsMissing means the host has no virtiofsd.
var ErrVirtiofsMissing = errors.New("vm: virtiofsd is not installed on this host")

// startShares brings up one virtiofsd per share, before QEMU.
//
// A partial start is rolled back rather than left running: a guest that comes
// up with three of its four shares mounts the wrong thing silently, which is
// worse than not starting.
func (m *Manager) startShares(ctx context.Context, spec Spec) error {
	for i, share := range spec.Shares {
		if err := m.startShare(ctx, spec, share); err != nil {
			for _, started := range spec.Shares[:i] {
				errlog.Log(m.stopShare(ctx, spec.ID, started.ID))
			}
			return err
		}
	}
	return nil
}

// startShare runs one virtiofsd and waits for its socket.
func (m *Manager) startShare(ctx context.Context, spec Spec, share Share) error {
	binary, err := lookupVirtiofsd()
	if err != nil {
		return fmt.Errorf("vm: %s shares %q but %w", spec.Name, share.Tag, err)
	}
	// The path is validated on the backend against the operator's configured
	// roots. This is the second check, on the side that can actually see the
	// filesystem: a share of a path that is not a directory would otherwise
	// fail as an opaque daemon crash.
	info, err := os.Stat(share.HostPath)
	if err != nil {
		return fmt.Errorf("vm: the shared path for %q cannot be read: %w", share.Tag, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("vm: the shared path for %q is not a directory", share.Tag)
	}

	unitName := ShareUnitName(spec.ID, share.ID)
	// A daemon left behind by a previous boot still holds the socket, and
	// virtiofsd refuses to bind a path that exists.
	errlog.Log(m.sup.Stop(ctx, unitName))
	errlog.Log(m.sup.Reset(ctx, unitName))
	errlog.Log(os.Remove(share.Socket))

	argv := []string{
		binary,
		"--socket-path=" + share.Socket,
		"--shared-dir=" + share.HostPath,
		// Sandboxing the daemon in its own mount namespace is what keeps a
		// symlink inside the shared directory from reaching the rest of the
		// host filesystem.
		"--sandbox=namespace",
	}
	if share.ReadOnly {
		argv = append(argv, "--readonly")
	}

	if err := m.sup.Start(ctx, supervisor.UnitSpec{
		Name:             unitName,
		Description:      "McHarbor virtiofs " + share.Tag + " for " + spec.Name,
		Argv:             argv,
		WorkingDirectory: PathsFor(spec.ID).Dir,
		KillMode:         "mixed",
		TimeoutStopSec:   10,
	}); err != nil {
		return fmt.Errorf("vm: start the virtiofs daemon for %q: %w", share.Tag, err)
	}

	if err := waitForSocket(ctx, share.Socket, virtiofsSocketTimeout); err != nil {
		// The unit's own verdict beats the timeout: a daemon refused by a
		// security policy exits at once, and reporting only that the socket
		// never appeared sends the reader looking in the wrong place.
		detail := ""
		if status, statusErr := m.sup.Status(ctx, unitName); statusErr == nil {
			detail = fmt.Sprintf(" (unit %s/%s, result %q)",
				status.ActiveState, status.SubState, status.Result)
		}
		errlog.Log(m.stopShare(ctx, spec.ID, share.ID))
		return fmt.Errorf("vm: the virtiofs daemon for %q never came up%s: %w",
			share.Tag, detail, err)
	}
	return nil
}

// stopShares stops every daemon belonging to a guest.
//
// The share ids come from the sockets on disk rather than from a spec, because
// the stop and delete paths are given only a guest id — and a daemon whose
// share was removed from the definition while the guest ran would otherwise
// survive the guest that owned it.
func (m *Manager) stopShares(ctx context.Context, vmID string) {
	entries, err := os.ReadDir(PathsFor(vmID).Dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "virtiofs-") || !strings.HasSuffix(name, ".sock") {
			continue
		}
		shareID := strings.TrimSuffix(strings.TrimPrefix(name, "virtiofs-"), ".sock")
		errlog.Log(m.stopShare(ctx, vmID, shareID))
	}
}

// stopShare stops one share's daemon and removes its socket.
func (m *Manager) stopShare(ctx context.Context, vmID, shareID string) error {
	unitName := ShareUnitName(vmID, shareID)
	if err := m.sup.Stop(ctx, unitName); err != nil && !errors.Is(err, supervisor.ErrNoUnit) {
		return fmt.Errorf("vm: stop the virtiofs daemon for %s: %w", shareID, err)
	}
	errlog.Log(m.sup.Reset(ctx, unitName))
	errlog.Log(os.Remove(ShareSocketPath(vmID, shareID)))
	return nil
}

// lookupVirtiofsd finds the daemon under any of the names it ships as.
func lookupVirtiofsd() (string, error) {
	for _, candidate := range virtiofsBinaries {
		if strings.ContainsRune(candidate, '/') {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", ErrVirtiofsMissing
}
