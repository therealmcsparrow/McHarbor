// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// handleSnapshotRequest dispatches a snapshot-family RPC to the
// platform-specific implementation (snapshot_linux.go,
// snapshot_windows.go, snapshot_darwin.go). Operation disambiguates
// capability / create / remove inside SnapshotPayload.
//
// All three operations share a 5-minute timeout set by the caller and
// are run from a goroutine so the read loop doesn't block while the
// snapshot is in flight.
func (a *Agent) handleSnapshotRequest(ctx context.Context, conn *websocket.Conn, reqID string, payload SnapshotPayload) {
	// Reset timeout to the snapshot's own cap when the request is
	// explicit (e.g. a create with a 3600s TimeoutSec). The host's
	// 5-minute outer cap still applies because the goroutine was
	// spawned with snapCtx (context.WithTimeout).
	if payload.TimeoutSec > 0 && time.Duration(payload.TimeoutSec)*time.Second < (5*time.Minute) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(payload.TimeoutSec)*time.Second)
		defer cancel()
	}

	out, err := dispatchSnapshot(ctx, payload)
	if err != nil && out == nil {
		// Both nil result and error means the platform
		// implementation didn't even fill a partial reply; we
		// synthesize one so the waiter doesn't hang.
		out = &SnapshotPayload{
			Operation: payload.Operation,
			EnvID:     payload.EnvID,
			Driver:    payload.Driver,
			Success:   false,
			Error:     err.Error(),
		}
	}
	if out == nil {
		out = &SnapshotPayload{
			Operation: payload.Operation,
			EnvID:     payload.EnvID,
			Driver:    payload.Driver,
			Success:   true,
		}
	}
	// Always echo the request's Operation / EnvID / Driver so the
	// server's SnapshotRPCHandle can route the reply without parsing
	// the body.
	if out.Operation == "" {
		out.Operation = payload.Operation
	}
	if out.EnvID == "" {
		out.EnvID = payload.EnvID
	}
	if out.Driver == "" {
		out.Driver = payload.Driver
	}
	errlog.Log(writeSnapshotResponse(ctx, conn, reqID, *out))
}

// dispatchSnapshot fans out to the per-operation handlers. The
// actual driver-specific work (zfs list, btrfs subvolume, lvs, VSS)
// lives in the build-tagged platform files. dispatchSnapshot only
// dispatches by Operation; the platform file picks the driver.
func dispatchSnapshot(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	switch payload.Operation {
	case "detect":
		return snapshotDetect(ctx, payload)
	case "capability":
		return snapshotCapability(ctx, payload)
	case "create":
		return snapshotCreate(ctx, payload)
	case "remove":
		return snapshotRemove(ctx, payload)
	default:
		return &SnapshotPayload{
			Success: false,
			Error:   "unknown snapshot operation: " + payload.Operation,
		}, nil
	}
}

// writeSnapshotResponse serializes the snapshot reply to the
// WebSocket. Picks the right response message type from the
// Operation field so the server's dispatch can route to the matching
// waiter map without an extra round-trip lookup.
//
// writeMu is taken exactly once, and only around the write: it is the
// global every producer shares, it is not reentrant, and a second
// acquisition here froze the whole agent on the first snapshot RPC it
// ever answered.
func writeSnapshotResponse(ctx context.Context, conn *websocket.Conn, id string, payload SnapshotPayload) error {
	respType := MsgSnapshotCapabilityResponse
	switch payload.Operation {
	case "detect":
		respType = MsgSnapshotDetectResponse
	case "create":
		respType = MsgSnapshotCreateResponse
	case "remove":
		respType = MsgSnapshotRemoveResponse
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteJSON(WSMessage{
		Type:     respType,
		ID:       id,
		Snapshot: &payload,
	})
}
