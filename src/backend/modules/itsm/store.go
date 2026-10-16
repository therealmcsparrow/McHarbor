// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package itsm keeps McHarbor's alerts and an ITSM's tickets in step: it owns
// the mapping from an alert condition to a remote ticket, delivers ticket
// operations through a durable ordered outbox, and applies what the ITSM
// reports back by webhook or poll.
package itsm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Link states. Pending has no external ticket yet; remote_closed means the
// ITSM closed it while the alert still fires; orphaned links are never sent again.
const (
	LinkPending      = "pending"
	LinkOpen         = "open"
	LinkRemoteClosed = "remote_closed"
	LinkResolved     = "resolved"
	LinkOrphaned     = "orphaned"
)

// Outbox operations.
const (
	OpOpen    = "open"
	OpUpdate  = "update"
	OpResolve = "resolve"
)

// ErrNoLink is returned when a lookup finds no link.
var ErrNoLink = errors.New("itsm: link not found")

// Link is one McHarbor condition bound to one remote ticket on one channel.
type Link struct {
	ID             string `json:"id"`
	ChannelID      string `json:"channelId"`
	ChannelType    string `json:"channelType"`
	DedupKey       string `json:"dedupKey"`
	AlertID        string `json:"alertId"`
	EnvironmentID  string `json:"environmentId"`
	SubjectID      string `json:"subjectId"`
	ExternalID     string `json:"externalId"`
	ExternalNumber string `json:"externalNumber"`
	ExternalURL    string `json:"externalUrl"`
	LocalState     string `json:"localState"`
	RemoteState    string `json:"remoteState"`
	RemoteAssignee string `json:"remoteAssignee"`
	LastError      string `json:"lastError"`
	OpenedAt       string `json:"openedAt"`
	ResolvedAt     string `json:"resolvedAt"`
	LastSyncedAt   string `json:"lastSyncedAt"`
	UpdatedAt      string `json:"updatedAt"`
	Parked         bool   `json:"parked"`
	AlertName      string `json:"alertName,omitempty"`
	ChannelName    string `json:"channelName,omitempty"`
}

// Store persists links and the outbox; it is the only code that writes them.
type Store struct {
	db  *sql.DB
	now func() time.Time
	mu  sync.Mutex
}

// NewStore returns a store over database.
func NewStore(database *sql.DB) *Store {
	return &Store{db: database, now: time.Now}
}

func (s *Store) stamp() string { return s.now().UTC().Format(time.RFC3339) }

const linkColumns = `l.id, l.channel_id, l.channel_type, l.dedup_key, l.alert_id, l.environment_id, l.subject_id,
	l.external_id, l.external_number, l.external_url, l.local_state, l.remote_state, l.remote_assignee,
	l.last_error, COALESCE(l.opened_at, ''), COALESCE(l.resolved_at, ''), COALESCE(l.last_synced_at, ''), l.updated_at`

func scanLink(row interface{ Scan(...any) error }) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.ChannelID, &l.ChannelType, &l.DedupKey, &l.AlertID, &l.EnvironmentID,
		&l.SubjectID, &l.ExternalID, &l.ExternalNumber, &l.ExternalURL, &l.LocalState, &l.RemoteState,
		&l.RemoteAssignee, &l.LastError, &l.OpenedAt, &l.ResolvedAt, &l.LastSyncedAt, &l.UpdatedAt)
	return l, err
}

func (s *Store) one(ctx context.Context, where string, args ...any) (Link, error) {
	l, err := scanLink(s.db.QueryRowContext(ctx, `SELECT `+linkColumns+` FROM incident_links l WHERE `+where+` LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNoLink
	}
	if err != nil {
		return Link{}, fmt.Errorf("itsm: load link: %w", err)
	}
	return l, nil
}

// EnsureLink returns the live link for (channel, key), creating a pending one
// when none is live; created tells the caller an open must be queued. It is
// serialised in-process because MySQL cannot enforce the partial unique index.
func (s *Store) EnsureLink(ctx context.Context, channelID, channelType, key, alertID, envID, subjectID string) (Link, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.one(ctx, `l.channel_id = ? AND l.dedup_key = ? AND l.local_state IN ('pending', 'open', 'remote_closed')`, channelID, key)
	if err == nil {
		return l, false, nil
	}
	if !errors.Is(err, ErrNoLink) {
		return Link{}, false, err
	}
	id := xid.New().String()
	now := s.stamp()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO incident_links
		(id, channel_id, channel_type, dedup_key, alert_id, environment_id, subject_id, local_state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, id, channelID, channelType, key, alertID, envID, subjectID, now, now); err != nil {
		return Link{}, false, fmt.Errorf("itsm: insert link: %w", err)
	}
	l, err = s.LinkByID(ctx, id)
	return l, err == nil, err
}

// LiveLink returns the live link for (channel, key), or ErrNoLink.
func (s *Store) LiveLink(ctx context.Context, channelID, key string) (Link, error) {
	return s.one(ctx, `l.channel_id = ? AND l.dedup_key = ? AND l.local_state IN ('pending', 'open', 'remote_closed')`, channelID, key)
}

// LinkByID returns one link.
func (s *Store) LinkByID(ctx context.Context, id string) (Link, error) {
	return s.one(ctx, `l.id = ?`, id)
}

// LinkByExternal finds a link by the vendor's ticket id.
func (s *Store) LinkByExternal(ctx context.Context, channelID, externalID string) (Link, error) {
	return s.one(ctx, `l.channel_id = ? AND l.external_id = ? ORDER BY l.created_at DESC`, channelID, externalID)
}

// LinkByKey finds the newest link for a dedup key.
func (s *Store) LinkByKey(ctx context.Context, channelID, key string) (Link, error) {
	return s.one(ctx, `l.channel_id = ? AND l.dedup_key = ? ORDER BY l.created_at DESC`, channelID, key)
}

func (s *Store) update(ctx context.Context, q string, args ...any) error {
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("itsm: update link: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrNoLink
	}
	return nil
}

// MarkOpened records the ticket a pending link opened.
func (s *Store) MarkOpened(ctx context.Context, id string, ref incident.Ref, lastError string) error {
	now := s.stamp()
	return s.update(ctx, `UPDATE incident_links SET external_id = ?, external_number = ?, external_url = ?,
		local_state = CASE WHEN local_state = 'pending' THEN 'open' ELSE local_state END,
		opened_at = COALESCE(opened_at, ?), last_error = ?, updated_at = ? WHERE id = ?`,
		ref.ExternalID, ref.Number, ref.URL, now, lastError, now, id)
}

// MarkResolved closes the local side so the next firing gets a fresh ticket.
func (s *Store) MarkResolved(ctx context.Context, id string) error {
	now := s.stamp()
	return s.update(ctx, `UPDATE incident_links SET local_state = 'resolved', resolved_at = ?, last_error = '', updated_at = ? WHERE id = ?`, now, now, id)
}

// MarkOrphaned stops a link for good: its channel is gone or the ticket was deleted.
func (s *Store) MarkOrphaned(ctx context.Context, id string) error {
	return s.update(ctx, `UPDATE incident_links SET local_state = 'orphaned', updated_at = ? WHERE id = ?`, s.stamp(), id)
}

// SetError records what the operator should see about a link's last delivery.
func (s *Store) SetError(ctx context.Context, id, msg string) error {
	return s.update(ctx, `UPDATE incident_links SET last_error = ?, updated_at = ? WHERE id = ?`, msg, s.stamp(), id)
}

// MarkRemote applies a remote status, backfilling the ticket reference, and
// moves an open link to remote_closed when the ITSM closed the ticket.
func (s *Store) MarkRemote(ctx context.Context, id string, st incident.RemoteStatus, alertStillFiring bool) error {
	now := s.stamp()
	next := ""
	if st.State.Terminal() {
		next = LinkResolved
		if alertStillFiring {
			next = LinkRemoteClosed
		}
	}
	return s.update(ctx, `UPDATE incident_links SET remote_state = ?, remote_assignee = ?, last_synced_at = ?,
		external_id = CASE WHEN external_id = '' THEN ? ELSE external_id END,
		external_number = CASE WHEN external_number = '' THEN ? ELSE external_number END,
		external_url = CASE WHEN external_url = '' THEN ? ELSE external_url END,
		local_state = CASE WHEN ? <> '' AND local_state IN ('pending', 'open') THEN ? ELSE local_state END,
		resolved_at = CASE WHEN ? = 'resolved' AND local_state IN ('pending', 'open') THEN ? ELSE resolved_at END,
		updated_at = ? WHERE id = ?`,
		string(st.State), st.Assignee, st.UpdatedAt.UTC().Format(time.RFC3339),
		st.Ref.ExternalID, st.Ref.Number, st.Ref.URL, next, next, next, now, now, id)
}

// OpenLinks returns a channel's links a poll should look at.
func (s *Store) OpenLinks(ctx context.Context, channelID string, limit int) ([]Link, error) {
	return s.list(ctx, `l.channel_id = ? AND l.local_state IN ('open', 'remote_closed') AND l.external_id <> ''
		ORDER BY l.updated_at`, limit, channelID)
}

// list runs the link query with a caller-built WHERE and always caps the result at limit.
func (s *Store) list(ctx context.Context, where string, limit int, args ...any) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+linkColumns+` FROM incident_links l WHERE `+where+` LIMIT ?`, append(args, limit)...) // SQL identifier: where is a constant clause written by the store's own callers, never request input
	if err != nil {
		return nil, fmt.Errorf("itsm: list links: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Link{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("itsm: scan link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListFilter narrows the linked-ticket table.
type ListFilter struct {
	State     string
	ChannelID string
	EnvID     string
}

// List returns links newest first with the alert and channel names joined in.
func (s *Store) List(ctx context.Context, f ListFilter, page, perPage int) ([]Link, int64, error) {
	where, args := " WHERE 1 = 1", []any{}
	if f.State != "" {
		where += " AND l.local_state = ?"
		args = append(args, f.State)
	}
	if f.ChannelID != "" {
		where += " AND l.channel_id = ?"
		args = append(args, f.ChannelID)
	}
	if f.EnvID != "" {
		where += " AND l.environment_id = ?"
		args = append(args, f.EnvID)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM incident_links l`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("itsm: count links: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+linkColumns+`, COALESCE(a.name, ''), COALESCE(c.name, ''),`+ // SQL identifier: linkColumns is a constant and where holds fixed AND clauses, every value is bound
		` EXISTS (SELECT 1 FROM incident_outbox o WHERE o.link_id = l.id AND o.parked = 1)
		FROM incident_links l LEFT JOIN alerts a ON a.id = l.alert_id LEFT JOIN communication_channels c ON c.id = l.channel_id`+
		where+` ORDER BY l.updated_at DESC, l.id DESC LIMIT ? OFFSET ?`, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, fmt.Errorf("itsm: list links: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Link{}
	for rows.Next() {
		var l Link
		var parked int
		if err := rows.Scan(&l.ID, &l.ChannelID, &l.ChannelType, &l.DedupKey, &l.AlertID, &l.EnvironmentID,
			&l.SubjectID, &l.ExternalID, &l.ExternalNumber, &l.ExternalURL, &l.LocalState, &l.RemoteState,
			&l.RemoteAssignee, &l.LastError, &l.OpenedAt, &l.ResolvedAt, &l.LastSyncedAt, &l.UpdatedAt,
			&l.AlertName, &l.ChannelName, &parked); err != nil {
			return nil, 0, fmt.Errorf("itsm: scan link: %w", err)
		}
		l.Parked = parked != 0
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// OutboxRow is one queued ticket operation.
type OutboxRow struct {
	ID        string
	LinkID    string
	ChannelID string
	Op        string
	Event     incident.Event
	Seq       int
	Attempts  int
}

// Enqueue appends an operation behind every earlier one for the same link.
func (s *Store) Enqueue(ctx context.Context, linkID, channelID, op string, ev incident.Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("itsm: encode event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var seq int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM incident_outbox WHERE link_id = ?`, linkID).Scan(&seq); err != nil {
		return fmt.Errorf("itsm: read sequence: %w", err)
	}
	now := s.stamp()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO incident_outbox (id, link_id, channel_id, op, event_json, seq, next_attempt_at,
		created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), linkID, channelID, op, string(raw), seq+1, now, now, now); err != nil {
		return fmt.Errorf("itsm: enqueue: %w", err)
	}
	return nil
}

// Heads returns, per link, the lowest-sequence unparked row that is due — so
// a later operation never overtakes an earlier one still waiting to retry.
func (s *Store) Heads(ctx context.Context, now time.Time, limit int) ([]OutboxRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT o.id, o.link_id, o.channel_id, o.op, o.event_json, o.seq, o.attempts
		FROM incident_outbox o
		WHERE o.parked = 0 AND o.seq = (SELECT MIN(i.seq) FROM incident_outbox i WHERE i.link_id = o.link_id AND i.parked = 0)
		  AND NOT EXISTS (SELECT 1 FROM incident_outbox p WHERE p.link_id = o.link_id AND p.parked = 1)
		  AND o.next_attempt_at <= ?
		ORDER BY o.next_attempt_at, o.link_id LIMIT ?`, now.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("itsm: read outbox: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []OutboxRow
	for rows.Next() {
		var r OutboxRow
		var raw string
		if err := rows.Scan(&r.ID, &r.LinkID, &r.ChannelID, &r.Op, &raw, &r.Seq, &r.Attempts); err != nil {
			return nil, fmt.Errorf("itsm: scan outbox: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &r.Event); err != nil {
			return nil, fmt.Errorf("itsm: decode event: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Done removes a delivered row.
func (s *Store) Done(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM incident_outbox WHERE id = ?`, id); err != nil {
		return fmt.Errorf("itsm: remove outbox row: %w", err)
	}
	return nil
}

// Retry schedules a row again after a transient failure.
func (s *Store) Retry(ctx context.Context, id string, attempts int, next time.Time, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incident_outbox SET attempts = ?, next_attempt_at = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		attempts, next.UTC().Format(time.RFC3339), msg, s.stamp(), id)
	if err != nil {
		return fmt.Errorf("itsm: reschedule: %w", err)
	}
	return nil
}

// Park stops retrying a row until an operator asks for it.
func (s *Store) Park(ctx context.Context, id, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incident_outbox SET parked = 1, last_error = ?, updated_at = ? WHERE id = ?`, msg, s.stamp(), id)
	if err != nil {
		return fmt.Errorf("itsm: park: %w", err)
	}
	return nil
}

// Unpark makes a link's parked rows due again now.
func (s *Store) Unpark(ctx context.Context, linkID string) (int, error) {
	now := s.stamp()
	res, err := s.db.ExecContext(ctx, `UPDATE incident_outbox SET parked = 0, attempts = 0, next_attempt_at = ?, updated_at = ? WHERE link_id = ? AND parked = 1`,
		now, now, linkID)
	if err != nil {
		return 0, fmt.Errorf("itsm: unpark: %w", err)
	}
	return int(db.RowsAffected(res)), nil
}

// DropQueue removes every queued operation of a link.
func (s *Store) DropQueue(ctx context.Context, linkID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM incident_outbox WHERE link_id = ?`, linkID); err != nil {
		return fmt.Errorf("itsm: drop queue: %w", err)
	}
	return nil
}

// Postpone moves a row behind the others without counting an attempt, used
// when an update or resolve arrives before its ticket exists.
func (s *Store) Postpone(ctx context.Context, id string, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incident_outbox SET next_attempt_at = ?, updated_at = ? WHERE id = ?`,
		next.UTC().Format(time.RFC3339), s.stamp(), id)
	if err != nil {
		return fmt.Errorf("itsm: postpone: %w", err)
	}
	return nil
}

// OrphanDeletedChannels marks the live links of channels that no longer exist
// orphaned and drops their queue, so a deleted channel is never contacted again.
func (s *Store) OrphanDeletedChannels(ctx context.Context) (int, error) {
	now := s.stamp()
	res, err := s.db.ExecContext(ctx, `UPDATE incident_links SET local_state = 'orphaned', updated_at = ?
		WHERE local_state IN ('pending', 'open', 'remote_closed')
		  AND channel_id NOT IN (SELECT id FROM communication_channels)`, now)
	if err != nil {
		return 0, fmt.Errorf("itsm: orphan links: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM incident_outbox WHERE channel_id NOT IN (SELECT id FROM communication_channels)`); err != nil {
		return 0, fmt.Errorf("itsm: drop orphaned queue: %w", err)
	}
	return int(db.RowsAffected(res)), nil
}
