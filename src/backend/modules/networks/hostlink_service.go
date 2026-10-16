// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Host link states reported by HostLinkStatus.
const (
	HostLinkStatePresent = "present"
	HostLinkStateMissing = "missing"
	HostLinkStateUnknown = "unknown"
	HostLinkStateNone    = "none"
)

// HostLinkStatus is the body of GET /networks/{id}/host-link. State is "unknown" when nothing could
// list the host's interfaces, which is different from "missing".
type HostLinkStatus struct {
	HostLink  *HostLinkInfo `json:"hostLink"`
	Parent    string        `json:"parent,omitempty"`
	State     string        `json:"state"`
	Addresses []string      `json:"addresses,omitempty"`
	// Available says whether Re-apply can run; Unavailable says why not.
	Available   bool   `json:"available"`
	Unavailable string `json:"unavailable,omitempty"`
}

// HostLinkStatus reports whether the interface a network's labels name exists on its host.
func (s *Service) HostLinkStatus(ctx context.Context, envID, id string) (HostLinkStatus, error) {
	net, err := s.Inspect(ctx, envID, id)
	if err != nil {
		return HostLinkStatus{}, err
	}
	info := hostLinkFromLabels(net.Labels)
	if info == nil {
		return HostLinkStatus{State: HostLinkStateNone}, nil
	}
	out := HostLinkStatus{HostLink: info, Parent: net.Options["parent"], State: HostLinkStateUnknown}
	linker, reason := s.hostLinkerFor(ctx, envID)
	out.Available, out.Unavailable = linker != nil, reason

	links, listed, _ := s.hostLinks(ctx, envID)
	if !listed {
		return out, nil
	}
	out.State = HostLinkStateMissing
	for _, l := range links {
		if l.Name == info.Name {
			out.State, out.Addresses = HostLinkStatePresent, l.Addresses
			break
		}
	}
	return out, nil
}

// ReapplyHostLink ensures the interface a network's labels name, which is how an operator repairs
// one that a reboot or an outside change removed.
func (s *Service) ReapplyHostLink(ctx context.Context, envID, id string) (HostLinkStatus, error) {
	net, err := s.Inspect(ctx, envID, id)
	if err != nil {
		return HostLinkStatus{}, err
	}
	info := hostLinkFromLabels(net.Labels)
	if info == nil {
		return HostLinkStatus{}, ErrHostLinkNotConfigured
	}
	linker, reason := s.hostLinkerFor(ctx, envID)
	if linker == nil {
		return HostLinkStatus{}, &HostLinkUnavailableError{Reason: reason}
	}
	if err := ensureHostLink(ctx, linker, net.ID, strings.TrimSpace(net.Options["parent"]), *info); err != nil {
		return HostLinkStatus{}, err
	}
	return s.HostLinkStatus(ctx, envID, net.ID)
}

// ReconcileHostLinks re-ensures the host link of every labelled macvlan network on envID and
// returns how many it applied. It exists for a native McHarbor after a host reboot, when the
// interfaces are gone but the labels that describe them are not.
func (s *Service) ReconcileHostLinks(ctx context.Context, envID string) (int, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return 0, err
	}
	callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	listRes, err := cli.NetworkList(callCtx, client.NetworkListOptions{})
	if err != nil {
		return 0, fmt.Errorf("listing networks: %w", err)
	}
	linker, reason := s.hostLinkerFor(ctx, envID)
	applied := 0
	for _, n := range listRes.Items {
		info := hostLinkFromLabels(n.Labels)
		if info == nil || n.Driver != "macvlan" {
			continue
		}
		if linker == nil {
			slog.Warn("host link reconcile: no executor", "env", envID, "network", n.ID, "reason", reason)
			continue
		}
		if err := ensureHostLink(ctx, linker, n.ID, strings.TrimSpace(n.Options["parent"]), *info); err != nil {
			slog.Warn("host link reconcile: ensure failed", "env", envID, "network", n.ID, "link", info.Name, "error", err)
			continue
		}
		applied++
	}
	return applied, nil
}
