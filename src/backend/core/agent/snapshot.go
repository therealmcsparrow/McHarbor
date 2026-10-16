// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// SnapshotMinVersion is the first agent release that understands the
// snapshot message family. Older agents answer nothing, so the host
// must not wait on them: every entry point below checks this first and
// the backup pipeline falls back to pausing the container.
const SnapshotMinVersion = "2.5.0"

// SnapshotSupported reports whether the connected agent can take
// filesystem snapshots.
func (t *AgentTransport) SnapshotSupported() bool {
	return t.conn.SupportsFeature(SnapshotMinVersion)
}

// SnapshotDetect asks the agent which snapshot drivers exist on its own
// host. This is the call behind the Snapshots settings tab: the host
// cannot know what is installed on a machine it only reaches through a
// WebSocket, so it asks.
func (t *AgentTransport) SnapshotDetect(ctx context.Context, envID string) ([]SnapshotDriverOut, error) {
	reply, err := t.snapshotRPC(ctx, MsgSnapshotDetectRequest, SnapshotPayload{
		Operation: "detect",
		EnvID:     envID,
	})
	if err != nil {
		return nil, err
	}
	return reply.Detected, nil
}

// SnapshotCapability asks the agent to list the snapshot targets one
// driver exposes: datasets, subvolumes, logical volumes, or shadow
// copy volumes.
func (t *AgentTransport) SnapshotCapability(ctx context.Context, envID, driver string) (SnapshotPayload, error) {
	reply, err := t.snapshotRPC(ctx, MsgSnapshotCapabilityRequest, SnapshotPayload{
		Operation: "capability",
		EnvID:     envID,
		Driver:    driver,
	})
	if err != nil {
		return SnapshotPayload{}, err
	}
	return *reply, nil
}

// SnapshotCreate asks the agent to freeze the given targets. A reply
// carrying MountPaths means the archive can be read from a frozen view
// and the container never has to be paused.
func (t *AgentTransport) SnapshotCreate(ctx context.Context, envID string, payload SnapshotPayload) (SnapshotPayload, error) {
	payload.Operation = "create"
	payload.EnvID = envID
	reply, err := t.snapshotRPC(ctx, MsgSnapshotCreateRequest, payload)
	if err != nil {
		return SnapshotPayload{}, err
	}
	if !reply.Success {
		return *reply, fmt.Errorf("agent snapshot create failed: %s", strings.TrimSpace(reply.Error))
	}
	return *reply, nil
}

// SnapshotRemove asks the agent to destroy a snapshot it created. The
// caller treats failure as non-fatal: the archive is already written,
// and a leaked snapshot is better than a failed backup.
func (t *AgentTransport) SnapshotRemove(ctx context.Context, envID string, payload SnapshotPayload) error {
	payload.Operation = "remove"
	payload.EnvID = envID
	reply, err := t.snapshotRPC(ctx, MsgSnapshotRemoveRequest, payload)
	if err != nil {
		return err
	}
	if !reply.Success {
		return fmt.Errorf("agent snapshot remove failed: %s", strings.TrimSpace(reply.Error))
	}
	return nil
}

// snapshotRPC is the one request/reply round trip the four calls above
// share. It registers a waiter, writes the request, and blocks until
// the agent answers, the caller's context expires, or the transport
// closes.
func (t *AgentTransport) snapshotRPC(ctx context.Context, messageType string, payload SnapshotPayload) (r0 *SnapshotPayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, messageType)
	defer func() { observe(err) }()
	if !t.SnapshotSupported() {
		return nil, fmt.Errorf("agent does not support snapshots (requires %s+)", SnapshotMinVersion)
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	t.mu.Lock()
	t.snapshotWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.snapshotWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{
		Type:     messageType,
		ID:       reqID,
		Snapshot: &payload,
	}); err != nil {
		return nil, fmt.Errorf("sending snapshot request to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg == nil || msg.Snapshot == nil {
			return nil, fmt.Errorf("empty snapshot response from agent")
		}
		return msg.Snapshot, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}
