// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// alertState is one subject's firing state for one rule.
type alertState struct {
	Key            string
	AlertID        string
	Active         bool
	ActiveSince    time.Time
	LastNotifiedAt time.Time
	LastSeen       time.Time
	// Acknowledged is set when an ITSM reported someone is on it; a still
	// firing condition then stops re-notifying on cooldown.
	Acknowledged bool
}

// notified reports whether anyone was actually told about this state.
//
// It is the flag that separates "the condition is true but has not
// held long enough yet" from "we sent something". Without it, a rule
// with a five-minute sustain window would send a recovery message for
// a blip that never fired in the first place.
func (s alertState) notified() bool { return !s.LastNotifiedAt.IsZero() }

// stateStore persists firing state.
//
// This used to be a map on the Engine struct, which had two failure
// modes worth fixing: a restart re-notified every still-firing
// condition, and in an HA cluster a leader failover did the same
// because the new leader started with an empty map. Cooldown and
// sustain windows both need to outlive the process anyway.
type stateStore struct {
	db *sql.DB
}

func newStateStore(db *sql.DB) *stateStore { return &stateStore{db: db} }

// loadForAlert returns every stored state for one rule, keyed by
// state_key.
func (s *stateStore) loadForAlert(ctx context.Context, alertID string) (map[string]alertState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT state_key, alert_id, active, COALESCE(active_since, ''), COALESCE(last_notified_at, ''), last_seen,
		        CASE WHEN acknowledged_at IS NULL THEN 0 ELSE 1 END
		   FROM alert_states
		  WHERE alert_id = ?
		  LIMIT 1000`, alertID)
	if err != nil {
		return nil, fmt.Errorf("loading alert states: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing alert state rows failed", "error", err)
		}
	}()
	out := make(map[string]alertState, 32)
	for rows.Next() {
		var state alertState
		var activeSince, lastNotified, lastSeen string
		var acknowledged int
		if err := rows.Scan(&state.Key, &state.AlertID, &state.Active, &activeSince, &lastNotified, &lastSeen, &acknowledged); err != nil {
			return nil, fmt.Errorf("scanning alert state: %w", err)
		}

		state.ActiveSince = parseDBTimeOrZero(activeSince)
		state.LastNotifiedAt = parseDBTimeOrZero(lastNotified)
		state.LastSeen = parseDBTimeOrZero(lastSeen)
		state.Acknowledged = acknowledged != 0
		out[state.Key] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating alert states: %w", err)
	}

	return out, nil
}

// save upserts one state row.
func (s *stateStore) save(ctx context.Context, state alertState) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO alert_states (state_key, alert_id, active, active_since, last_notified_at, last_seen, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(state_key) DO UPDATE SET
		     active           = excluded.active,
		     active_since     = excluded.active_since,
		     last_notified_at = excluded.last_notified_at,
		     last_seen        = excluded.last_seen,
		     updated_at       = excluded.updated_at`,
		state.Key, state.AlertID, state.Active,
		formatDBTime(state.ActiveSince), formatDBTime(state.LastNotifiedAt),
		formatDBTime(state.LastSeen), formatDBTime(state.LastSeen), formatDBTime(state.LastSeen))
	if err != nil {
		return fmt.Errorf("saving alert state %s: %w", state.Key, err)
	}
	if !state.Active {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE alert_states SET acknowledged_at = NULL, acknowledged_by = '' WHERE state_key = ? AND acknowledged_at IS NOT NULL`,
			state.Key); err != nil {
			return fmt.Errorf("clearing acknowledgement of %s: %w", state.Key, err)
		}
	}
	return nil
}

// deleteKeys removes states for subjects that no longer exist.
func (s *stateStore) deleteKeys(ctx context.Context, keys []string) error {
	// bounded by the stale subject keys of one evaluation pass; each delete is idempotent
	for _, key := range keys {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM alert_states WHERE state_key = ?", key); err != nil {
			return fmt.Errorf("deleting alert state %s: %w", key, err)
		}
	}
	return nil
}

// deleteForAlert removes every state belonging to a rule. Called when
// a rule is deleted or disabled so re-enabling it starts clean rather
// than resolving conditions nobody was watching.
func (s *stateStore) deleteForAlert(ctx context.Context, alertID string) error {
	// Dropped after the delete, so a concurrent cold load cannot resurrect the rows.
	defer sharedStateCache.drop(alertID)
	if _, err := s.db.ExecContext(ctx, "DELETE FROM alert_states WHERE alert_id = ?", alertID); err != nil {
		return fmt.Errorf("deleting alert states for %s: %w", alertID, err)
	}
	return nil
}

// prune drops states not seen since the cutoff. A subject that stopped
// being reported has already been resolved; this is only cleanup.
func (s *stateStore) prune(ctx context.Context, cutoff time.Time) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM alert_states WHERE last_seen < ?", formatDBTime(cutoff)); err != nil {
		return fmt.Errorf("pruning alert states: %w", err)
	}
	return nil
}

// stateKey identifies one subject of one rule in one environment.
// Environment is empty for host-wide conditions.
func stateKey(alertID, environmentID, subjectID string) string {
	return alertID + ":" + environmentID + ":" + subjectID
}

// decision is what the state machine concluded for one observation.
type decision struct {
	// Next is the state to persist.
	Next alertState

	// Notify is true when something should be delivered.
	Notify bool

	// State is EventStateFiring or EventStateResolved, meaningful
	// only when Notify is true.
	State string
}

// decide runs the firing state machine for one subject.
//
// The rules, in the order they matter:
//
//  1. A condition that is true but has not held for `sustain` yet is
//     recorded, not delivered. This is what makes "CPU > 80% for 5m"
//     mean what it says instead of firing on a single spike.
//  2. A condition that has held long enough delivers once. It
//     delivers again only if a cooldown is configured and has
//     elapsed — cooldown 0 keeps the previous behaviour of one
//     notification per episode.
//  3. A condition that goes false delivers a recovery only if
//     something was actually delivered for it, and only when the rule
//     asks for recovery messages.
//
// observedSince lets a source that knows when a state began (a
// container's FinishedAt, a backup's last success) override the
// engine's own first-seen time. It is recomputed every tick, which is
// what lets "no successful backup for 24h" resolve the moment a backup
// succeeds.
func decide(prev alertState, triggered bool, observedSince, now time.Time, sustain, cooldown time.Duration, notifyOnRecovery bool) decision {
	next := prev
	next.LastSeen = now

	if !triggered {
		next.Active = false
		next.ActiveSince = time.Time{}
		next.LastNotifiedAt = time.Time{}

		if prev.Active && prev.notified() && notifyOnRecovery {
			return decision{Next: next, Notify: true, State: EventStateResolved}
		}
		return decision{Next: next}
	}

	activeSince := observedSince
	if activeSince.IsZero() {
		activeSince = prev.ActiveSince
	}
	if activeSince.IsZero() || !prev.Active {
		if observedSince.IsZero() {
			// Nothing known about when this started, so it starts
			// now. Later than the truth, never earlier — the safe
			// direction, because the alternative fires on a window
			// that was never observed.
			activeSince = now
		}
	}

	next.Active = true
	next.ActiveSince = activeSince

	if sustain > 0 && now.Sub(activeSince) < sustain {
		// Still inside the sustain window. Clear any previous
		// notification marker so that when it does fire, it fires as
		// a fresh incident rather than as a cooldown repeat.
		if !prev.Active {
			next.LastNotifiedAt = time.Time{}
		}
		return decision{Next: next}
	}

	if !prev.notified() {
		next.LastNotifiedAt = now
		return decision{Next: next, Notify: true, State: EventStateFiring}
	}

	if cooldown > 0 && now.Sub(prev.LastNotifiedAt) >= cooldown && !prev.Acknowledged {
		next.LastNotifiedAt = now
		return decision{Next: next, Notify: true, State: EventStateFiring}
	}

	return decision{Next: next}
}

// formatDBTime renders a timestamp for storage, matching the RFC3339
// UTC shape every other module writes. A zero time stores as empty
// rather than year 1, so a NULL-ish read is unambiguous.
func formatDBTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// normalizeSeverity keeps an unrecognised severity from silently
// becoming something else.
func normalizeSeverity(severity string) string {
	trimmed := strings.ToLower(strings.TrimSpace(severity))
	if validAlertSeverities[trimmed] {
		return trimmed
	}
	return "warning"
}
