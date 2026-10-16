// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// PasswordPolicy is the runtime-configurable password policy. All
// values come from settings (seeded in migration 072) so admins can
// tighten or relax the policy without restarting the server.
type PasswordPolicy struct {
	// MinLength is the bare minimum length. Anything shorter is
	// rejected regardless of complexity.
	MinLength int

	// RequireUpper, RequireLower, RequireDigit, RequireSymbol gate
	// individual character-class requirements.
	RequireUpper  bool
	RequireLower  bool
	RequireDigit  bool
	RequireSymbol bool

	// MaxAgeDays, when > 0, forces users to change their password
	// after this many days. 0 disables.
	MaxAgeDays int

	// HistorySize, when > 0, prevents reusing any of the last N
	// passwords. The history is stored in password_history.
	HistorySize int
}

// DefaultPasswordPolicy matches the seeded settings.
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength:     12,
		RequireUpper:  true,
		RequireLower:  true,
		RequireDigit:  true,
		RequireSymbol: true,
		MaxAgeDays:    0,
		HistorySize:   5,
	}
}

// LoadPasswordPolicy reads the policy from settings. Falls back to
// defaults for missing or unparseable rows.
func LoadPasswordPolicy(ctx context.Context, db *sql.DB) PasswordPolicy {
	p := DefaultPasswordPolicy()
	if db == nil {
		return p
	}
	if v, ok := readSettingInt(ctx, db, "password_min_length"); ok && v >= 0 {
		p.MinLength = v
	}
	if v, ok := readSettingBool(ctx, db, "password_require_upper"); ok {
		p.RequireUpper = v
	}
	if v, ok := readSettingBool(ctx, db, "password_require_lower"); ok {
		p.RequireLower = v
	}
	if v, ok := readSettingBool(ctx, db, "password_require_digit"); ok {
		p.RequireDigit = v
	}
	if v, ok := readSettingBool(ctx, db, "password_require_symbol"); ok {
		p.RequireSymbol = v
	}
	if v, ok := readSettingInt(ctx, db, "password_max_age_days"); ok && v >= 0 {
		p.MaxAgeDays = v
	}
	if v, ok := readSettingInt(ctx, db, "password_history_size"); ok && v >= 0 {
		p.HistorySize = v
	}
	return p
}

// PasswordValidationError is returned by ValidatePassword when the
// policy rejects a candidate. Field names the rule that failed —
// "too_short", "no_upper", "no_lower", "no_digit", "no_symbol", or
// "reused" (set by the history check, not by ValidatePassword).
type PasswordValidationError struct {
	Field string
}

func (e *PasswordValidationError) Error() string {
	return fmt.Sprintf("password rejected: %s", e.Field)
}

// Code maps the failed rule onto the i18n code a handler should
// return. Every write path shares this so the client sees the same
// message for the same rule no matter which endpoint rejected it.
// Unknown fields fall back to the generic "too weak".
func (e *PasswordValidationError) Code() i18n.MsgCode {
	if e == nil {
		return i18n.ErrPasswordTooWeak
	}
	switch e.Field {
	case "too_short":
		return i18n.ErrPasswordTooShort
	case "no_upper":
		return i18n.ErrPasswordNoUpper
	case "no_lower":
		return i18n.ErrPasswordNoLower
	case "no_digit":
		return i18n.ErrPasswordNoDigit
	case "no_symbol":
		return i18n.ErrPasswordNoSymbol
	case "reused":
		return i18n.ErrPasswordReused
	default:
		return i18n.ErrPasswordTooWeak
	}
}

// ErrPasswordRejected wraps ValidatePassword errors so call sites
// can compare with errors.Is.
var ErrPasswordRejected = errors.New("password rejected")

// ValidatePassword returns nil when the candidate meets the policy.
// On error it returns *PasswordValidationError so the caller can
// produce a typed i18n code.
func ValidatePassword(p string, policy PasswordPolicy) error {
	if len(p) < policy.MinLength {
		return &PasswordValidationError{Field: "too_short"}
	}
	var hasUpper, hasLower, hasDigit, hasSymbol bool
	for _, r := range p {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSymbol = true
		}
	}
	if policy.RequireUpper && !hasUpper {
		return &PasswordValidationError{Field: "no_upper"}
	}
	if policy.RequireLower && !hasLower {
		return &PasswordValidationError{Field: "no_lower"}
	}
	if policy.RequireDigit && !hasDigit {
		return &PasswordValidationError{Field: "no_digit"}
	}
	if policy.RequireSymbol && !hasSymbol {
		return &PasswordValidationError{Field: "no_symbol"}
	}
	return nil
}

// IsExpired reports whether the user's password is older than the
// policy's MaxAgeDays. A password_changed_at of NULL is treated as
// "never changed" and is not expired (handled at registration).
func IsExpired(passwordChangedAt sql.NullString, policy PasswordPolicy) bool {
	if policy.MaxAgeDays <= 0 || !passwordChangedAt.Valid || passwordChangedAt.String == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, passwordChangedAt.String)
	if err != nil {
		return false
	}
	age := time.Since(t)
	return age > time.Duration(policy.MaxAgeDays)*24*time.Hour
}

// InHistory checks whether the candidate's hash matches any of the
// last N entries in password_history. The plaintext is not stored;
// only the Argon2id hash is.
//
// The caller passes the PLAINTEXT candidate, not a hash. Every hash
// carries its own random salt (see HashPassword), so two hashes of
// the same password never compare equal — the comparison has to run
// through VerifyPassword against each stored entry.
func (s *Service) InHistory(userID, candidatePlain string, policy PasswordPolicy) (bool, error) {
	return s.InHistoryContext(context.Background(), userID, candidatePlain, policy)
}

// InHistoryContext is InHistory bound to the caller's context, so a cancelled request stops the history lookup.
func (s *Service) InHistoryContext(parent context.Context, userID, candidatePlain string, policy PasswordPolicy) (bool, error) {
	if s == nil || s.db == nil || policy.HistorySize <= 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		SELECT password_hash
		  FROM password_history
		 WHERE user_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?
	`, userID, policy.HistorySize)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing password history rows failed", "error", err)
		}
	}()
	for rows.Next() {
		var stored string
		if err := rows.Scan(&stored); err != nil {
			return false, err
		}
		match, err := VerifyPassword(candidatePlain, stored)
		if err != nil {
			// A malformed historical hash must not block the
			// change; skip it and keep checking the rest.
			continue
		}
		if match {
			return true, nil
		}
	}
	return false, rows.Err()
}

// RecordPasswordHistory inserts the previous hash into the history
// table and trims the table to the policy size. Called BEFORE updating
// the users.password_hash row.
func (s *Service) RecordPasswordHistory(userID, oldHash string, policy PasswordPolicy) error {
	return s.RecordPasswordHistoryContext(context.Background(), userID, oldHash, policy)
}

// RecordPasswordHistoryContext is RecordPasswordHistory bound to the caller's context, so a cancelled request stops the history insert.
func (s *Service) RecordPasswordHistoryContext(parent context.Context, userID, oldHash string, policy PasswordPolicy) error {
	if s == nil || s.db == nil || oldHash == "" || policy.HistorySize <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()
	id := xid.New().String()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO password_history (id, user_id, password_hash, created_at)
		VALUES (?, ?, ?, ?)
	`, id, userID, oldHash, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	// Trim old entries beyond HistorySize.
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM password_history
		 WHERE user_id = ? AND id NOT IN (
		    SELECT id FROM password_history
		     WHERE user_id = ?
		     ORDER BY created_at DESC
		     LIMIT ?
		 )
	`, userID, userID, policy.HistorySize); err != nil {
		return fmt.Errorf("trimming password history: %w", err)
	}
	return nil
}

// SetPassword updates the password and clears the must-reset flag.
// The handler must have validated the candidate already.
func (s *Service) SetPassword(userID, newHash string, policy PasswordPolicy) error {
	return s.SetPasswordContext(context.Background(), userID, newHash, policy)
}

// SetPasswordContext is SetPassword bound to the caller's context, so a cancelled request stops the password update.
func (s *Service) SetPasswordContext(parent context.Context, userID, newHash string, policy PasswordPolicy) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("service not initialized")
	}
	if s.IsServicePrincipalContext(parent, userID) {
		return ErrServicePrincipal
	}
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()

	// Pull the current hash so we can record it in history.
	var oldHash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&oldHash)
	if err != nil {
		return err
	}

	if err := s.RecordPasswordHistoryContext(parent, userID, oldHash, policy); err != nil {
		return fmt.Errorf("recording password history: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		UPDATE users
		   SET password_hash = ?, password_changed_at = ?, must_reset_password = 0,
		       password_failed_count = 0, password_locked_until = NULL,
		       updated_at = ?
		 WHERE id = ?
	`, newHash, now, now, userID)
	if err != nil {
		return err
	}
	s.ClearFailedAttemptsContext(parent, userID)
	return nil
}

// readSettingBool reads a settings row that contains "true"/"false".
// Returns (value, true) only when the row parses cleanly.
func readSettingBool(ctx context.Context, db *sql.DB, key string) (bool, bool) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if err != nil {
		return false, false
	}
	v, err := strconv.ParseBool(strings.TrimSpace(strings.ToLower(raw)))
	if err != nil {
		return false, false
	}
	return v, true
}
