// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// QEMU event kinds as the agent forwards them, and the SHUTDOWN reasons that
// separate a guest powering itself off from something on the host killing it.
const (
	qemuEventShutdown     = "shutdown"
	qemuEventReset        = "reset"
	qemuEventStop         = "stop"
	qemuEventResume       = "resume"
	qemuEventPowerdown    = "powerdown"
	qemuEventGuestPanic   = "guest_panic"
	qemuEventBlockIOError = "block_io_error"

	shutdownReasonGuestShutdown = "guest-shutdown"
	shutdownReasonGuestReset    = "guest-reset"
	shutdownReasonGuestPanic    = "guest-panic"
	shutdownReasonHostSignal    = "host-signal"
	shutdownReasonHostQMPQuit   = "host-qmp-quit"
	shutdownReasonHostQMPReset  = "host-qmp-system-reset"
	shutdownReasonHostUI        = "host-ui"
	shutdownReasonSubsystem     = "subsystem-reset"
)

// persistedQEMUEvents are the kinds worth keeping: each one either changes whether
// the guest is up or explains why it is not.
var persistedQEMUEvents = map[string]bool{
	qemuEventShutdown: true, qemuEventReset: true, qemuEventStop: true,
	qemuEventResume: true, qemuEventPowerdown: true, qemuEventGuestPanic: true,
	qemuEventBlockIOError: true,
}

// eventClockSkew is how far ahead of this host an agent timestamp may be before
// it is replaced by the receive time; a guest-side clock cannot reorder history.
const eventClockSkew = 5 * time.Minute

// vmEvent is one persisted QEMU event. Reason and device come from the guest's
// QEMU and are untrusted display text.
type vmEvent struct {
	at     time.Time
	kind   string
	reason string
	device string
}

// recordVMEvent persists one lifecycle-relevant QEMU event. It runs on the event
// worker, never on the transport's read loop, so a slow write stalls nothing else.
func (s *Service) recordVMEvent(event GuestEvent) {
	if s == nil || s.db == nil || !persistedQEMUEvents[event.Kind] || event.GuestID == "" {
		return
	}
	now := time.Now().UTC()
	at := event.At.UTC()
	if at.IsZero() || at.After(now.Add(eventClockSkew)) {
		at = now
	}
	ctx, cancel := context.WithTimeout(context.Background(), eventApplyTimeout)
	defer cancel()

	stamp := now.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_vm_events
			(id, vm_id, environment_id, kind, reason, device, at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), boundText(event.GuestID), boundText(event.EnvID), event.Kind,
		nullIfEmpty(boundText(event.Detail["reason"])), nullIfEmpty(boundText(event.Device)),
		at.Format(time.RFC3339), stamp, stamp); err != nil {
		s.logger.Warn("kvm: recording a QEMU event failed", "guest", event.GuestID, "kind", event.Kind, "error", err)
		return
	}
	s.maybePruneIncidentTables(ctx)
}

// readEvents reads persisted QEMU events since a moment, for one guest or (vmID
// "") for all, grouped by guest and oldest first.
func (s *Service) readEvents(ctx context.Context, vmID string, since time.Time) (map[string][]vmEvent, error) {
	query := `SELECT vm_id, kind, COALESCE(reason, ''), COALESCE(device, ''), at
		  FROM plugin_addon_kvm_vm_events WHERE at >= ?`
	args := []any{since.UTC().Format(time.RFC3339)}
	if vmID != "" {
		query += ` AND vm_id = ?`
		args = append(args, vmID)
	}
	query += ` ORDER BY at DESC LIMIT ?`
	args = append(args, incidentHistoryRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading QEMU events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]vmEvent{}
	for rows.Next() {
		var event vmEvent
		var id, at string
		if err := rows.Scan(&id, &event.kind, &event.reason, &event.device, &at); err != nil {
			return nil, fmt.Errorf("scanning QEMU event: %w", err)
		}
		event.at = parseTime(at)
		out[id] = append(out[id], event)
	}
	reverseGroups(out)
	return out, rows.Err()
}
