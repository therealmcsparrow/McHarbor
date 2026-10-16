// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/rs/xid"
)

// Lifecycle actions McHarbor records before it asks a host to act on a guest.
const (
	intentStart         = "start"
	intentShutdown      = "shutdown"
	intentKill          = "kill"
	intentReset         = "reset"
	intentPause         = "pause"
	intentResume        = "resume"
	intentDelete        = "delete"
	intentSnapshotPause = "snapshot_pause"
)

// Outcomes of a recorded action. Pending is what a row reads while the agent call
// is still running, and what it keeps if the process died before it returned.
const (
	intentPending = "pending"
	intentOK      = "ok"
	intentFailed  = "failed"
)

const (
	// intentRetention is how long intents and QEMU events are kept.
	intentRetention = 30 * 24 * time.Hour

	// intentPruneEvery rate-limits the prune so it rides the write path for free.
	intentPruneEvery = time.Hour

	// intentWriteTimeout bounds one intent write, detached from a request that
	// may already be answered.
	intentWriteTimeout = 5 * time.Second

	// maxIntentText bounds every free-text column, error messages included.
	maxIntentText = 240
)

// intentOrigin says who asked for an action and through which path.
type intentOrigin struct {
	actor  string
	source string
}

type intentOriginKey struct{}

// withIntentOrigin marks ctx with who is acting, so the service can record it
// without every lifecycle method growing an actor parameter.
func withIntentOrigin(ctx context.Context, actor, source string) context.Context {
	return context.WithValue(ctx, intentOriginKey{}, intentOrigin{actor: actor, source: source})
}

// intentOriginFrom reads the origin, defaulting to "system" so an unmarked caller
// is still recorded as McHarbor acting rather than dropped.
func intentOriginFrom(ctx context.Context) intentOrigin {
	if origin, ok := ctx.Value(intentOriginKey{}).(intentOrigin); ok && origin.actor != "" {
		if origin.source == "" {
			origin.source = "system"
		}
		return origin
	}
	return intentOrigin{actor: "system", source: "system"}
}

// userActor names a person for the actor column.
func userActor(userID string) string { return "user:" + userID }

// lifecycleIntent is one recorded action as the classifier reads it.
type lifecycleIntent struct {
	at         time.Time
	finishedAt time.Time
	action     string
	actor      string
	source     string
	outcome    string
	errorText  string
}

// intentPruneAt is when the tables were last pruned, as Unix seconds.
var intentPruneAt atomic.Int64

// beginIntent records that McHarbor is about to act on vm and returns the row id.
// A failed write is logged and returns "", because recording must never block the act.
func (s *Service) beginIntent(ctx context.Context, vm VM, action string) string {
	if s == nil || s.db == nil {
		return ""
	}
	origin := intentOriginFrom(ctx)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), intentWriteTimeout)
	defer cancel()

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_lifecycle_intents
			(id, vm_id, environment_id, action, actor, source, outcome, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, vm.ID, vm.EnvironmentID, action,
		boundText(origin.actor), boundText(origin.source), intentPending, now, now); err != nil {
		s.logger.Warn("kvm: recording a lifecycle action failed", "vm", vm.ID, "action", action, "error", err)
		return ""
	}
	s.maybePruneIncidentTables(ctx)
	return id
}

// finishIntent stamps the outcome on a row beginIntent wrote.
func (s *Service) finishIntent(ctx context.Context, id string, actErr error) {
	if s == nil || s.db == nil || id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), intentWriteTimeout)
	defer cancel()

	outcome, message := intentOK, ""
	if actErr != nil {
		outcome, message = intentFailed, boundText(actErr.Error())
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_lifecycle_intents
		   SET outcome = ?, error = ?, updated_at = ?
		 WHERE id = ?`,
		outcome, nullIfEmpty(message), time.Now().UTC().Format(time.RFC3339), id); err != nil {
		s.logger.Warn("kvm: recording a lifecycle outcome failed", "intent", id, "error", err)
	}
}

// incidentHistoryRows bounds one read of intents or events. The newest rows are
// kept when it bites, because the classifier only ever needs the recent end.
const incidentHistoryRows = 5000

// readIntents reads recorded actions since a moment, for one guest or (vmID "")
// for all, grouped by guest and oldest first.
func (s *Service) readIntents(ctx context.Context, vmID string, since time.Time) (map[string][]lifecycleIntent, error) {
	query := `SELECT vm_id, action, actor, source, outcome, COALESCE(error, ''), created_at, updated_at
		  FROM plugin_addon_kvm_lifecycle_intents WHERE created_at >= ?`
	args := []any{since.UTC().Format(time.RFC3339)}
	if vmID != "" {
		query += ` AND vm_id = ?`
		args = append(args, vmID)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, incidentHistoryRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading lifecycle actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]lifecycleIntent{}
	for rows.Next() {
		var intent lifecycleIntent
		var id, created, updated string
		if err := rows.Scan(&id, &intent.action, &intent.actor, &intent.source, &intent.outcome,
			&intent.errorText, &created, &updated); err != nil {
			return nil, fmt.Errorf("scanning lifecycle action: %w", err)
		}
		intent.at = parseTime(created)
		intent.finishedAt = parseTime(updated)
		out[id] = append(out[id], intent)
	}
	reverseGroups(out)
	return out, rows.Err()
}

// maybePruneIncidentTables drops intents and events past retention, at most once
// an hour, so the tables stay bounded without a scheduler of their own.
func (s *Service) maybePruneIncidentTables(ctx context.Context) {
	now := time.Now().UTC()
	last := intentPruneAt.Load()
	if now.Unix()-last < int64(intentPruneEvery.Seconds()) || !intentPruneAt.CompareAndSwap(last, now.Unix()) {
		return
	}
	cutoff := now.Add(-intentRetention).Format(time.RFC3339)
	for _, table := range []string{"plugin_addon_kvm_lifecycle_intents", "plugin_addon_kvm_vm_events"} {
		if _, err := s.db.Exec(ctx, `DELETE FROM `+table+` WHERE created_at < ?`, cutoff); err != nil {
			s.logger.Warn("kvm: pruning incident history failed", "table", table, "error", err)
		}
	}
}

// ensureIncidentSchema creates the intent and event tables on an installation
// that predates migration 0007, since install-time migrations never re-run.
func (s *Service) ensureIncidentSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_lifecycle_intents (
		    id             TEXT PRIMARY KEY,
		    vm_id          TEXT NOT NULL,
		    environment_id TEXT NOT NULL,
		    action         TEXT NOT NULL,
		    actor          TEXT NOT NULL,
		    source         TEXT NOT NULL,
		    outcome        TEXT NOT NULL DEFAULT 'pending',
		    error          TEXT,
		    created_at     TEXT NOT NULL,
		    updated_at     TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_lifecycle_intents_vm
		    ON plugin_addon_kvm_lifecycle_intents(vm_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_lifecycle_intents_created
		    ON plugin_addon_kvm_lifecycle_intents(created_at)`,
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_vm_events (
		    id             TEXT PRIMARY KEY,
		    vm_id          TEXT NOT NULL,
		    environment_id TEXT NOT NULL,
		    kind           TEXT NOT NULL,
		    reason         TEXT,
		    device         TEXT,
		    at             TEXT NOT NULL,
		    created_at     TEXT NOT NULL,
		    updated_at     TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_events_vm
		    ON plugin_addon_kvm_vm_events(vm_id, at)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_events_created
		    ON plugin_addon_kvm_vm_events(created_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("preparing the incident history tables: %w", err)
		}
	}
	return nil
}

// boundText trims, strips control characters and caps a free-text value, because
// actors, errors and QEMU reasons all end up in tables a person reads.
func boundText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	if runes := []rune(value); len(runes) > maxIntentText {
		value = string(runes[:maxIntentText])
	}
	return value
}

// lifecycleIntentAction names the recorded action for a simple lifecycle message.
func lifecycleIntentAction(messageType string) string {
	switch messageType {
	case msgVMReset:
		return intentReset
	case msgVMPause:
		return intentPause
	case msgVMResume:
		return intentResume
	}
	return messageType
}

// reverseGroups turns newest-first rows into oldest-first per guest in place.
func reverseGroups[T any](groups map[string][]T) {
	for _, rows := range groups {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
}
