// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// rootsFileName is where the agent remembers the locations McHarbor gave it.
//
// Persisted rather than re-requested on connect because a guest's QEMU command
// line already points at the old root: an agent that came back up on its
// defaults would look at an empty directory and call every running guest an
// orphan.
const rootsFileName = "compute-roots.json"

// deniedRootPrefixes are directories compute state may never be rooted at.
//
// The same reasoning as storage.ValidatePoolTarget, duplicated deliberately:
// that one guards pool targets, this one guards the runtime roots, and the
// agent runs as root, so neither may rely on the control plane having checked.
var deniedRootPrefixes = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64", "/proc",
	"/root", "/run", "/sbin", "/sys", "/usr", "/var/lib/docker", "/var/lib/kubelet",
}

// validateRoot checks one location McHarbor asked this host to use.
func validateRoot(label, value string) error {
	if !filepath.IsAbs(value) {
		return fmt.Errorf("compute: the %s location %q must be an absolute path", label, value)
	}
	clean := filepath.Clean(value)
	if clean != value {
		return fmt.Errorf("compute: the %s location %q is not a normalized path (did you mean %q?)",
			label, value, clean)
	}
	for _, denied := range deniedRootPrefixes {
		if clean == denied {
			return fmt.Errorf("compute: refusing the %s location %q: it is a system directory", label, clean)
		}
		if denied != "/" && strings.HasPrefix(clean, denied+"/") {
			return fmt.Errorf("compute: refusing the %s location %q: it is inside %s", label, clean, denied)
		}
	}
	return nil
}

// effectiveRoots reports the locations currently in force.
func effectiveRoots(rt *computeRuntime) ComputeRoots {
	roots := ComputeRoots{VMRoot: vm.RuntimeRoot(), CTRoot: lxc.RuntimeRoot()}
	if rt != nil && rt.images != nil {
		roots.ImageRoot = rt.images.Root()
	}
	return roots
}

// rootsPath is where this agent stores its locations.
func (a *Agent) rootsPath() string {
	return filepath.Join(a.cfg.DataDir, rootsFileName)
}

// loadComputeRoots restores the locations McHarbor last set.
//
// Called before the compute runtime is built, so the image fetcher is
// constructed against the right directory rather than moved afterwards.
func (a *Agent) loadComputeRoots() ComputeRoots {
	var roots ComputeRoots
	data, err := os.ReadFile(a.rootsPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("compute: reading stored locations failed", "error", err)
		}
		return roots
	}
	if err := json.Unmarshal(data, &roots); err != nil {
		slog.Warn("compute: stored locations are unreadable; using the defaults", "error", err)
		return ComputeRoots{}
	}
	return roots
}

// saveComputeRoots persists the locations so a restart keeps them.
func (a *Agent) saveComputeRoots(roots ComputeRoots) error {
	data, err := json.Marshal(roots)
	if err != nil {
		return fmt.Errorf("compute: encoding locations: %w", err)
	}
	if err := os.MkdirAll(a.cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("compute: create %s: %w", a.cfg.DataDir, err)
	}
	return os.WriteFile(a.rootsPath(), data, 0o600)
}

// applyStoredRoots points the compute packages at the stored locations.
//
// Runs before any guest is touched, so there is nothing to strand and no need
// for the running-guest check handleComputeConfig makes.
func applyStoredRoots(roots ComputeRoots) {
	if roots.VMRoot != "" && validateRoot("virtual machine", roots.VMRoot) == nil {
		vm.SetRuntimeRoot(roots.VMRoot)
	}
	if roots.CTRoot != "" && validateRoot("system container", roots.CTRoot) == nil {
		lxc.SetRuntimeRoot(roots.CTRoot)
	}
}

// handleComputeConfig applies the locations McHarbor wants this host to use.
//
// A location is only moved when nothing is running on the old one. The paths
// are baked into live QEMU command lines and libcontainer state, so switching
// underneath a running guest would leave it unmanageable — the agent refuses
// and names what is in the way instead of half-applying.
func (rt *computeRuntime) handleComputeConfig(ctx context.Context, a *Agent, msg WSMessage) {
	reply := &ComputePayload{Operation: msg.Type}
	if msg.Compute != nil {
		reply.EnvID = msg.Compute.EnvID
	}

	if err := rt.applyRoots(ctx, a, msg); err != nil {
		reply.Error = err.Error()
	} else {
		reply.Success = true
	}
	roots := effectiveRoots(rt)
	reply.Roots = &roots
	errlog.Log(rt.send(WSMessage{Type: MsgComputeConfigResponse, ID: msg.ID, Compute: reply}))
}

// applyRoots is handleComputeConfig's body, split out so every failure path
// still reports the locations that remain in effect.
func (rt *computeRuntime) applyRoots(ctx context.Context, a *Agent, msg WSMessage) error {
	if msg.Compute == nil || msg.Compute.Roots == nil {
		return errors.New("compute: compute_config_request carried no locations")
	}
	want := *msg.Compute.Roots
	have := effectiveRoots(rt)

	changing := map[string]string{}
	if want.VMRoot != "" && want.VMRoot != have.VMRoot {
		if err := validateRoot("virtual machine", want.VMRoot); err != nil {
			return err
		}
		changing["vm"] = want.VMRoot
	}
	if want.CTRoot != "" && want.CTRoot != have.CTRoot {
		if err := validateRoot("system container", want.CTRoot); err != nil {
			return err
		}
		changing["ct"] = want.CTRoot
	}
	if want.ImageRoot != "" && want.ImageRoot != have.ImageRoot {
		if err := validateRoot("image", want.ImageRoot); err != nil {
			return err
		}
		changing["image"] = want.ImageRoot
	}
	if len(changing) == 0 {
		return nil
	}

	if _, moving := changing["vm"]; moving {
		if busy, err := rt.runningVMs(ctx); err != nil {
			return err
		} else if len(busy) > 0 {
			return fmt.Errorf("compute: cannot move the virtual machine location while %d guest(s) are running: %s",
				len(busy), strings.Join(busy, ", "))
		}
	}

	for kind, root := range changing {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return fmt.Errorf("compute: create %s: %w", root, err)
		}
		switch kind {
		case "vm":
			vm.SetRuntimeRoot(root)
		case "ct":
			lxc.SetRuntimeRoot(root)
		case "image":
			if err := rt.images.SetRoot(root); err != nil {
				return err
			}
		}
	}

	applied := effectiveRoots(rt)
	if err := a.saveComputeRoots(applied); err != nil {
		// The locations are live either way; failing to record them means the
		// next restart reverts, which the operator must be told about.
		rt.logger.Error("compute: the new locations could not be persisted", "error", err)
		return err
	}
	rt.logger.Info("compute: locations updated",
		"vm", applied.VMRoot, "ct", applied.CTRoot, "image", applied.ImageRoot)
	return nil
}

// runningVMs names the guests that would be stranded by a move.
func (rt *computeRuntime) runningVMs(ctx context.Context) ([]string, error) {
	if rt.vms == nil {
		return nil, nil
	}
	states, _, err := rt.vms.Query(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("compute: checking for running guests: %w", err)
	}
	var busy []string
	for _, state := range states {
		if state.State == vm.StateRunning || state.State == vm.StatePaused {
			busy = append(busy, state.VMID)
		}
	}
	return busy, nil
}
