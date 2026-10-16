// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// SudoWindowTTL is how long a step-up window lasts. Five minutes is long
// enough to finish the action that needed it and short enough that an
// unattended session is not a standing authorisation.
const SudoWindowTTL = 5 * time.Minute

// Sudo scopes. A window is minted for one named action: a window taken to
// delete an environment must not silently authorise a credential disclosure.
const (
	// SudoScopeSecretsReveal gates the secrets vault's reveal endpoint.
	SudoScopeSecretsReveal = "secrets.reveal"
	// SudoScopeSessionTerminate and SudoScopeEnvironmentDelete are the two
	// other call sites this primitive was built in core/auth for rather than
	// inside the secrets module. Neither is wired yet; the constants exist so
	// the next one to need it does not invent a third spelling.
	SudoScopeSessionTerminate  = "sessions.terminate"
	SudoScopeEnvironmentDelete = "environments.delete"
	// SudoScopeBreakGlass gates activating an emergency session, which grants
	// everything: a hijacked session of a designated account must not be enough.
	SudoScopeBreakGlass = "break_glass"
)

// ErrSudoRequired is returned when an action needs a live step-up window and
// the session has none. Callers map it onto a 403 with a code that tells the
// UI to prompt for a password or a TOTP code.
var ErrSudoRequired = errors.New("step-up re-authentication required")

// ErrSudoVerificationFailed is a wrong password or a wrong TOTP code.
var ErrSudoVerificationFailed = errors.New("step-up verification failed")

// TOTPVerifier is the slice of the MFA service the step-up path needs. Narrow
// on purpose, and an interface so core/auth does not import modules/mfa —
// that dependency runs the wrong way.
type TOTPVerifier interface {
	VerifyForLogin(ctx context.Context, userID, code string) error
}

// MintSudoWindow re-verifies the user and opens a step-up window bound to
// their session.
//
// Why this exists alongside the elevation machinery, rather than instead of
// it: an approved elevation is a **time-bounded grant from an approver**, and
// it protects against standing over-privilege and against a single insider
// acting alone. It does nothing about a stolen session cookie belonging to
// someone who happens to hold a live elevation. A sudo window closes exactly
// that gap, because a thief with the cookie still cannot produce the password
// or the second factor. The two are complementary and the reveal endpoint
// requires both.
//
// A TOTP code is preferred where the user has MFA enrolled — it proves
// possession, where a password only proves knowledge of something that may
// already be in the same place the cookie was stolen from.
func (s *Service) MintSudoWindow(
	ctx context.Context, userID, sessionID, scope, password, totpCode string, totp TOTPVerifier,
) (time.Time, error) {
	if userID == "" || sessionID == "" || scope == "" {
		return time.Time{}, ErrSudoVerificationFailed
	}

	// The window is bound to the cookie's session, so that session must belong
	// to the user proving who they are; an API key's owner cannot open a window
	// on somebody else's browser session.
	var owner string
	if err := s.db.QueryRowContext(ctx,
		"SELECT user_id FROM sessions WHERE id = ? AND revoked_at IS NULL", SessionKey(sessionID),
	).Scan(&owner); err != nil || owner != userID {
		return time.Time{}, ErrSudoVerificationFailed
	}
	// A locked account gets no step-up: the lockout exists to stop guessing, and
	// this is a password prompt like the login form.
	if locked, err := s.isLocked(ctx, userID); err != nil {
		return time.Time{}, fmt.Errorf("checking account lockout: %w", err)
	} else if locked > 0 {
		return time.Time{}, ErrSudoVerificationFailed
	}

	method := ""
	switch {
	case totpCode != "" && totp != nil:
		if err := totp.VerifyForLogin(ctx, userID, totpCode); err != nil {
			s.countSudoFailure(ctx, userID)
			return time.Time{}, ErrSudoVerificationFailed
		}
		method = "totp"
	case password != "":
		ok, err := s.verifyUserPassword(ctx, userID, password)
		if err != nil {
			return time.Time{}, err
		}
		if !ok {
			s.countSudoFailure(ctx, userID)
			return time.Time{}, ErrSudoVerificationFailed
		}
		method = "password"
	default:
		return time.Time{}, ErrSudoVerificationFailed
	}

	now := time.Now().UTC()
	expiresAt := now.Add(SudoWindowTTL)

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sudo_windows (id, session_id, user_id, scope, method, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, xid.New().String(), SessionKey(sessionID), userID, scope, method,
		expiresAt.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return time.Time{}, fmt.Errorf("opening step-up window: %w", err)
	}

	// Opportunistic prune so the table cannot grow without bound. A failure
	// here is not worth failing the step-up.
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM sudo_windows WHERE expires_at < ?", now.Add(-time.Hour).Format(time.RFC3339),
	); err != nil {
		slog.Warn("pruning expired sudo windows failed", "error", err)
		return expiresAt, nil
	}

	return expiresAt, nil
}

// HasSudoWindow reports whether the session holds a live window for the scope.
//
// It does not consume the window: a user who reveals two secrets inside five
// minutes should not have to re-authenticate between them, and the audit log
// records each disclosure separately regardless.
func (s *Service) HasSudoWindow(ctx context.Context, sessionID, scope string) (bool, error) {
	if sessionID == "" || scope == "" {
		return false, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sudo_windows
		WHERE session_id = ? AND scope = ? AND expires_at > ?
	`, SessionKey(sessionID), scope, time.Now().UTC().Format(time.RFC3339)).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking step-up window: %w", err)
	}
	return count > 0, nil
}

// RevokeSudoWindows closes every window on a session. Called on logout and
// whenever a session is terminated, so a window cannot outlive the session it
// was bound to even if the row would otherwise survive.
func (s *Service) RevokeSudoWindows(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM sudo_windows WHERE session_id = ?", SessionKey(sessionID))
	if err != nil {
		return fmt.Errorf("revoking step-up windows: %w", err)
	}
	_ = coredb.RowsAffected(result)
	return nil
}

// verifyUserPassword checks a password against the stored Argon2id hash
// without going through Login — a step-up must not touch last_login, must not
// mint a session, and must not be affected by the login lockout counter, which
// is about failed *logins*.
//
// A user with no password hash (an SSO-only account) cannot step up with a
// password and must use TOTP.
func (s *Service) verifyUserPassword(ctx context.Context, userID, password string) (bool, error) {
	var hash string
	err := s.db.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE id = ? AND is_active = 1", userID,
	).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loading user for step-up: %w", err)
	}
	if hash == "" {
		return false, nil
	}
	return VerifyPassword(password, hash)
}

// countSudoFailure charges a failed step-up to the account's lockout counter,
// exactly as a failed login is, so a stolen session cannot guess the password
// here faster than the login form would allow.
func (s *Service) countSudoFailure(ctx context.Context, userID string) {
	var username string
	if err := s.db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", userID).Scan(&username); err != nil {
		return
	}
	s.recordFailedAttempt(ctx, username, LoadLockoutPolicy(ctx, s.db))
}
