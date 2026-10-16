// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
)

// handleHostLinkRequest answers one host_link_request. It runs on its own
// goroutine, because a netlink call against a busy host can take a moment.
func (a *Agent) handleHostLinkRequest(ctx context.Context, conn *websocket.Conn, reqID string, req HostLinkPayload) {
	reply := a.executeHostLink(ctx, req)

	writeMu.Lock()
	writeErr := conn.WriteJSON(WSMessage{Type: MsgHostLinkResponse, ID: reqID, HostLink: reply})
	writeMu.Unlock()
	if writeErr != nil {
		a.logger.Warn("host links: sending the reply failed", "op", req.Op, "owner", req.Owner, "error", writeErr)
	}
}

// executeHostLink applies one request and persists its desired state, so the
// link comes back after a reboot without the server having to ask again.
func (a *Agent) executeHostLink(_ context.Context, req HostLinkPayload) *HostLinkPayload {
	reply := &HostLinkPayload{Op: req.Op, Owner: req.Owner, OwnerID: req.OwnerID, Name: req.Name, Parent: req.Parent}
	fail := func(err error) *HostLinkPayload {
		reply.Error = err.Error()
		reply.ErrorCode = hostLinkErrorCode(err)
		return reply
	}

	switch req.Op {
	case HostLinkOpEnsure:
		rec := hostLinkRecord{
			Owner:       req.Owner,
			OwnerID:     req.OwnerID,
			Name:        req.Name,
			Parent:      req.Parent,
			IPv4CIDR:    req.IPv4CIDR,
			IPv6Enabled: req.IPv6Enabled,
			IPv6CIDR:    req.IPv6CIDR,
		}
		name, err := a.ensureHostLinkRecord(rec)
		if err != nil {
			return fail(err)
		}
		reply.Name = name

	case HostLinkOpRemove:
		if err := a.removeHostLinkRecord(req.Owner, req.OwnerID); err != nil {
			return fail(err)
		}

	default:
		return fail(fmt.Errorf("%w: unknown operation %q", netconf.ErrHostLinkInvalid, req.Op))
	}
	reply.Success = true
	return reply
}

// ensureHostLinkRecord applies rec over whatever the previous record applied,
// then saves it, and returns the link name actually used.
func (a *Agent) ensureHostLinkRecord(rec hostLinkRecord) (string, error) {
	if _, err := a.hostLinks.path(rec.Owner, rec.OwnerID); err != nil {
		return "", err
	}
	hostLinkApplyMu.Lock()
	defer hostLinkApplyMu.Unlock()

	prev, err := a.hostLinks.load(rec.Owner, rec.OwnerID)
	if err != nil {
		// An unreadable record only costs the stale-address cleanup; the link
		// itself is still worth bringing up.
		a.logger.Warn("host links: previous record unreadable", "owner", rec.Owner, "ownerId", rec.OwnerID, "error", err)
		prev = nil
	}
	name, err := applyHostLinkRecord(rec, prev.staleCIDRs())
	if err != nil {
		return "", err
	}
	rec.Name = name
	if err := a.hostLinks.save(rec); err != nil {
		// The link is up; only the reboot replay is lost, so this is logged
		// rather than failing a request whose effect already happened.
		a.logger.Error("host links: persisting desired state failed", "owner", rec.Owner, "ownerId", rec.OwnerID, "error", err)
	}
	return name, nil
}

// removeHostLinkRecord deletes owner/ownerID's link and forgets its record.
func (a *Agent) removeHostLinkRecord(owner, ownerID string) error {
	if _, err := a.hostLinks.path(owner, ownerID); err != nil {
		return err
	}
	hostLinkApplyMu.Lock()
	defer hostLinkApplyMu.Unlock()
	if err := netconf.RemoveHostLink(owner, ownerID); err != nil {
		return err
	}
	return a.hostLinks.forget(owner, ownerID)
}

// applyHostLinkRecord puts rec on the host. The caller holds hostLinkApplyMu.
func applyHostLinkRecord(rec hostLinkRecord, stale []string) (string, error) {
	parent := rec.Parent
	if rec.VLANTag != 0 {
		resolved, err := netconf.EnsureVLANParent(rec.Parent, rec.VLANTag)
		if err != nil {
			return "", err
		}
		parent = resolved
	}
	spec := netconf.ResolveHostShim(netconf.HostShimSpec{
		Driver:       netconf.DriverMacvlan,
		Parent:       parent,
		AddressCIDR:  rec.IPv4CIDR,
		Name:         rec.Name,
		Owner:        rec.Owner,
		OwnerID:      rec.OwnerID,
		IPv6Enabled:  rec.IPv6Enabled,
		AddressCIDR6: rec.IPv6CIDR,
		StaleCIDRs:   stale,
	})
	if err := netconf.EnsureHostShim(spec); err != nil {
		return "", err
	}
	return spec.Name, nil
}

// startHostLinkReplay restores every persisted host link in the background.
func (a *Agent) startHostLinkReplay(ctx context.Context) {
	records, err := a.hostLinks.list()
	if err != nil {
		a.logger.Warn("host links: some records could not be read", "error", err)
	}
	if len(records) == 0 {
		return
	}
	// goroutine lifecycle: ends when ctx (the agent's shutdown context) is cancelled, or once every record is applied or has given up after hostLinkRetryWindow.
	go a.replayHostLinks(ctx, records)
}

// replayHostLinks applies records through the shared replay loop, with the
// production retry schedule.
func (a *Agent) replayHostLinks(ctx context.Context, records []hostLinkRecord) {
	hostLinkReplayer{
		store:  a.hostLinks,
		logger: a.logger,
		apply: func(rec hostLinkRecord) error {
			_, err := applyHostLinkRecord(rec, nil)
			return err
		},
		delay:  hostLinkRetryDelay,
		window: hostLinkRetryWindow,
	}.run(ctx, records)
}
