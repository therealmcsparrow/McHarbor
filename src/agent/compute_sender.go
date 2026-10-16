// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"sync"
)

// The compute subsystem's outbound path.
//
// Compute writes from several places at once — a guest console pump, a stats
// sampler, an unsolicited QMP event, a command reply — and a gorilla WebSocket
// connection permits exactly one concurrent writer. Every other producer in
// this agent serializes on the per-connection write mutex; compute does the
// same, through a sender installed for the life of a connection.
//
// It is deliberately not a stored *websocket.Conn. The connection is replaced
// on every reconnect, and a compute goroutine holding a stale one would write
// into a dead socket rather than failing in a way the caller can act on.

// errNoComputeConnection means there is no live server connection.
//
// Callers treat this as "the operation could not be reported", never as "the
// operation failed" — the guest may well have started. Reconciliation on the
// next connect is what resolves the difference, which is why compute commands
// are never queued for replay.
var errNoComputeConnection = errors.New("compute: not connected to McHarbor")

var (
	computeSendMu sync.RWMutex
	computeSendFn func(WSMessage) error
)

// setComputeSender installs the outbound path for a connection. Passing nil
// clears it, which the connection handler does on the way out so nothing writes
// into a socket that has gone.
func setComputeSender(send func(WSMessage) error) {
	computeSendMu.Lock()
	computeSendFn = send
	computeSendMu.Unlock()
}

// sendCompute publishes a compute message to the server.
func (a *Agent) sendCompute(msg WSMessage) error {
	computeSendMu.RLock()
	send := computeSendFn
	computeSendMu.RUnlock()
	if send == nil {
		return errNoComputeConnection
	}
	return send(msg)
}

// replyComputeError answers a request the runtime could not even start.
//
// The reply is shaped to the family so the server's waiter finds a payload
// where it expects one; a bare error message with no payload would look like a
// dropped response and hang the caller until its own timeout.
func (a *Agent) replyComputeError(msg WSMessage, err error) {
	text := err.Error()
	reply := WSMessage{ID: msg.ID}

	switch {
	case msg.Compute != nil:
		reply.Type = MsgComputeProbeResponse
		reply.Compute = &ComputePayload{Operation: "probe", Success: false, Error: text}
	case msg.Pool != nil:
		reply.Type = MsgPoolDetectResponse
		reply.Pool = &ComputePoolPayload{Operation: msg.Pool.Operation, Success: false, Error: text}
	case msg.Net != nil:
		reply.Type = MsgNetDetectResponse
		reply.Net = &ComputeNetPayload{Operation: msg.Net.Operation, Success: false, Error: text}
	case msg.ComputeImage != nil:
		reply.Type = MsgImageFetchResult
		reply.ComputeImage = &ComputeImagePayload{Operation: msg.ComputeImage.Operation, Success: false, Error: text}
	case msg.VM != nil:
		reply.Type = MsgVMResult
		reply.VM = &VMPayload{Operation: msg.Type, VMID: msg.VM.VMID, Success: false, Error: text}
	case msg.CT != nil:
		reply.Type = MsgCTResult
		reply.CT = &CTPayload{Operation: msg.Type, ContainerID: msg.CT.ContainerID, Success: false, Error: text}
	default:
		reply.Type = MsgComputeProbeResponse
		reply.Compute = &ComputePayload{Success: false, Error: text}
	}

	if err := a.sendCompute(reply); err != nil {
		a.logger.Debug("compute error reply failed", "id", msg.ID, "error", err)
	}
}
