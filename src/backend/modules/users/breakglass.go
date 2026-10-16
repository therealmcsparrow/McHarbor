// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Break-glass limits. The window is short on purpose: an emergency grant that
// outlives the emergency is a standing wildcard with a better story.
const (
	BreakGlassMinMinutes     = 5
	BreakGlassMaxMinutes     = 240
	BreakGlassDefaultMinutes = 60
	BreakGlassMinReason      = 10
)

// Break-glass errors.
var (
	ErrBreakGlassNotDesignated = errors.New("user is not a break-glass account")
	ErrBreakGlassInvalid       = errors.New("break-glass request is invalid")
	ErrBreakGlassActive        = errors.New("a break-glass session is already open")
	ErrBreakGlassNoSession     = errors.New("no open break-glass session")
)

// BreakGlassAccount is a user an admin has designated for emergency access.
type BreakGlassAccount struct {
	ID           string             `json:"id"`
	UserID       string             `json:"userId"`
	Username     string             `json:"username"`
	MaxMinutes   int                `json:"maxMinutes"`
	Note         string             `json:"note"`
	DesignatedBy string             `json:"designatedBy"`
	CreatedAt    string             `json:"createdAt"`
	Active       *BreakGlassSession `json:"active"`
}

// BreakGlassSession is one use of emergency access.
type BreakGlassSession struct {
	ID        string  `json:"id"`
	UserID    string  `json:"userId"`
	Username  string  `json:"username"`
	Reason    string  `json:"reason"`
	StartedAt string  `json:"startedAt"`
	ExpiresAt string  `json:"expiresAt"`
	EndedAt   *string `json:"endedAt"`
	EndedBy   *string `json:"endedBy"`
}

// BreakGlassStatus is what the caller's own break-glass panel shows.
type BreakGlassStatus struct {
	Designated bool               `json:"designated"`
	MaxMinutes int                `json:"maxMinutes"`
	Active     *BreakGlassSession `json:"active"`
}

// DesignateInput is the body of PUT /break-glass/accounts/{userId}.
type DesignateInput struct {
	MaxMinutes int    `json:"maxMinutes"`
	Note       string `json:"note"`
}

// ActivateInput is the body of POST /break-glass/activate.
type ActivateInput struct {
	Reason  string `json:"reason"`
	Minutes int    `json:"minutes"`
}

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339) }

const sessionColumns = `g.id, g.user_id, COALESCE(u.username, ''), g.reason, g.started_at, g.expires_at, g.ended_at, g.ended_by`

func scanSession(row interface{ Scan(...any) error }) (*BreakGlassSession, error) {
	var s BreakGlassSession
	var ended, by sql.NullString
	if err := row.Scan(&s.ID, &s.UserID, &s.Username, &s.Reason, &s.StartedAt, &s.ExpiresAt, &ended, &by); err != nil {
		return nil, err
	}
	if ended.Valid {
		s.EndedAt = &ended.String
	}
	if by.Valid {
		s.EndedBy = &by.String
	}
	return &s, nil
}

// openSession returns the user's open, unexpired session, or nil.
func (s *Service) openSession(ctx context.Context, userID string) (*BreakGlassSession, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM break_glass_sessions g
		LEFT JOIN users u ON u.id = g.user_id
		WHERE g.user_id = ? AND g.ended_at IS NULL AND g.expires_at > ? ORDER BY g.started_at DESC LIMIT 1`, userID, nowRFC()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading break-glass session: %w", err)
	}
	return sess, nil
}

// BreakGlassAccounts lists designated accounts with any open session.
func (s *Service) BreakGlassAccounts(ctx context.Context) ([]BreakGlassAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.id, b.user_id, COALESCE(u.username, ''), b.max_minutes, b.note,
		COALESCE(b.designated_by, ''), b.created_at FROM break_glass_accounts b LEFT JOIN users u ON u.id = b.user_id
		ORDER BY u.username LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing break-glass accounts: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	out := []BreakGlassAccount{}
	for rows.Next() {
		var a BreakGlassAccount
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.MaxMinutes, &a.Note, &a.DesignatedBy, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning break-glass account: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Active, err = s.openSession(ctx, out[i].UserID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Designate marks userID as a break-glass account, or updates its limit.
func (s *Service) Designate(ctx context.Context, userID, by string, in DesignateInput) error {
	if in.MaxMinutes == 0 {
		in.MaxMinutes = BreakGlassDefaultMinutes
	}
	if in.MaxMinutes < BreakGlassMinMinutes || in.MaxMinutes > BreakGlassMaxMinutes {
		return ErrBreakGlassInvalid
	}
	exists, err := s.Exists(ctx, userID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
	}
	now := nowRFC()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO break_glass_accounts (id, user_id, max_minutes, note, designated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET max_minutes = EXCLUDED.max_minutes, note = EXCLUDED.note, updated_at = EXCLUDED.updated_at`,
		xid.New().String(), userID, in.MaxMinutes, strings.TrimSpace(in.Note), by, now, now); err != nil {
		return fmt.Errorf("designating break-glass account: %w", err)
	}
	return nil
}

// Undesignate removes the designation and ends any open session with it.
func (s *Service) Undesignate(ctx context.Context, userID, by string) error {
	return db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM break_glass_accounts WHERE user_id = ?`, userID)
		if err != nil {
			return fmt.Errorf("removing break-glass designation: %w", err)
		}
		if db.RowsAffected(res) == 0 {
			return ErrBreakGlassNotDesignated
		}
		now := nowRFC()
		if _, err := tx.ExecContext(ctx, `UPDATE break_glass_sessions SET ended_at = ?, ended_by = ?, updated_at = ?
			WHERE user_id = ? AND ended_at IS NULL`, now, by, now, userID); err != nil {
			return fmt.Errorf("ending break-glass sessions: %w", err)
		}
		return nil
	})
}

// BreakGlassStatusFor reports whether userID may activate and whether they have.
func (s *Service) BreakGlassStatusFor(ctx context.Context, userID string) (*BreakGlassStatus, error) {
	st := &BreakGlassStatus{}
	err := s.db.QueryRowContext(ctx, `SELECT max_minutes FROM break_glass_accounts WHERE user_id = ?`, userID).Scan(&st.MaxMinutes)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("reading break-glass designation: %w", err)
	}
	st.Designated = err == nil
	if st.Active, err = s.openSession(ctx, userID); err != nil {
		return nil, err
	}
	return st, nil
}

// ActivateBreakGlass opens an emergency session for a designated account. The
// reason is mandatory and the window is capped by the account's designation.
func (s *Service) ActivateBreakGlass(ctx context.Context, userID string, in ActivateInput) (*BreakGlassSession, error) {
	st, err := s.BreakGlassStatusFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !st.Designated {
		return nil, ErrBreakGlassNotDesignated
	}
	if st.Active != nil {
		return nil, ErrBreakGlassActive
	}
	reason := strings.TrimSpace(in.Reason)
	if len(reason) < BreakGlassMinReason {
		return nil, ErrBreakGlassInvalid
	}
	minutes := in.Minutes
	if minutes == 0 {
		minutes = st.MaxMinutes
	}
	if minutes < BreakGlassMinMinutes || minutes > st.MaxMinutes {
		return nil, ErrBreakGlassInvalid
	}
	now := time.Now().UTC()
	id := xid.New().String()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO break_glass_sessions (id, user_id, reason, started_at, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, userID, reason, now.Format(time.RFC3339),
		now.Add(time.Duration(minutes)*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		// Two concurrent activations both pass the check above; the unique index
		// on open sessions rejects the second, which is "already active".
		if open, openErr := s.openSession(ctx, userID); openErr == nil && open != nil {
			return nil, ErrBreakGlassActive
		}
		return nil, fmt.Errorf("opening break-glass session: %w", err)
	}
	return s.openSession(ctx, userID)
}

// EndBreakGlass closes the open session of userID; by records who closed it.
func (s *Service) EndBreakGlass(ctx context.Context, userID, by string) (*BreakGlassSession, error) {
	sess, err := s.openSession(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, ErrBreakGlassNoSession
	}
	now := nowRFC()
	if _, err := s.db.ExecContext(ctx, `UPDATE break_glass_sessions SET ended_at = ?, ended_by = ?, updated_at = ? WHERE id = ?`,
		now, by, now, sess.ID); err != nil {
		return nil, fmt.Errorf("ending break-glass session: %w", err)
	}
	sess.EndedAt, sess.EndedBy = &now, &by
	return sess, nil
}

// BreakGlassSessions returns recent sessions, newest first, for review.
func (s *Service) BreakGlassSessions(ctx context.Context, limit int) ([]BreakGlassSession, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM break_glass_sessions g
		LEFT JOIN users u ON u.id = g.user_id ORDER BY g.started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing break-glass sessions: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	out := []BreakGlassSession{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning break-glass session: %w", err)
		}
		out = append(out, *sess)
	}
	return out, rows.Err()
}
