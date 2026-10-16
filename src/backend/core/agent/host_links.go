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

// ErrHostLinksUnsupported means the connected agent predates host links. It
// wraps ErrComputeUnsupported, so a caller already mapping that keeps working.
var ErrHostLinksUnsupported = fmt.Errorf("%w: agent does not support host links", ErrComputeUnsupported)

// HostLinksSupported reports whether this agent build carries the
// host_link_request handler. Version and build both; see HostLinksMinBuild.
func (t *AgentTransport) HostLinksSupported() bool {
	if t == nil || t.conn == nil {
		return false
	}
	return clearsBuildFloor(t.conn, HostLinksMinVersion, HostLinksMinBuild)
}

// hostLinksFloor refuses a host link feature on an agent that would drop the
// message or ignore the field, naming what is running beside what is needed.
func (t *AgentTransport) hostLinksFloor() error {
	if t.HostLinksSupported() {
		return nil
	}
	version, build := "", ""
	if t != nil && t.conn != nil {
		t.conn.mu.Lock()
		version, build = t.conn.Version, t.conn.Build
		t.conn.mu.Unlock()
	}
	return fmt.Errorf("%w: this agent is %s build %q and host links need %s build %s or newer",
		ErrHostLinksUnsupported, version, build, HostLinksMinVersion, HostLinksMinBuild)
}

// HostLink ensures or removes one host link on the agent's host and returns
// the agent's answer. A refusal the agent made on purpose comes back as a
// payload with ErrorCode set, not as an error; err is reserved for transport
// failure and for an agent too old to be asked.
func (t *AgentTransport) HostLink(ctx context.Context, payload HostLinkPayload) (r0 *HostLinkPayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_link_"+payload.Op)
	defer func() { observe(err) }()
	if err := t.hostLinksFloor(); err != nil {
		return nil, err
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	// Registered before the write, so a fast reply always finds its waiter.
	t.mu.Lock()
	t.hostLinkWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.hostLinkWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{Type: MsgHostLinkRequest, ID: reqID, HostLink: &payload}); err != nil {
		return nil, fmt.Errorf("sending host link request to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		return msg.HostLink, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}

// netUsesHostLinks reports whether a net request carries a field only a host
// link agent understands; an older one would ignore it and report success.
func netUsesHostLinks(payload ComputeNetPayload) bool {
	return strings.TrimSpace(payload.HostLinkName) != "" || payload.IPv6Enabled ||
		strings.TrimSpace(payload.HostAccessIPv6) != ""
}

// ctUsesHostLinks reports whether a ct request asks for a static IPv6 address,
// which an older agent would drop without a word.
func ctUsesHostLinks(payload CTPayload) bool {
	if payload.Spec == nil {
		return false
	}
	for _, nic := range payload.Spec.NICs {
		if strings.TrimSpace(nic.StaticIPv6) != "" || strings.TrimSpace(nic.StaticGW6) != "" {
			return true
		}
	}
	return false
}
