// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"fmt"
	"slices"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// HostServicesSupported reports whether this agent build carries the
// host_service_request handler. Version and build both; see HostServicesMinBuild.
func (t *AgentTransport) HostServicesSupported() bool {
	return SupportsHostServices(t.conn.Version, t.conn.Build)
}

// HostServicesVersion returns the agent's version and build, so a refusal can
// name what is running beside what is needed.
func (t *AgentTransport) HostServicesVersion() (string, string) {
	return t.conn.Version, t.conn.Build
}

// HostServicesCapable reports what the agent said about systemd at handshake:
// true or false when it reported capabilities at all, and known=false when it
// did not (an older agent, or a connection relayed from another node).
func (t *AgentTransport) HostServicesCapable() (capable, known bool) {
	if t.conn.Capabilities == nil {
		return false, false
	}
	return slices.Contains(t.conn.Capabilities, HostServiceCapability), true
}

// HostService runs one systemd operation on the agent's host and returns the
// agent's answer. A refusal the agent made on purpose comes back as a payload
// with ErrorCode set, not as an error; err is reserved for transport failure.
func (t *AgentTransport) HostService(ctx context.Context, payload HostServicePayload) (r0 *HostServicePayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_service_"+payload.Op)
	defer func() { observe(err) }()
	if !t.HostServicesSupported() {
		return nil, fmt.Errorf("agent does not support host services (requires %s build %s+)", HostServicesMinVersion, HostServicesMinBuild)
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	t.mu.Lock()
	t.hostServiceWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.hostServiceWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{Type: MsgHostServiceRequest, ID: reqID, HostService: &payload}); err != nil {
		return nil, fmt.Errorf("sending host service request to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		return msg.HostService, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}
