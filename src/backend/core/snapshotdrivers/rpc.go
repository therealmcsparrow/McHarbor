// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package snapshotdrivers

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
)

// RPCHandle is the bridge a snapshot plugin uses to reach an agent
// host. A plugin runs inside the McHarbor process and cannot execute
// anything on a machine McHarbor only reaches over a WebSocket, so the
// plugin describes what it wants and this handle carries it across.
//
// It satisfies the host's SnapshotRPCHandle contract, which names no
// driver: the plugins that need it (zfs, btrfs, lvm) each own one
// driver and the agent is asked to pick the one it actually has. Create
// and Remove resolve that driver from a capability probe first, because
// the agent needs a concrete driver to act on.
type RPCHandle struct {
	service *Service
}

// NewRPCHandle builds the plugin-facing snapshot bridge.
func NewRPCHandle(pool AgentPool) *RPCHandle {
	return &RPCHandle{service: New(pool)}
}

// Capability probes the agent for its snapshot targets.
func (h *RPCHandle) Capability(ctx context.Context, envID string) (snapshot.SnapshotCapability, error) {
	transport, err := h.service.transport(envID)
	if err != nil {
		return snapshot.SnapshotCapability{Available: false, Error: err.Error()}, nil //nolint:nilerr // a failed capability probe is reported in-band as SnapshotCapability.Error
	}
	reply, err := transport.SnapshotCapability(ctx, envID, "")
	if err != nil {
		return snapshot.SnapshotCapability{Available: false, Error: err.Error()}, nil //nolint:nilerr // a failed capability probe is reported in-band as SnapshotCapability.Error
	}
	return snapshot.SnapshotCapability{
		Driver:    reply.Driver,
		ToolPath:  reply.ToolPath,
		Version:   reply.Version,
		Available: reply.Available,
		Targets:   convertTargets(reply.TargetsOut),
		Error:     reply.Error,
	}, nil
}

// Create asks the agent to freeze the requested targets.
func (h *RPCHandle) Create(ctx context.Context, envID string, req snapshot.SnapshotCreateRequest) (snapshot.SnapshotRef, error) {
	driver, err := h.resolveDriver(ctx, envID)
	if err != nil {
		return snapshot.SnapshotRef{}, err
	}
	ref, _, err := h.service.CreateWithPaths(ctx, envID, driver, req)
	return ref, err
}

// Remove asks the agent to destroy a snapshot it created.
func (h *RPCHandle) Remove(ctx context.Context, envID string, ref snapshot.SnapshotRef) error {
	driver, err := h.resolveDriver(ctx, envID)
	if err != nil {
		return err
	}
	client, _, ok := h.service.Client(driver)
	if !ok {
		return fmt.Errorf("snapshot driver %q is not available on environment %s", driver, envID)
	}
	return client.Remove(ctx, map[string]string{EnvIDKey: envID}, ref)
}

// resolveDriver asks the agent which driver its host can actually use.
// The plugin contract carries no driver name, and guessing one would
// mean running `zfs` on a btrfs host.
func (h *RPCHandle) resolveDriver(ctx context.Context, envID string) (string, error) {
	capability, err := h.Capability(ctx, envID)
	if err != nil {
		return "", err
	}
	if !capability.Available || capability.Driver == "" {
		reason := capability.Error
		if reason == "" {
			reason = "no snapshot-capable driver found"
		}
		return "", fmt.Errorf("snapshot unavailable on environment %s: %s", envID, reason)
	}
	return capability.Driver, nil
}
