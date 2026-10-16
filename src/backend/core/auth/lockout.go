// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/rs/xid"
)

// authSettingsQueryTimeout bounds the small settings reads the lockout and
// password-policy paths make. They run on the login path, so a database that
// has stopped answering has to fail the request rather than hold login open.
const authSettingsQueryTimeout = 5 * time.Second

// LockoutReason is a typed error returned by CheckLockout. The strings
// here are the same values used in the login_attempts.reason column so
// dashboards can aggregate either side.
type LockoutReason string

const (
	// LockoutReasonLocked marks an account inside an active lockout window.
	LockoutReasonLocked LockoutReason = "locked"
	// LockoutReasonExpired marks an account whose lockout window has passed and was cleared.
	LockoutReasonExpired LockoutReason = "expired"
	// LockoutReasonDisabled marks an account an administrator disabled.
	LockoutReasonDisabled LockoutReason = "disabled"
	// LockoutReasonOK marks an account that may attempt a login.
	LockoutReasonOK LockoutReason = "ok"
)

// LockoutPolicy is the runtime-configurable lockout policy. Values
// are read from the settings table at the start of each login attempt
// so admins can change them without restarting the server.
type LockoutPolicy struct {
	MaxAttempts int           // total failed attempts within Window before lock
	Window      time.Duration // sliding window for counting failures
	InitialLock time.Duration // first lock duration (exponential backoff from here)
	MaxLock     time.Duration // cap on lock duration

	// MaxAttemptsPerIP is the failure count from one client address within
	// IPWindow that locks that address for IPLock. Zero disables it.
	MaxAttemptsPerIP int
	IPWindow         time.Duration
	IPLock           time.Duration
}

// DefaultLockoutPolicy matches the values seeded in migrations 065 and 110. The
// per-address allowance is six times the per-account one so an office behind a
// single NAT egress survives it.
func DefaultLockoutPolicy() LockoutPolicy {
	return LockoutPolicy{
		MaxAttempts:      5,
		Window:           15 * time.Minute,
		InitialLock:      1 * time.Minute,
		MaxLock:          1 * time.Hour,
		MaxAttemptsPerIP: 30,
		IPWindow:         15 * time.Minute,
		IPLock:           15 * time.Minute,
	}
}

// LoadLockoutPolicy reads the policy from the settings table. Falls
// back to defaults if any value is missing or unparseable. A nil db
// returns the defaults.
func LoadLockoutPolicy(ctx context.Context, db *sql.DB) LockoutPolicy {
	p := DefaultLockoutPolicy()
	if db == nil {
		return p
	}
	if v, ok := readSettingInt(ctx, db, "lockout_max_attempts"); ok && v > 0 {
		p.MaxAttempts = v
	}
	if v, ok := readSettingInt(ctx, db, "lockout_window_seconds"); ok && v > 0 {
		p.Window = time.Duration(v) * time.Second
	}
	if v, ok := readSettingInt(ctx, db, "lockout_initial_seconds"); ok && v > 0 {
		p.InitialLock = time.Duration(v) * time.Second
	}
	if v, ok := readSettingInt(ctx, db, "lockout_max_lock_seconds"); ok && v > 0 {
		p.MaxLock = time.Duration(v) * time.Second
	}
	if v, ok := readSettingInt(ctx, db, SettingLockoutIPMaxAttempts); ok && v >= 0 {
		p.MaxAttemptsPerIP = v
	}
	if v, ok := readSettingInt(ctx, db, SettingLockoutIPWindowSeconds); ok && v > 0 {
		p.IPWindow = time.Duration(v) * time.Second
	}
	if v, ok := readSettingInt(ctx, db, SettingLockoutIPLockSeconds); ok && v > 0 {
		p.IPLock = time.Duration(v) * time.Second
	}
	return p
}

// CheckLockout returns the lockout state for the user. It is called
// BEFORE VerifyPassword so a locked account never even attempts the
// expensive Argon2 verification (slows down brute-force by ~2 orders
// of magnitude).
//
// Returns (true, reason, retryAfter) when the user is locked.
// Returns (false, "", 0) when the user is fine.
func (s *Service) CheckLockout(username string, policy LockoutPolicy) (bool, LockoutReason, time.Duration) {
	return s.CheckLockoutContext(context.Background(), username, policy)
}

// CheckLockoutContext is CheckLockout bound to the caller's context, so a cancelled request stops the lockout lookup.
func (s *Service) CheckLockoutContext(parent context.Context, username string, policy LockoutPolicy) (bool, LockoutReason, time.Duration) {
	if s == nil || s.db == nil {
		return false, "", 0
	}
	// Defence-in-depth: normalise here too, even though LoginWithContext
	// already does it, so this lookup always matches the login path
	// exactly regardless of caller. Idempotent — safe to call twice.
	username = NormalizeUsername(username)
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()

	var id string
	var lockedUntil sql.NullString
	var failedCount int
	var disabledAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, password_locked_until, password_failed_count, disabled_at
		  FROM users WHERE username = ?
	`, username).Scan(&id, &lockedUntil, &failedCount, &disabledAt)
	if err == sql.ErrNoRows {
		// Unknown user — never lock an account that doesn't exist;
		// just return OK so we go to the password check which will
		// then fail normally. We don't want to leak which usernames
		// exist.
		return false, "", 0
	}
	if err != nil {
		// Be permissive on error — denying users would be worse.
		return false, "", 0
	}
	if disabledAt.Valid && disabledAt.String != "" {
		return true, LockoutReasonDisabled, 0
	}
	if lockedUntil.Valid && lockedUntil.String != "" {
		t, terr := time.Parse(time.RFC3339, lockedUntil.String)
		if terr != nil {
			return false, "", 0
		}
		now := time.Now().UTC()
		if now.Before(t) {
			return true, LockoutReasonLocked, t.Sub(now)
		}
		// Lock expired — clean up the marker so the next login proceeds.
		if _, err := s.db.ExecContext(ctx,
			`UPDATE users SET password_locked_until = NULL WHERE id = ?`, id); err != nil {
			slog.Warn("auth: clearing expired lockout failed", "user", id, "error", err)
		}
	}
	return false, "", 0
}

// RecordFailedAttempt bumps the failed counter and, when the policy
// threshold is reached inside the window, sets password_locked_until.
// Returns the new lock duration (0 if not locked).
func (s *Service) RecordFailedAttempt(username string, policy LockoutPolicy) time.Duration {
	return s.recordFailedAttempt(context.Background(), username, policy)
}

// RecordFailedAttemptContext is RecordFailedAttempt bound to the caller's context, so a cancelled request stops the failure bookkeeping.
func (s *Service) RecordFailedAttemptContext(ctx context.Context, username string, policy LockoutPolicy) time.Duration {
	return s.recordFailedAttempt(ctx, username, policy)
}

// recordFailedAttempt is RecordFailedAttempt with the caller's context.
func (s *Service) recordFailedAttempt(ctx context.Context, username string, policy LockoutPolicy) time.Duration {
	if s == nil || s.db == nil {
		return 0
	}
	// Same defence-in-depth normalisation as CheckLockout — must key
	// on the same canonical username so the failed-attempt counter
	// accumulates against one row, not split across casings.
	username = NormalizeUsername(username)
	ctx, cancel := context.WithTimeout(ctx, authSettingsQueryTimeout)
	defer cancel()

	var id string
	var failedCount int
	var lastFail sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, password_failed_count, password_locked_until
		  FROM users WHERE username = ?
	`, username).Scan(&id, &failedCount, &lastFail)
	if err != nil {
		return 0
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// If the user is already locked, leave the lock alone — the
	// CheckLockout gate prevents reaching here while locked unless
	// the lock has just expired and we are in the same request.
	if lastFail.Valid && lastFail.String != "" {
		t, terr := time.Parse(time.RFC3339, lastFail.String)
		if terr == nil && now.Before(t) {
			return t.Sub(now)
		}
	}

	newCount := failedCount + 1
	// Rolling window: if the last failure is older than the window,
	// reset the counter to 1.
	if failedCount > 0 && lastFail.Valid && lastFail.String != "" {
		t, terr := time.Parse(time.RFC3339, lastFail.String)
		if terr == nil && now.Sub(t) > policy.Window {
			newCount = 1
		}
	}

	if newCount >= policy.MaxAttempts {
		// Exponential backoff: previous lock (if any) doubles, capped at MaxLock.
		var prevLock time.Duration
		if lastFail.Valid && lastFail.String != "" {
			t, terr := time.Parse(time.RFC3339, lastFail.String)
			if terr == nil && now.Before(t) {
				prevLock = t.Sub(now)
			}
		}
		nextLock := policy.InitialLock
		if prevLock > 0 {
			nextLock = prevLock * 2
		}
		if nextLock > policy.MaxLock {
			nextLock = policy.MaxLock
		}
		lockedUntil := now.Add(nextLock).Format(time.RFC3339)
		if _, err := s.db.ExecContext(ctx, `
			UPDATE users
			   SET password_failed_count = ?, password_locked_until = ?,
			       updated_at = ?
			 WHERE id = ?
		`, newCount, lockedUntil, nowStr, id); err != nil {
			slog.Warn("auth: recording lockout failed", "user", id, "error", err)
		}
		return nextLock
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE users
		   SET password_failed_count = ?, password_locked_until = NULL,
		       updated_at = ?
		 WHERE id = ?
	`, newCount, nowStr, id); err != nil {
		slog.Warn("auth: recording failed attempt failed", "user", id, "error", err)
	}
	return 0
}

// ClearFailedAttempts resets the failed-attempt counter and any
// outstanding lock. Called on successful login (already in
// MarkLoginSuccess) and from the admin "unlock account" action.
func (s *Service) ClearFailedAttempts(userID string) {
	s.ClearFailedAttemptsContext(context.Background(), userID)
}

// ClearFailedAttemptsContext is ClearFailedAttempts bound to the caller's context, so a cancelled request stops the counter reset.
func (s *Service) ClearFailedAttemptsContext(parent context.Context, userID string) {
	if s == nil || s.db == nil || userID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE users
		   SET password_failed_count = 0,
		       password_locked_until = NULL,
		       updated_at = ?
		 WHERE id = ?
	`, time.Now().UTC().Format(time.RFC3339), userID); err != nil {
		slog.Warn("auth: clearing failed attempts failed", "user", userID, "error", err)
	}
}

// readSettingInt reads a settings row that contains an integer value.
// Returns (value, true) when the row exists and parses cleanly.
func readSettingInt(ctx context.Context, db *sql.DB, key string) (int, bool) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

// recordLoginAttempt inserts a forensic row in login_attempts. Called
// from LoginWithContext on every attempt — success, invalid creds,
// disabled, or lockout — so a security dashboard can correlate the
// audit log with the IP/User-Agent trail. Errors are intentionally
// swallowed; this is observability, not a critical path. The row is also
// what RecordIPFailure counts, so it must be written before that call.
func (s *Service) recordLoginAttempt(ctx context.Context, username string, client LoginClient, reason string, succeeded bool, userID string) {
	if s == nil || s.db == nil {
		return
	}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	var uid interface{}
	if userID != "" {
		uid = userID
	}
	var uname interface{}
	if username != "" {
		uname = username
	}
	ip := client.IP
	if ip == "" {
		ip = "unknown"
	}
	ua := client.UserAgent
	if len(ua) > 256 {
		ua = ua[:256]
	}
	succeededInt := 0
	if succeeded {
		succeededInt = 1
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO login_attempts (id, username, ip_address, ip_source, succeeded, user_id, user_agent, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, uname, ip, client.Source, succeededInt, uid, ua, reason, now); err != nil {
		slog.Warn("auth: recording login attempt failed", "error", err)
	}
}

// IsLocked returns the remaining lock duration for the user. A nil
// or zero duration means the user is not locked. Used by the admin
// dialog to display the unlock button.
func (s *Service) IsLocked(userID string) (time.Duration, error) {
	return s.isLocked(context.Background(), userID)
}

// IsLockedContext is IsLocked bound to the caller's context, so a cancelled request stops the lock lookup.
func (s *Service) IsLockedContext(ctx context.Context, userID string) (time.Duration, error) {
	return s.isLocked(ctx, userID)
}

// isLocked is IsLocked with the caller's context.
func (s *Service) isLocked(ctx context.Context, userID string) (time.Duration, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("service not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, authSettingsQueryTimeout)
	defer cancel()
	var lockedUntil sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT password_locked_until FROM users WHERE id = ?`, userID).Scan(&lockedUntil)
	if err != nil {
		return 0, err
	}
	if !lockedUntil.Valid || lockedUntil.String == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, lockedUntil.String)
	if err != nil {
		return 0, nil
	}
	rem := time.Until(t)
	if rem < 0 {
		return 0, nil
	}
	return rem, nil
}
