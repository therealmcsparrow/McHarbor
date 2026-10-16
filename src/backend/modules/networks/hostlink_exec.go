// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// Reasons a host link cannot be applied, reported as data.unavailable on the refusal and as
// hostLinkUnavailable on GET /networks/host-interfaces.
const (
	UnavailableAgentOutdated = "agent_outdated"
)

// hostLinker is what ensures and removes a host link: an agent transport or the native executor.
type hostLinker interface {
	HostLink(ctx context.Context, payload coreagent.HostLinkPayload) (*coreagent.HostLinkPayload, error)
}

// Executors hostLinkRoute can choose.
const (
	routeNone   = ""
	routeAgent  = "agent"
	routeNative = "native"
)

// hostLinkFacts are the inputs of the executor decision, gathered so the decision is a pure table.
type hostLinkFacts struct {
	AgentEnv        bool
	AgentConnected  bool
	AgentSupports   bool
	NativeHost      bool
	LocalSocket     bool
	NativeAvailable bool
}

// hostLinkRoute picks who touches the host's interfaces for an environment, or says why nobody can.
// An agent environment never falls back to the native path: the interfaces McHarbor sees are not
// that host's.
func hostLinkRoute(f hostLinkFacts) (route, unavailable string) {
	switch {
	case f.AgentEnv && !f.AgentConnected:
		return routeNone, UnavailableAgentUnreachable
	case f.AgentEnv && !f.AgentSupports:
		return routeNone, UnavailableAgentOutdated
	case f.AgentEnv:
		return routeAgent, ""
	case f.NativeHost && f.LocalSocket && f.NativeAvailable:
		return routeNative, ""
	}
	return routeNone, UnavailableContainerized
}

// hostLinkerFor resolves the executor for envID, or the reason there is none.
func (s *Service) hostLinkerFor(ctx context.Context, envID string) (hostLinker, string) {
	if s.linkerFor != nil {
		return s.linkerFor(ctx, envID)
	}
	facts := hostLinkFacts{NativeHost: nativeHost.Load()}
	var transport *coreagent.AgentTransport
	if s.pool != nil {
		facts.AgentEnv = s.pool.IsAgentEnvContext(ctx, envID)
	}
	if facts.AgentEnv && s.agents != nil {
		if transport = s.agents.ResolveTransport(envID); transport != nil {
			facts.AgentConnected = true
			facts.AgentSupports = transport.HostLinksSupported()
		}
	}
	native := newNativeHostLinker()
	if !facts.AgentEnv && facts.NativeHost {
		facts.LocalSocket = s.localSocketEnv(ctx, envID)
		facts.NativeAvailable = native != nil
	}
	route, unavailable := hostLinkRoute(facts)
	switch route {
	case routeAgent:
		return transport, ""
	case routeNative:
		return native, ""
	}
	return nil, unavailable
}

// hostLinkPayload builds the ensure request for a network's host link.
func hostLinkPayload(networkID, parent string, info HostLinkInfo) coreagent.HostLinkPayload {
	p := coreagent.HostLinkPayload{
		Op:       coreagent.HostLinkOpEnsure,
		Owner:    coreagent.HostLinkOwnerDocker,
		OwnerID:  networkID,
		Name:     info.Name,
		Parent:   parent,
		IPv4CIDR: info.IPv4,
	}
	if info.IPv6 != "" || info.IPv6Auto {
		p.IPv6Enabled = true
		p.IPv6CIDR = info.IPv6
	}
	return p
}

// runHostLink sends one operation and turns an agent refusal into a typed error whose text carries
// the agent's own words, so the handler can log and return them redacted.
func runHostLink(ctx context.Context, l hostLinker, p coreagent.HostLinkPayload) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	res, err := l.HostLink(ctx, p)
	if err != nil {
		if errors.Is(err, coreagent.ErrHostLinksUnsupported) {
			return &HostLinkUnavailableError{Reason: UnavailableAgentOutdated}
		}
		return fmt.Errorf("%w: %w", ErrHostLinkFailed, err)
	}
	if res == nil || res.Success {
		return nil
	}
	if res.ErrorCode == coreagent.HostLinkErrNameTaken {
		return fmt.Errorf("%w: %s", ErrHostLinkNameTaken, res.Error)
	}
	return fmt.Errorf("%w: %s (%s)", ErrHostLinkFailed, strings.TrimSpace(res.Error), res.ErrorCode)
}

// ensureHostLink applies info for networkID through l.
func ensureHostLink(ctx context.Context, l hostLinker, networkID, parent string, info HostLinkInfo) error {
	return runHostLink(ctx, l, hostLinkPayload(networkID, parent, info))
}

// removeHostLink removes networkID's host link through l; a link already gone is success.
func removeHostLink(ctx context.Context, l hostLinker, networkID, name string) error {
	return runHostLink(ctx, l, coreagent.HostLinkPayload{
		Op:      coreagent.HostLinkOpRemove,
		Owner:   coreagent.HostLinkOwnerDocker,
		OwnerID: networkID,
		Name:    name,
	})
}

// logRollback records a rollback that itself failed; the original error is what the caller returns.
func logRollback(what, id string, err error) {
	if err != nil {
		slog.Error("host link: rollback failed, remove it by hand", "step", what, "network", id, "error", err)
	}
}
