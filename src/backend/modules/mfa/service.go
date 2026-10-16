// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package mfa

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Sentinel errors. The handler maps these onto i18n codes; the raw
// error text never reaches a client.
var (
	// ErrAlreadyEnabled is returned when enrollment is started for a
	// user who already completed it. Re-enrolling requires an
	// explicit disable first, so a hijacked session can't silently
	// swap the authenticator device.
	ErrAlreadyEnabled = errors.New("mfa: already enabled")
	// ErrNotEnrolled is returned when confirm/disable is called
	// without a pending or active enrollment.
	ErrNotEnrolled = errors.New("mfa: not enrolled")
	// ErrInvalidCode covers a wrong TOTP code, a wrong recovery
	// code, and a replayed code inside its own validity window.
	ErrInvalidCode = errors.New("mfa: invalid code")
	// ErrChallengeInvalid is returned for an unknown, expired, or
	// already-consumed login challenge.
	ErrChallengeInvalid = errors.New("mfa: invalid challenge")
)

// challengeTTL is how long the second login step stays open. Long
// enough to fetch a phone, short enough that a stolen challenge id is
// near-worthless.
const challengeTTL = 5 * time.Minute

// Service owns all MFA persistence and verification. Secrets and
// recovery codes are AES-256-GCM encrypted at rest via the shared
// encryption service.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
	// now is indirected so tests can pin the TOTP clock.
	now func() time.Time
}

// NewService creates an MFA service.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: database, enc: enc, now: func() time.Time { return time.Now().UTC() }}
}

// Status returns the MFA state for a user. A user with no row is
// simply not enrolled — that is not an error.
func (s *Service) Status(ctx context.Context, userID string) (*Status, error) {
	var enabled bool
	var enrolledAt, lastUsedAt sql.NullString
	var remaining int

	err := s.db.QueryRowContext(ctx,
		`SELECT enabled, enrolled_at, last_used_at, scratch_count
		 FROM user_mfa WHERE user_id = ?`,
		userID,
	).Scan(&enabled, &enrolledAt, &lastUsedAt, &remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return &Status{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying mfa status: %w", err)
	}

	st := &Status{
		Enabled:           enabled,
		RecoveryRemaining: remaining,
		HasRecovery:       remaining > 0,
	}
	if enrolledAt.Valid {
		st.EnrolledAt = enrolledAt.String
	}
	if lastUsedAt.Valid {
		st.LastUsedAt = lastUsedAt.String
	}
	return st, nil
}

// IsEnabled reports whether the user has completed enrollment. Used by
// the login gate, where a missing row must mean "no MFA" rather than
// an error that would lock everyone out.
func (s *Service) IsEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled FROM user_mfa WHERE user_id = ?`, userID,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying mfa enabled: %w", err)
	}
	return enabled, nil
}

// BeginEnrollment generates a fresh secret and recovery codes, stores
// them encrypted in a *pending* (enabled = 0) row, and returns the
// plaintext exactly once. Nothing is enforced at login until
// ConfirmEnrollment proves the user can produce a valid code.
func (s *Service) BeginEnrollment(ctx context.Context, userID, username, issuer string) (*Enrollment, error) {
	enabled, err := s.IsEnabled(ctx, userID)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, ErrAlreadyEnabled
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, err
	}
	codes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}

	secretEnc, err := s.enc.Encrypt(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting mfa secret: %w", err)
	}
	codesJSON, err := json.Marshal(codes)
	if err != nil {
		return nil, fmt.Errorf("marshalling recovery codes: %w", err)
	}
	codesEnc, err := s.enc.Encrypt(string(codesJSON))
	if err != nil {
		return nil, fmt.Errorf("encrypting recovery codes: %w", err)
	}

	now := s.now().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO user_mfa
		   (user_id, enabled, secret_encrypted, recovery_codes_encrypted,
		    enrolled_at, last_used_at, scratch_count, created_at, updated_at)
		 VALUES (?, 0, ?, ?, NULL, NULL, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   enabled = 0,
		   secret_encrypted = excluded.secret_encrypted,
		   recovery_codes_encrypted = excluded.recovery_codes_encrypted,
		   enrolled_at = NULL,
		   last_used_at = NULL,
		   scratch_count = excluded.scratch_count,
		   updated_at = excluded.updated_at`,
		userID, secretEnc, codesEnc, len(codes), now, now,
	); err != nil {
		return nil, fmt.Errorf("storing mfa enrollment: %w", err)
	}

	return &Enrollment{
		Secret:        secret,
		OtpauthURI:    otpauthURI(issuer, username, secret),
		RecoveryCodes: codes,
	}, nil
}

// ConfirmEnrollment verifies the first code from the authenticator and
// flips the pending row to enabled.
func (s *Service) ConfirmEnrollment(ctx context.Context, userID, code string) error {
	pending, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if pending.Enabled {
		return ErrAlreadyEnabled
	}

	counter, ok, err := verifyTOTP(pending.Secret, code, s.now())
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCode
	}

	now := s.now().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_mfa
		 SET enabled = 1, enrolled_at = ?, last_used_at = ?, updated_at = ?
		 WHERE user_id = ?`,
		now, windowStart(counter), now, userID,
	)
	if err != nil {
		return fmt.Errorf("enabling mfa: %w", err)
	}
	if coredb.RowsAffected(res) == 0 {
		return ErrNotEnrolled
	}
	return nil
}

// Disable removes the enrollment entirely — secret, recovery codes,
// and any outstanding login challenges.
func (s *Service) Disable(ctx context.Context, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_mfa WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting mfa enrollment: %w", err)
	}
	if coredb.RowsAffected(res) == 0 {
		return ErrNotEnrolled
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM mfa_login_challenges WHERE user_id = ?`, userID,
	); err != nil {
		return fmt.Errorf("clearing mfa challenges: %w", err)
	}
	return nil
}

// VerifyForLogin accepts either a TOTP code or one single-use recovery
// code and reports success. A TOTP code is rejected if it was already
// used inside its own 30-second window, so an observed code cannot be
// replayed for the remainder of the skew window.
func (s *Service) VerifyForLogin(ctx context.Context, userID, code string) error {
	record, err := s.load(ctx, userID)
	if err != nil {
		return err
	}
	if !record.Enabled {
		return ErrNotEnrolled
	}

	counter, ok, err := verifyTOTP(record.Secret, code, s.now())
	if err != nil {
		return err
	}
	if ok {
		if isReplay(record.LastUsedAt, counter) {
			return ErrInvalidCode
		}
		now := s.now().Format(time.RFC3339)
		if _, err := s.db.ExecContext(ctx,
			`UPDATE user_mfa SET last_used_at = ?, updated_at = ? WHERE user_id = ?`,
			windowStart(counter), now, userID,
		); err != nil {
			return fmt.Errorf("recording mfa use: %w", err)
		}
		return nil
	}

	return s.consumeRecoveryCode(ctx, userID, record, code)
}

// consumeRecoveryCode burns a single-use recovery code, if the supplied
// value matches one.
func (s *Service) consumeRecoveryCode(ctx context.Context, userID string, record *pendingSecret, code string) error {
	candidate := normalizeRecoveryCode(code)
	if candidate == "" {
		return ErrInvalidCode
	}

	remaining := make([]string, 0, len(record.RecoveryCodes))
	matched := false
	for _, stored := range record.RecoveryCodes {
		// Constant-time compare on every entry — and no early
		// break — so the response time doesn't leak which code
		// matched or how many codes are left.
		if subtle.ConstantTimeCompare([]byte(normalizeRecoveryCode(stored)), []byte(candidate)) == 1 {
			matched = true
			continue
		}
		remaining = append(remaining, stored)
	}
	if !matched {
		return ErrInvalidCode
	}

	codesJSON, err := json.Marshal(remaining)
	if err != nil {
		return fmt.Errorf("marshalling recovery codes: %w", err)
	}
	codesEnc, err := s.enc.Encrypt(string(codesJSON))
	if err != nil {
		return fmt.Errorf("encrypting recovery codes: %w", err)
	}

	now := s.now().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE user_mfa
		 SET recovery_codes_encrypted = ?, scratch_count = ?, last_used_at = ?, updated_at = ?
		 WHERE user_id = ?`,
		codesEnc, len(remaining), now, now, userID,
	); err != nil {
		return fmt.Errorf("consuming recovery code: %w", err)
	}
	return nil
}

// CreateChallenge opens a short-lived second-step slot for a user who
// has already cleared the password check.
func (s *Service) CreateChallenge(ctx context.Context, userID string) (string, error) {
	now := s.now()
	id := xid.New().String()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_login_challenges (id, user_id, created_at, expires_at, consumed)
		 VALUES (?, ?, ?, ?, 0)`,
		id, userID, now.Format(time.RFC3339), now.Add(challengeTTL).Format(time.RFC3339),
	); err != nil {
		return "", fmt.Errorf("creating mfa challenge: %w", err)
	}

	// Opportunistic prune so the table can't grow without bound on a
	// busy instance. A failure here is not worth failing the login.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM mfa_login_challenges WHERE expires_at < ?`,
		now.Add(-challengeTTL).Format(time.RFC3339),
	); err != nil {
		return "", fmt.Errorf("pruning mfa challenges: %w", err)
	}

	return id, nil
}

// PeekChallenge resolves a live challenge to its user without burning
// it, so a mistyped code costs the user a retry rather than a whole
// re-authentication. Brute force is bounded by the 10-requests/minute
// limiter on the public auth router and the five-minute TTL.
func (s *Service) PeekChallenge(ctx context.Context, challengeID string) (string, error) {
	if challengeID == "" {
		return "", ErrChallengeInvalid
	}

	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM mfa_login_challenges
		 WHERE id = ? AND consumed = 0 AND expires_at > ?`,
		challengeID, s.now().Format(time.RFC3339),
	).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrChallengeInvalid
	}
	if err != nil {
		return "", fmt.Errorf("reading mfa challenge: %w", err)
	}
	return userID, nil
}

// ConsumeChallenge atomically burns a challenge and returns the user it
// belongs to. The single UPDATE guarded on `consumed = 0` is what makes
// the challenge one-shot even under concurrent requests.
func (s *Service) ConsumeChallenge(ctx context.Context, challengeID string) (string, error) {
	if challengeID == "" {
		return "", ErrChallengeInvalid
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE mfa_login_challenges
		 SET consumed = 1
		 WHERE id = ? AND consumed = 0 AND expires_at > ?`,
		challengeID, s.now().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("consuming mfa challenge: %w", err)
	}
	if coredb.RowsAffected(res) == 0 {
		return "", ErrChallengeInvalid
	}

	var userID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM mfa_login_challenges WHERE id = ?`, challengeID,
	).Scan(&userID); err != nil {
		return "", fmt.Errorf("reading mfa challenge: %w", err)
	}
	return userID, nil
}

// load reads and decrypts a user's MFA row.
func (s *Service) load(ctx context.Context, userID string) (*pendingSecret, error) {
	var enabled bool
	var secretEnc, codesEnc string
	var lastUsedAt sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT enabled, secret_encrypted, recovery_codes_encrypted, last_used_at
		 FROM user_mfa WHERE user_id = ?`,
		userID,
	).Scan(&enabled, &secretEnc, &codesEnc, &lastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotEnrolled
	}
	if err != nil {
		return nil, fmt.Errorf("querying mfa row: %w", err)
	}

	secret, err := s.enc.Decrypt(secretEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting mfa secret: %w", err)
	}

	var codes []string
	if codesEnc != "" {
		plain, err := s.enc.Decrypt(codesEnc)
		if err != nil {
			return nil, fmt.Errorf("decrypting recovery codes: %w", err)
		}
		if err := json.Unmarshal([]byte(plain), &codes); err != nil {
			return nil, fmt.Errorf("parsing recovery codes: %w", err)
		}
	}

	record := &pendingSecret{Secret: secret, Enabled: enabled, RecoveryCodes: codes}
	if lastUsedAt.Valid {
		record.LastUsedAt = lastUsedAt.String
	}
	return record, nil
}

// windowStart renders the start of a TOTP step as an RFC 3339 stamp, so
// last_used_at identifies the exact window a code came from.
func windowStart(counter int64) string {
	return time.Unix(counter*int64(totpPeriod.Seconds()), 0).UTC().Format(time.RFC3339)
}

// isReplay reports whether a code from `counter` was already spent. The
// stored stamp is the window start of the last accepted code, so any
// counter at or below it has been used.
func isReplay(lastUsedAt string, counter int64) bool {
	if lastUsedAt == "" {
		return false
	}
	last, err := time.Parse(time.RFC3339, lastUsedAt)
	if err != nil {
		// An unparseable stamp predates this format. Treat it as
		// "no recorded use" rather than locking the user out.
		return false
	}
	return counterAt(last) >= counter
}
