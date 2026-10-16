// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package users manages user accounts: lifecycle, sessions, role and override assignment, break-glass access and password resets.
package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Phase 4: account management + reset flow.
//
// All methods in this file are additive — the existing user CRUD in
// service.go is preserved. The new endpoints are exposed under
// /users/{id}/{disable,enable,force-reset,sessions,...} and a public
// /auth/password/* flow for the email-link reset path.

// Errors specific to the account management flow.
var (
	// ErrUserAlreadyDisabled is returned when Disable is called on a
	// user that is already disabled (idempotent no-op would hide
	// race conditions, so we surface a typed error).
	ErrUserAlreadyDisabled = errors.New("user already disabled")

	// ErrUserNotDisabled is returned when Enable is called on a
	// user that is not currently disabled.
	ErrUserNotDisabled = errors.New("user not disabled")

	// ErrLastAdmin is returned by Disable / Delete when the action
	// would remove the last active administrator from the system.
	// The check is conservative: ANY active user with role='admin'
	// (or the wildcard permission) counts. Lockout is a real risk
	// for self-hosted installs and the plan prioritises safety.
	ErrLastAdmin = errors.New("cannot remove the last active admin")

	// ErrUserIdentityProviderLocked is returned when an admin
	// action would mutate a user whose auth_provider is NOT
	// 'local'. Those accounts are owned by the IdP — local writes
	// would be silently overwritten on the next login sync.
	ErrUserIdentityProviderLocked = errors.New("user is managed by an external identity provider")

	// ErrPasswordResetTokenInvalid / Expired / Consumed are
	// returned by the public password-reset endpoints. The HTTP
	// layer maps each to a distinct i18n code so the UI can show
	// the correct retry flow without leaking which one matched.
	ErrPasswordResetTokenInvalid  = errors.New("password reset token invalid")
	ErrPasswordResetTokenExpired  = errors.New("password reset token expired")
	ErrPasswordResetTokenConsumed = errors.New("password reset token already used")

	// ErrAccountDisabled / MustResetPassword are returned by
	// Service.Authenticate to drive the must_reset_password and
	// disabled_at branches of the login flow.
	ErrAccountDisabled   = errors.New("account disabled")
	ErrMustResetPassword = errors.New("password reset required")
)

// SessionView is one row in the sessions list returned to admins /
// the user themselves. We never expose the raw session id; the
// stable handle is the truncated id (first 8 chars) plus a
// fingerprint (last 4).
type SessionView struct {
	ID             string `json:"id"`
	Handle         string `json:"handle"`
	IPAddress      string `json:"ipAddress"`
	UserAgent      string `json:"userAgent"`
	CreatedAt      string `json:"createdAt"`
	ExpiresAt      string `json:"expiresAt"`
	RevokedAt      string `json:"revokedAt,omitempty"`
	RevokedBy      string `json:"revokedBy,omitempty"`
	CurrentSession bool   `json:"currentSession"`
}

// countActiveAdmins returns the number of users with role='admin'
// who are not disabled and not the synthetic 'system' /
// '__legacy_owner__' placeholders. Used by the last-admin guard.
func (s *Service) countActiveAdmins(ctx context.Context, excludeID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users
		 WHERE role = 'admin'
		   AND is_active = 1
		   AND (disabled_at IS NULL OR disabled_at = '')
		   AND id NOT IN ('system', '__legacy_owner__')
		   AND id != ?
	`, excludeID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting active admins: %w", err)
	}
	return n, nil
}

// isIdentityProviderManaged returns true when the user was
// provisioned by an external IdP (OIDC / SAML / LDAP). We refuse
// local admin actions on those accounts.
func (s *Service) isIdentityProviderManaged(ctx context.Context, id string) (bool, error) {
	var provider string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(auth_provider, 'local') FROM users WHERE id = ?`, id).Scan(&provider)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("checking auth provider for %s: %w", id, err)
	}
	return provider != "local", nil
}

// isExternallyManaged is isIdentityProviderManaged minus service principals:
// nobody else manages a service principal, so an admin must be able to disable
// it, while its password paths stay refused through the stricter check.
func (s *Service) isExternallyManaged(ctx context.Context, id string) (bool, error) {
	var provider string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(auth_provider, 'local') FROM users WHERE id = ?`, id).Scan(&provider)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("checking auth provider for %s: %w", id, err)
	}
	return provider != "local" && provider != KindService, nil
}

// Disable marks the user as disabled, stores the reason, and
// revokes all their existing sessions. Returns ErrUserAlreadyDisabled
// if already disabled, ErrLastAdmin if the action would leave zero
// active admins, and ErrUserIdentityProviderLocked for non-local users.
func (s *Service) Disable(ctx context.Context, id, by, reason string) error {
	if managed, err := s.isExternallyManaged(ctx, id); err != nil {
		return err
	} else if managed {
		return ErrUserIdentityProviderLocked
	}

	// Last-admin guard: only enforce when disabling an active admin.
	var isActiveAdmin bool
	err := s.db.QueryRowContext(ctx, `
		SELECT (is_active = 1) AND (role = 'admin') AND (disabled_at IS NULL OR disabled_at = '')
		  FROM users WHERE id = ?
	`, id).Scan(&isActiveAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("checking target user: %w", err)
	}
	if isActiveAdmin {
		n, err := s.countActiveAdmins(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrLastAdmin
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting disable transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	// Set disabled_at only if not already set.
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		   SET disabled_at = ?, disabled_by = ?, disabled_reason = ?,
		       is_active = 0, updated_at = ?
		 WHERE id = ? AND (disabled_at IS NULL OR disabled_at = '')
	`, now, by, reason, now, id)
	if err != nil {
		return fmt.Errorf("disabling user %s: %w", id, err)
	}
	if dbRowsAffected(res) == 0 {
		return ErrUserAlreadyDisabled
	}

	// Revoke all active sessions.
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?, revoked_by = ?
		 WHERE user_id = ? AND (revoked_at IS NULL OR revoked_at = '')
	`, now, by, id); err != nil {
		return fmt.Errorf("revoking sessions for %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing disable: %w", err)
	}

	if s.rbac != nil {
		s.rbac.InvalidateCache(id)
	}
	return nil
}

// Enable clears the disabled_at flag, restoring access. Returns
// ErrUserNotDisabled if the user was not disabled.
func (s *Service) Enable(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE users
		   SET disabled_at = NULL, disabled_by = NULL, disabled_reason = NULL,
		       is_active = 1, password_failed_count = 0, password_locked_until = NULL,
		       updated_at = ?
		 WHERE id = ? AND disabled_at IS NOT NULL AND disabled_at != ''
	`, now, id)
	if err != nil {
		return fmt.Errorf("enabling user %s: %w", id, err)
	}
	if dbRowsAffected(res) == 0 {
		return ErrUserNotDisabled
	}
	return nil
}

// ForceResetPassword stamps must_reset_password=1 so the next login
// forces a password change. Does NOT change the password itself —
// the user must complete the change on the next login.
func (s *Service) ForceResetPassword(ctx context.Context, id, by string) error {
	if managed, err := s.isIdentityProviderManaged(ctx, id); err != nil {
		return err
	} else if managed {
		return ErrUserIdentityProviderLocked
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE users
		   SET must_reset_password = 1, updated_at = ?
		 WHERE id = ?
	`, now, id)
	if err != nil {
		return fmt.Errorf("force-resetting user %s: %w", id, err)
	}
	if dbRowsAffected(res) == 0 {
		return ErrUserNotFound
	}
	_ = by // reserved for audit
	return nil
}

// Unlock clears the failed-attempt counter and any active lock for
// the target user. Admin-only; the handler enforces permission.
//
// Returns ErrUserNotFound if the user doesn't exist. No-op if the
// user isn't currently locked (the lockout table may still have
// stale rows from prior attempts; ClearFailedAttempts is idempotent).
func (s *Service) Unlock(ctx context.Context, id, by string) error {
	exists, err := s.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
	}
	if s.auth != nil {
		s.auth.ClearFailedAttemptsContext(ctx, id)
	}
	_ = by // reserved for audit
	return nil
}

// ListSessions returns the active and recently-revoked sessions for
// a user. The 'currentID' argument flags the caller's own session
// so the UI can render a "this device" badge.
func (s *Service) ListSessions(ctx context.Context, userID, currentID string) ([]SessionView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(ip_address, ''), COALESCE(user_agent, ''),
		       created_at, expires_at,
		       COALESCE(revoked_at, ''), COALESCE(revoked_by, '')
		  FROM sessions
		 WHERE user_id = ?
		 ORDER BY created_at DESC
		 LIMIT 100
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing sessions for %s: %w", userID, err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	out := []SessionView{}
	for rows.Next() {
		var sv SessionView
		var id string
		if err := rows.Scan(&id, &sv.IPAddress, &sv.UserAgent, &sv.CreatedAt,
			&sv.ExpiresAt, &sv.RevokedAt, &sv.RevokedBy); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sv.ID = id
		sv.Handle = id
		if len(id) > 12 {
			sv.Handle = id[:8] + "…" + id[len(id)-4:]
		}
		sv.CurrentSession = id == currentID
		out = append(out, sv)
	}
	return out, rows.Err()
}

// RevokeSession marks one session as revoked. Returns
// ErrRoleAssignmentNotFound (re-used) when no row matched.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID, by string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?, revoked_by = ?
		 WHERE id = ? AND user_id = ? AND (revoked_at IS NULL OR revoked_at = '')
	`, now, by, sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoking session %s: %w", sessionID, err)
	}
	if dbRowsAffected(res) == 0 {
		return ErrRoleAssignmentNotFound
	}
	return nil
}

// RevokeAllSessions revokes every active session for a user, except
// optionally one (the caller's own). Returns the number revoked.
func (s *Service) RevokeAllSessions(ctx context.Context, userID, keepSessionID, by string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var res sql.Result
	var err error
	if keepSessionID != "" {
		res, err = s.db.ExecContext(ctx, `
			UPDATE sessions SET revoked_at = ?, revoked_by = ?
			 WHERE user_id = ? AND id != ? AND (revoked_at IS NULL OR revoked_at = '')
		`, now, by, userID, keepSessionID)
	} else {
		res, err = s.db.ExecContext(ctx, `
			UPDATE sessions SET revoked_at = ?, revoked_by = ?
			 WHERE user_id = ? AND (revoked_at IS NULL OR revoked_at = '')
		`, now, by, userID)
	}
	if err != nil {
		return 0, fmt.Errorf("revoking all sessions for %s: %w", userID, err)
	}
	return dbRowsAffected(res), nil
}

// ─── Password reset flow ─────────────────────────────────────────

// PasswordResetEmailTTL is the lifetime of a self-requested password-reset email link.
const PasswordResetEmailTTL = 1 * time.Hour

// IssuePasswordResetToken creates a single-use password-reset token
// for a user. The plaintext secret is returned to the caller (it
// goes in the email body / shown once to the admin); only the
// sha256 hash is persisted. The 'createdBy' field distinguishes
// 'self' from an admin-initiated reset for audit.
//
// The caller is responsible for delivering the token to the user
// (email or admin UI). Returns the plaintext secret and its expiry.
func (s *Service) IssuePasswordResetToken(ctx context.Context, userID, createdBy string, ttl time.Duration) (plaintext string, expiresAt time.Time, err error) {
	// 32 bytes = 256 bits of entropy.
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, fmt.Errorf("generating reset token: %w", err)
	}
	plaintext = hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plaintext))
	hash := hex.EncodeToString(sum[:])

	id := xid.New().String()
	now := time.Now().UTC()
	expiresAt = now.Add(ttl)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens
		    (id, user_id, token_hash, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, userID, hash, createdBy, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("inserting reset token: %w", err)
	}
	return plaintext, expiresAt, nil
}

// ResolvePasswordResetToken validates a plaintext token WITHOUT
// consuming it and returns the token row id plus the user id.
// Errors:
//   - ErrPasswordResetTokenInvalid: token hash not found
//   - ErrPasswordResetTokenExpired: expires_at is in the past
//   - ErrPasswordResetTokenConsumed: consumed_at is already set
//
// Split out from ConsumePasswordResetToken so the reset handler can
// check the token, then run the new password through the policy, and
// only burn the single-use token once the change is certain to
// succeed. Consuming first meant a policy rejection cost the user
// their reset link.
func (s *Service) ResolvePasswordResetToken(ctx context.Context, plaintext string) (tokenID, userID string, err error) {
	sum := sha256.Sum256([]byte(plaintext))
	hash := hex.EncodeToString(sum[:])

	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, expires_at, COALESCE(consumed_at, '')
		  FROM password_reset_tokens
		 WHERE token_hash = ?
	`, hash)
	var id, uid, expiresAt, consumedAt string
	if scanErr := row.Scan(&id, &uid, &expiresAt, &consumedAt); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return "", "", ErrPasswordResetTokenInvalid
		}
		return "", "", fmt.Errorf("looking up reset token: %w", scanErr)
	}
	if consumedAt != "" {
		return "", "", ErrPasswordResetTokenConsumed
	}
	// Lexicographic compare on RFC3339 strings is correct.
	if expiresAt <= time.Now().UTC().Format(time.RFC3339) {
		return "", "", ErrPasswordResetTokenExpired
	}
	return id, uid, nil
}

// ConsumePasswordResetToken validates a plaintext token, marks it
// consumed, and returns the associated user id. Errors:
//   - ErrPasswordResetTokenInvalid: token hash not found
//   - ErrPasswordResetTokenExpired: expires_at is in the past
//   - ErrPasswordResetTokenConsumed: consumed_at is already set
//
// On success, also sets must_reset_password=0 on the user (since
// they have just demonstrated control of the reset flow). The
// caller is expected to then call ChangePassword.
func (s *Service) ConsumePasswordResetToken(ctx context.Context, plaintext, ip, userAgent string) (userID string, err error) {
	id, uid, err := s.ResolvePasswordResetToken(ctx, plaintext)
	if err != nil {
		return "", err
	}
	nowStr := time.Now().UTC().Format(time.RFC3339)

	// Mark consumed + clear must_reset_password + audit fields.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("starting consume transaction: %w", err)
	}
	defer closeutil.Rollback(tx)
	if _, err := tx.ExecContext(ctx, `
		UPDATE password_reset_tokens
		   SET consumed_at = ?, consumed_ip = ?, consumed_user_agent = ?
		 WHERE id = ? AND (consumed_at IS NULL OR consumed_at = '')
	`, nowStr, ip, userAgent, id); err != nil {
		return "", fmt.Errorf("consuming reset token: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		   SET must_reset_password = 0, updated_at = ?
		 WHERE id = ?
	`, nowStr, uid); err != nil {
		return "", fmt.Errorf("clearing must_reset_password: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing reset consume: %w", err)
	}
	return uid, nil
}

// AdminSetPassword changes a user's password and clears the
// must_reset_password flag. The caller is expected to have already
// verified the admin permission. Returns ErrUserIdentityProviderLocked
// for non-local accounts.
//
// Phase 6d: the new password is validated against the policy and
// rejected if it appears in the last N hashes (history-based
// reuse prevention). The policy is loaded from settings so admins
// can tighten it without restarting.
func (s *Service) AdminSetPassword(ctx context.Context, id, newPassword string) error {
	if managed, err := s.isIdentityProviderManaged(ctx, id); err != nil {
		return err
	} else if managed {
		return ErrUserIdentityProviderLocked
	}
	policy := auth.LoadPasswordPolicy(ctx, s.db)
	if err := auth.ValidatePassword(newPassword, policy); err != nil {
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	if reused, err := s.auth.InHistoryContext(ctx, id, newPassword, policy); err != nil {
		return fmt.Errorf("checking password history: %w", err)
	} else if reused {
		return &auth.PasswordValidationError{Field: "reused"}
	}
	if err := s.auth.SetPasswordContext(ctx, id, hash, policy); err != nil {
		return fmt.Errorf("admin setting password for %s: %w", id, err)
	}
	return nil
}

// ─── Login pre-checks ────────────────────────────────────────────

// CheckAccountStatus returns ErrAccountDisabled if the user is
// disabled, ErrMustResetPassword if a password change is required,
// or nil if the account is in good standing. Used by the login
// handler to short-circuit before the password is checked (so the
// password isn't even hashed for disabled accounts, which slightly
// reduces attack surface for offline timing).
func (s *Service) CheckAccountStatus(ctx context.Context, id string) error {
	var disabledAt sql.NullString
	var mustReset int
	err := s.db.QueryRowContext(ctx, `SELECT disabled_at, must_reset_password FROM users WHERE id = ?`, id).Scan(&disabledAt, &mustReset)
	if err != nil {
		return fmt.Errorf("checking account status: %w", err)
	}
	if disabledAt.Valid && disabledAt.String != "" {
		return ErrAccountDisabled
	}
	if mustReset == 1 {
		return ErrMustResetPassword
	}
	return nil
}

// RecordLoginAttempt inserts a row into login_attempts for forensic
// visibility. The login flow calls this on every attempt regardless
// of success or failure reason.
func (s *Service) RecordLoginAttempt(ctx context.Context, username, ip, userAgent, reason string, succeeded bool, userID string) {
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
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO login_attempts (id, username, ip_address, succeeded, user_id, user_agent, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, uname, ip, boolToInt(succeeded), uid, userAgent, reason, now); err != nil {
		slog.Warn("users: recording login attempt failed", "error", err)
	}
}

// MarkLoginSuccess updates last_login_ip / last_login_user_agent on
// the user and clears failed-count / locked-until.
func (s *Service) MarkLoginSuccess(ctx context.Context, id, ip, userAgent string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE users
		   SET last_login = ?, last_login_ip = ?, last_login_user_agent = ?,
		       password_failed_count = 0, password_locked_until = NULL,
		       updated_at = ?
		 WHERE id = ?
	`, now, ip, userAgent, now, id); err != nil {
		slog.Warn("users: marking login success failed", "user", id, "error", err)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// dbRowsAffected abstracts the rows-affected count regardless of
// whether the driver exposes RowsAffected() directly. Already used
// in this package (see db.RowsAffected); we re-export here to keep
// the new file self-contained.
func dbRowsAffected(res sql.Result) int {
	return int(db.RowsAffected(res))
}
