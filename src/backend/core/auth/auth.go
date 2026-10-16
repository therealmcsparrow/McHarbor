// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package auth owns local accounts, sessions and login lockout, and is linked into every plugin.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"
	"golang.org/x/crypto/argon2"

	"github.com/therealmcsparrow/mcharbor/core/db"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

const (
	// SessionCookie is the cookie name browsers carry the session ID in.
	SessionCookie = "mcharbor_session"
	// SessionDuration is how long a session stays valid after login.
	SessionDuration = 24 * time.Hour

	// Argon2id params matching Bun.password.hash defaults
	argonMemory  = 19456 // KiB
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// User represents an authenticated user.
type User struct {
	ID                string  `json:"id"`
	Username          string  `json:"username"`
	DisplayName       *string `json:"displayName"`
	Email             *string `json:"email"`
	PreferredLanguage string  `json:"preferredLanguage"`
	// IdentityProviderID is the row id of the identity_providers
	// record the user signed in with, or empty for local users.
	// Frontend uses this to render displayName/email as read-only
	// (those values are managed by the IdP and not editable from
	// McHarbor).
	IdentityProviderID string `json:"identityProviderId,omitempty"`
	// TimeFormat is "24h" or "12h". Defaults to "24h" when empty.
	TimeFormat string `json:"timeFormat"`
	// DateFormat is "ddmmyyyy" or "mmddyyyy". Defaults to
	// "ddmmyyyy" when empty.
	DateFormat string `json:"dateFormat"`

	// Toast* are this account's overrides of the app-wide toast defaults.
	// nil means "inherit", which is deliberately distinct from a zero
	// value: 0 is a legal duration ("never auto-dismiss") and false is a
	// legal richColors, so neither can stand in for "not set". They are
	// ignored entirely while the app-wide toastAllowUserOverride flag is
	// off - revoking the ability does not discard what a user chose.
	ToastPosition    *string `json:"toastPosition,omitempty"`
	ToastDurationMs  *int    `json:"toastDurationMs,omitempty"`
	ToastMaxVisible  *int    `json:"toastMaxVisible,omitempty"`
	ToastRichColors  *bool   `json:"toastRichColors,omitempty"`
	ToastCloseButton *bool   `json:"toastCloseButton,omitempty"`

	// Appearance* are this account's overrides of the app-wide appearance
	// defaults — theme, accent, interface font and root font size. nil means
	// "inherit", and like the toast columns they are ignored entirely while
	// the app-wide appearanceAllowUserOverride flag is off, so revoking the
	// ability does not discard what a user chose.
	AppearanceThemeID    *string `json:"appearanceThemeId,omitempty"`
	AppearanceAccent     *string `json:"appearanceAccent,omitempty"`
	AppearanceFont       *string `json:"appearanceFont,omitempty"`
	AppearanceBaseFontPx *int    `json:"appearanceBaseFontPx,omitempty"`
}

// AuthResult is returned from login/register; the auth prefix stays because plugins already import the name.
//
//nolint:revive // renaming an exported plugin-linked type would break every plugin build
type AuthResult struct {
	Success   bool   `json:"success"`
	SessionID string `json:"sessionId,omitempty"`
	User      *User  `json:"user,omitempty"`
	Error     string `json:"error,omitempty"`

	// Lockout / lifecycle surface (Phase 2.1). When the login
	// attempt fails because the account is currently locked out
	// after too many failed attempts, `Locked` is true and
	// `RetryAfterSeconds` carries the remaining seconds (rounded up)
	// until the lockout window expires. When the user is disabled
	// `Disabled` is true. When the user must rotate their password
	// before continuing, `MustResetPassword` is true.
	//
	// Success results may also carry `MustResetPassword` if the
	// account is in a forced-reset state — the frontend treats this
	// the same way as a lockout (force the user onto the password
	// change screen before allowing the rest of the app).
	Locked            bool  `json:"locked,omitempty"`
	Disabled          bool  `json:"disabled,omitempty"`
	MustResetPassword bool  `json:"mustResetPassword,omitempty"`
	RetryAfterSeconds int64 `json:"retryAfterSeconds,omitempty"`
}

// Service handles authentication operations.
type Service struct {
	db          *sql.DB
	authEnabled bool

	// policyCache memoises the admin-tunable session policy so the
	// idle-timeout check does not cost a SELECT on every request.
	policyCache sessionPolicyCache
}

// NewService creates a new auth service. Authentication is enabled
// by default — the synthetic "system" user short-circuit in the
// auth middleware is gated on IsAuthEnabled() returning false, and
// main.go explicitly calls SetAuthEnabled(false) when AUTH_DISABLE
// is set. Without this default, Go would zero-initialise
// authEnabled to false and the middleware would treat every request
// as the system user, making logout impossible.
func NewService(db *sql.DB) *Service {
	return &Service{db: db, authEnabled: true}
}

// DefaultLanguage returns the application-wide default language
// stored in the settings table by an admin in Settings → General.
// The default is "en" when the row is missing or the stored
// value is not a supported language. The auth handler reads this
// from the public /auth/status endpoint so the i18n init on the
// frontend can pick it up before login.
func (s *Service) DefaultLanguage(ctx context.Context) string {
	if s.db == nil {
		return "en"
	}
	var value string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM settings WHERE key = 'default_language' LIMIT 1",
	).Scan(&value)
	if err != nil || value == "" {
		return "en"
	}
	return value
}

// NormalizeUsername is the single canonical transform applied to a
// username before it reaches any lookup, uniqueness check, or INSERT
// against the users table. Usernames are treated as case-insensitive
// identifiers — "Linda" and "linda" are the same account — so every
// caller must run the value through this helper before it touches
// SQL. Uses strings.ToLower (Unicode-aware) rather than an ASCII-only
// fold so accented and non-Latin usernames normalise correctly too.
//
// Deliberately does NOT trim whitespace or apply any other
// normalisation — the only reported bug is case sensitivity, and
// scope stays narrow to that.
//
// Passwords are never passed through this function; password
// comparison remains fully case-sensitive via VerifyPassword.
func NormalizeUsername(s string) string {
	return strings.ToLower(s)
}

// HashPassword hashes a password with Argon2id.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Format: $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		hex.EncodeToString(salt),
		hex.EncodeToString(hash),
	), nil
}

// VerifyPassword checks a password against an Argon2id hash.
func VerifyPassword(password, encoded string) (bool, error) {
	var version int
	var memory, time uint32
	var threads uint8
	var saltHex, hashHex string

	_, err := fmt.Sscanf(encoded, "$argon2id$v=%d$m=%d,t=%d,p=%d$%s",
		&version, &memory, &time, &threads, &saltHex)
	if err != nil {
		return false, fmt.Errorf("parsing hash: %w", err)
	}

	// Split the combined salt$hash
	parts := splitLast(saltHex, "$")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid hash format")
	}
	saltHex = parts[0]
	hashHex = parts[1]

	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false, fmt.Errorf("decoding salt: %w", err)
	}

	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false, fmt.Errorf("decoding hash: %w", err)
	}

	computed := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	// Constant-time compare
	if len(computed) != len(expectedHash) {
		return false, nil
	}
	var diff byte
	for i := range computed {
		diff |= computed[i] ^ expectedHash[i]
	}
	return diff == 0, nil
}

// Login authenticates a user with username/password.
func (s *Service) Login(username, password string) (*AuthResult, error) {
	return s.LoginWithContext(username, password, "", "")
}

// LoginWithContext is the same as Login but captures the request IP
// and user-agent for forensic visibility (login_attempts table).
// The users service handler threads these through; the older Login
// signature remains for tests that don't care about the audit trail.
func (s *Service) LoginWithContext(username, password, ip, userAgent string) (*AuthResult, error) {
	return s.LoginFrom(username, password, LoginClient{IP: ip, UserAgent: userAgent})
}

// ipLockedResult is deliberately identical to the account-lock result, so a
// caller cannot tell which threshold tripped and learn whether a username exists.
func ipLockedResult(retryAfter time.Duration) *AuthResult {
	return &AuthResult{
		Success:           false,
		Error:             "Account temporarily locked due to too many failed attempts",
		Locked:            true,
		RetryAfterSeconds: int64(retryAfter.Seconds()),
	}
}

// LoginFrom authenticates username/password from an attributed client. It
// enforces the per-address lock before any user lookup, so a sprayed unknown
// username is throttled too and a locked address never costs an Argon2 check.
func (s *Service) LoginFrom(username, password string, client LoginClient) (*AuthResult, error) {
	return s.LoginFromContext(context.Background(), username, password, client)
}

// LoginFromContext is LoginFrom bound to the caller's context, so a cancelled request stops the login lookups and lockout bookkeeping.
func (s *Service) LoginFromContext(ctx context.Context, username, password string, client LoginClient) (*AuthResult, error) {
	// Usernames are case-insensitive; normalise once, up front, so
	// every downstream use (the lookup below, the lockout gate, the
	// failed-attempt counter, and the forensic login_attempts row)
	// keys on the exact same canonical value. Passwords are untouched
	// — they stay fully case-sensitive via VerifyPassword.
	username = NormalizeUsername(username)

	policy := LoadLockoutPolicy(ctx, s.db)
	if locked, retryAfter := s.CheckIPLockoutContext(ctx, client.IP, policy); locked {
		s.recordLoginAttempt(ctx, username, client, loginReasonIPLocked, false, "")
		return ipLockedResult(retryAfter), nil
	}
	// failed records one failure, then reports the address lock it armed, if any.
	failed := func(reason, userID string) time.Duration {
		s.recordLoginAttempt(ctx, username, client, reason, false, userID)
		return s.RecordIPFailureContext(ctx, client.IP, policy)
	}

	var id, passwordHash, preferredLanguage, identityProviderID string
	var timeFormat, dateFormat string
	var toast toastOverrideScan
	var look appearanceOverrideScan
	var displayName, email sql.NullString
	var isActive, mustReset bool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, password_hash, display_name, email, is_active,
		        COALESCE(must_reset_password, 0),
		        COALESCE(preferred_language, 'en'),
		        COALESCE(identity_provider_id, ''),
		        COALESCE(time_format, '24h'),
		        COALESCE(date_format, 'ddmmyyyy'),
		        toast_position, toast_duration_ms, toast_max_visible,
		        toast_rich_colors, toast_close_button,
		        appearance_theme_id, appearance_accent, appearance_font,
		        appearance_base_font_px
		 FROM users WHERE username = ?`,
		username,
	).Scan(append([]any{&id, &passwordHash, &displayName, &email, &isActive, &mustReset, &preferredLanguage, &identityProviderID, &timeFormat, &dateFormat}, append(toast.dest(), look.dest()...)...)...)

	if errors.Is(err, sql.ErrNoRows) {
		if ipLock := failed("invalid_credentials", ""); ipLock > 0 {
			return ipLockedResult(ipLock), nil
		}
		return &AuthResult{Success: false, Error: "Invalid username or password"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}

	if !isActive {
		if ipLock := failed("disabled", id); ipLock > 0 {
			return ipLockedResult(ipLock), nil
		}
		return &AuthResult{Success: false, Error: "Account is disabled", Disabled: true}, nil
	}

	// A service principal acts unattended and never signs in. It answers exactly
	// like a wrong password, before any Argon2 work, so the kind does not leak.
	if s.IsServicePrincipalContext(ctx, id) {
		if ipLock := failed("service_principal", id); ipLock > 0 {
			return ipLockedResult(ipLock), nil
		}
		return &AuthResult{Success: false, Error: "Invalid username or password"}, nil
	}

	// Lockout gate. Runs BEFORE VerifyPassword so a locked account
	// never spends Argon2 CPU on the brute-force attempt — and so the
	// frontend can show a live countdown. We only consult the lock
	// table once we know the user exists, but the ErrNoRows branch
	// above never reaches here so unknown usernames don't leak lock
	// state.
	locked, _, retryAfter := s.CheckLockoutContext(ctx, username, policy)
	if locked {
		if ipLock := failed("locked", id); ipLock > retryAfter {
			retryAfter = ipLock
		}
		return &AuthResult{
			Success:           false,
			Error:             "Account temporarily locked due to too many failed attempts",
			Locked:            true,
			RetryAfterSeconds: int64(retryAfter.Seconds()),
		}, nil
	}

	valid, err := VerifyPassword(password, passwordHash)
	if err != nil || !valid {
		// Record the failure; if it trips the threshold this call also
		// arms the lock. Either way we tell the caller only the
		// generic "invalid" message so we don't leak whether the
		// user exists — but the locked countdown on subsequent
		// attempts still works.
		nextLock := s.RecordFailedAttemptContext(ctx, username, policy)
		if nextLock > 0 {
			if ipLock := failed("locked", id); ipLock > nextLock {
				nextLock = ipLock
			}
			return &AuthResult{
				Success:           false,
				Error:             "Account temporarily locked due to too many failed attempts",
				Locked:            true,
				RetryAfterSeconds: int64(nextLock.Seconds()),
			}, nil
		}
		if ipLock := failed("invalid_credentials", id); ipLock > 0 {
			return ipLockedResult(ipLock), nil
		}
		return &AuthResult{Success: false, Error: "Invalid username or password"}, nil
	}

	// Success — clear any prior failure streak so the next login
	// doesn't start already-counted.
	s.ClearFailedAttemptsContext(ctx, id)

	if _, err := s.db.ExecContext(ctx, "UPDATE users SET last_login = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return nil, fmt.Errorf("updating last login: %w", err)
	}

	// also clear must_reset_password in case we miss the explicit reset path
	if mustReset {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET must_reset_password = 0, updated_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), id); err != nil {
			slog.Warn("auth: clearing must_reset_password failed", "user", id, "error", err)
		}
	}

	// Create session
	sessionID, err := s.CreateSessionContext(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	user := &User{
		ID:                 id,
		Username:           username,
		PreferredLanguage:  normalizePreferredLanguage(preferredLanguage),
		IdentityProviderID: identityProviderID,
		TimeFormat:         timeFormat,
		DateFormat:         dateFormat,
	}
	toast.apply(user)
	look.apply(user)
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if email.Valid {
		user.Email = &email.String
	}

	return &AuthResult{
		Success:           true,
		SessionID:         sessionID,
		User:              user,
		MustResetPassword: mustReset,
	}, nil
}

// Register creates a new user with username/password.
func (s *Service) Register(username, password string, email *string) (*AuthResult, error) {
	return s.RegisterContext(context.Background(), username, password, email)
}

// RegisterContext is Register bound to the caller's context, so a cancelled request stops the user insert and first-admin assignment.
func (s *Service) RegisterContext(ctx context.Context, username, password string, email *string) (*AuthResult, error) {
	// Usernames are case-insensitive; normalise once, up front, so the
	// duplicate check and the INSERT below agree on the same value.
	username = NormalizeUsername(username)

	// Check if username exists
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count); err != nil {
		return nil, fmt.Errorf("checking username: %w", err)
	}
	if count > 0 {
		return &AuthResult{Success: false, Error: "Username already taken"}, nil
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO users (id, username, password_hash, email, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, username, hash, email, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting user: %w", err)
	}

	// Auto-assign Admin role if this is the first login-capable user.
	// Must not count the `__legacy_owner__` placeholder that migration
	// 068 seeds on every DB, or a fresh install's first admin ends up
	// with no role at all and every RBAC-gated route returns 403.
	userCount, err := s.loginCapableUserCount(ctx)
	if err != nil {
		return nil, err
	}
	if userCount == 1 {
		if err := s.AssignAdminRoleContext(ctx, id); err != nil {
			return nil, err
		}
	}

	sessionID, err := s.CreateSessionContext(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	user := &User{
		ID:                id,
		Username:          username,
		PreferredLanguage: "en",
		TimeFormat:        "24h",
		DateFormat:        "ddmmyyyy",
	}
	if email != nil {
		user.Email = email
	}

	return &AuthResult{Success: true, SessionID: sessionID, User: user}, nil
}

// CreateSession creates a new session for a user.
func (s *Service) CreateSession(userID string) (string, error) {
	return s.CreateSessionContext(context.Background(), userID)
}

// CreateSessionContext is CreateSession bound to the caller's context, so a cancelled request stops the session insert.
func (s *Service) CreateSessionContext(ctx context.Context, userID string) (string, error) {
	// Every sign-in path — password, identity provider, passkey, device flow —
	// ends here, so this is the one place a service principal is refused a session.
	if s.IsServicePrincipalContext(ctx, userID) {
		return "", ErrServicePrincipal
	}
	// The id is the bearer credential in the session cookie, so it must be
	// unguessable: an xid is a timestamp, a machine id, a pid and a counter,
	// which anyone holding one session id can walk to the next.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating session id: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	expiresAt := now.Add(SessionDuration).Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (id, user_id, expires_at, created_at, updated_at, last_activity_at) VALUES (?, ?, ?, ?, ?, ?)",
		SessionKey(id), userID, expiresAt, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return id, nil
}

// SessionKey returns the form a session id is stored and looked up under: its SHA-256, hex
// encoded, so a copy of the database or its WAL holds no cookie anyone can replay.
func SessionKey(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])
}

// ValidateSession checks if a session ID is valid and returns the user.
//
// A session has to clear three gates, in cost order: it must not have
// been revoked, it must be inside its absolute lifetime (expires_at),
// and — when automatic sign-out is configured — the user must have been
// active inside the idle window.
func (s *Service) ValidateSession(sessionID string) (*User, error) {
	return s.validateSession(context.Background(), sessionID)
}

// ValidateSessionContext is ValidateSession bound to the caller's context, so a cancelled request stops the session lookups.
func (s *Service) ValidateSessionContext(ctx context.Context, sessionID string) (*User, error) {
	return s.validateSession(ctx, sessionID)
}

// validateSession is ValidateSession with the caller's context, so the HTTP middleware can cancel the lookups with the request.
func (s *Service) validateSession(ctx context.Context, sessionID string) (*User, error) {
	var userID, expiresAt string
	var revokedAt, lastActivityAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id, expires_at, revoked_at, last_activity_at FROM sessions WHERE id = ?", SessionKey(sessionID),
	).Scan(&userID, &expiresAt, &revokedAt, &lastActivityAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}

	// Revoked from Security -> Users -> Sessions, or by a "revoke all"
	// sweep. The row is kept (the UI lists recently-revoked sessions),
	// so the cookie has to be rejected here.
	if revokedAt.Valid && strings.TrimSpace(revokedAt.String) != "" {
		return nil, nil
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		slog.Warn("session has an unparseable expiry, destroying it", "error", err)
		s.destroySession(ctx, sessionID)
		return nil, nil
	}
	if time.Now().UTC().After(expires) {
		s.destroySession(ctx, sessionID)
		return nil, nil
	}

	if idle := s.sessionPolicy(ctx).IdleTimeout(); idle > 0 {
		last, ok := parseSessionTimestamp(lastActivityAt)
		if !ok {
			// Pre-091 session, or a value written by something that
			// did not use RFC3339. Start the clock now rather than
			// signing the operator out for an upgrade they did not ask
			// for; the next touch keeps it moving.
			s.stampActivity(ctx, sessionID)
		} else if time.Since(last) > idle {
			s.destroySession(ctx, sessionID)
			return nil, nil
		}
	}

	var username, preferredLanguage, identityProviderID string
	var timeFormat, dateFormat string
	var toast toastOverrideScan
	var look appearanceOverrideScan
	var displayName, email sql.NullString
	var isActive bool

	err = s.db.QueryRowContext(ctx,
		`SELECT username, display_name, email, is_active,
		        COALESCE(preferred_language, 'en'),
		        COALESCE(identity_provider_id, ''),
		        COALESCE(time_format, '24h'),
		        COALESCE(date_format, 'ddmmyyyy'),
		        toast_position, toast_duration_ms, toast_max_visible,
		        toast_rich_colors, toast_close_button,
		        appearance_theme_id, appearance_accent, appearance_font,
		        appearance_base_font_px
		 FROM users WHERE id = ?`, userID,
	).Scan(append([]any{&username, &displayName, &email, &isActive, &preferredLanguage, &identityProviderID, &timeFormat, &dateFormat}, append(toast.dest(), look.dest()...)...)...)

	if errors.Is(err, sql.ErrNoRows) || !isActive {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	if s.isServicePrincipal(ctx, userID) {
		return nil, nil
	}

	user := &User{
		ID:                 userID,
		Username:           username,
		PreferredLanguage:  normalizePreferredLanguage(preferredLanguage),
		IdentityProviderID: identityProviderID,
		TimeFormat:         timeFormat,
		DateFormat:         dateFormat,
	}
	toast.apply(user)
	look.apply(user)
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if email.Valid {
		user.Email = &email.String
	}

	return user, nil
}

// parseSessionTimestamp reads a stored timestamp. Rows written by Go
// are RFC3339; rows written by a SQL default are `CURRENT_TIMESTAMP`,
// which SQLite renders without the T and the zone. Both are accepted so
// a hand-repaired database does not read as "never active".
func parseSessionTimestamp(v sql.NullString) (time.Time, bool) {
	if !v.Valid {
		return time.Time{}, false
	}
	raw := strings.TrimSpace(v.String)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// stampActivity records "the user did something just now" on a session.
// Failures are logged nowhere and swallowed on purpose: the caller is
// either the middleware healing a NULL column or a fire-and-forget
// keepalive, and neither should turn a write hiccup into a 500.
func (s *Service) stampActivity(ctx context.Context, sessionID string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET last_activity_at = ?, updated_at = ? WHERE id = ?",
		now, now, SessionKey(sessionID),
	); err != nil {
		return
	}
}

// TouchSession records user activity against a session, resetting its
// idle-timeout clock. Returns false when no such session exists (it was
// revoked, expired, or already swept), so the caller can answer 401
// instead of pretending the keepalive landed.
func (s *Service) TouchSession(sessionID string) (bool, error) {
	return s.TouchSessionContext(context.Background(), sessionID)
}

// TouchSessionContext is TouchSession bound to the caller's context, so a cancelled request stops the session update.
func (s *Service) TouchSessionContext(ctx context.Context, sessionID string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_activity_at = ?, updated_at = ?
		  WHERE id = ? AND (revoked_at IS NULL OR revoked_at = '')`,
		now, now, SessionKey(sessionID),
	)
	if err != nil {
		return false, fmt.Errorf("touching session: %w", err)
	}
	return db.RowsAffected(res) > 0, nil
}

// DestroySession removes a session.
func (s *Service) DestroySession(sessionID string) {
	s.destroySession(context.Background(), sessionID)
}

// DestroySessionContext is DestroySession bound to the caller's context, so a cancelled request stops the session delete.
func (s *Service) DestroySessionContext(ctx context.Context, sessionID string) {
	s.destroySession(ctx, sessionID)
}

// destroySession deletes a session row; a failed delete is deliberately quiet because the caller is already rejecting the session.
func (s *Service) destroySession(ctx context.Context, sessionID string) {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", SessionKey(sessionID)); err != nil {
		slog.Warn("deleting session failed", "error", err)
	}
}

// HasAnyUser returns true when at least one login-capable user
// exists. The synthetic `__legacy_owner__` placeholder that several
// migrations (071, 086) insert as a backfill target for legacy
// rows is intentionally excluded — it has no password_hash so it
// can never authenticate, and counting it would suppress the
// first-time setup wizard on a fresh DB.
func (s *Service) HasAnyUser() bool {
	return s.HasAnyUserContext(context.Background())
}

// HasAnyUserContext is HasAnyUser bound to the caller's context, so a cancelled request stops the user count.
func (s *Service) HasAnyUserContext(ctx context.Context) bool {
	count, err := s.loginCapableUserCount(ctx)
	if err != nil {
		return false
	}
	return count > 0
}

// loginCapableUserCount counts users that can actually authenticate.
// Placeholder rows such as `__legacy_owner__` carry an empty
// password_hash and are excluded. Register and HasAnyUser must agree
// on this definition of "first user", so both go through here.
func (s *Service) loginCapableUserCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE password_hash != ''",
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// RepairFirstAdminRole heals installs whose sole login-capable user
// registered while Register still miscounted the `__legacy_owner__`
// placeholder and therefore never received the Admin role. It runs at
// startup and is a no-op unless there is exactly one real user and no
// role assignment at all, so it can never demote or re-grant anything
// on a configured install.
func (s *Service) RepairFirstAdminRole() (bool, error) {
	return s.RepairFirstAdminRoleContext(context.Background())
}

// RepairFirstAdminRoleContext is RepairFirstAdminRole bound to the caller's context, so a cancelled request stops the first-admin repair.
func (s *Service) RepairFirstAdminRoleContext(ctx context.Context) (bool, error) {
	count, err := s.loginCapableUserCount(ctx)
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, nil
	}

	var assignments int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_roles").Scan(&assignments); err != nil {
		return false, fmt.Errorf("counting role assignments: %w", err)
	}
	if assignments > 0 {
		return false, nil
	}

	var userID string
	if err := s.db.QueryRowContext(ctx,
		"SELECT id FROM users WHERE password_hash != '' LIMIT 1",
	).Scan(&userID); err != nil {
		return false, fmt.Errorf("finding first user: %w", err)
	}
	if err := s.AssignAdminRoleContext(ctx, userID); err != nil {
		return false, err
	}
	return true, nil
}

// IsAuthEnabled returns whether auth is enabled.
func (s *Service) IsAuthEnabled() bool {
	return s.authEnabled
}

// SetAuthEnabled sets the auth enabled flag.
func (s *Service) SetAuthEnabled(enabled bool) {
	s.authEnabled = enabled
}

// ValidateUserByID loads a user by ID (for API key middleware).
func (s *Service) ValidateUserByID(userID string) (*User, error) {
	return s.ValidateUserByIDContext(context.Background(), userID)
}

// ValidateUserByIDContext is ValidateUserByID bound to ctx, so a request that goes away also cancels the lookup.
func (s *Service) ValidateUserByIDContext(ctx context.Context, userID string) (*User, error) {
	var username, preferredLanguage, identityProviderID string
	var timeFormat, dateFormat string
	var toast toastOverrideScan
	var look appearanceOverrideScan
	var displayName, email sql.NullString
	var isActive bool

	err := s.db.QueryRowContext(ctx,
		`SELECT username, display_name, email, is_active,
		        COALESCE(preferred_language, 'en'),
		        COALESCE(identity_provider_id, ''),
		        COALESCE(time_format, '24h'),
		        COALESCE(date_format, 'ddmmyyyy'),
		        toast_position, toast_duration_ms, toast_max_visible,
		        toast_rich_colors, toast_close_button,
		        appearance_theme_id, appearance_accent, appearance_font,
		        appearance_base_font_px
		 FROM users WHERE id = ?`, userID,
	).Scan(append([]any{&username, &displayName, &email, &isActive, &preferredLanguage, &identityProviderID, &timeFormat, &dateFormat}, append(toast.dest(), look.dest()...)...)...)

	if errors.Is(err, sql.ErrNoRows) || !isActive {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}
	// The API-key middleware resolves a key's owner here; a service principal
	// owns no key that may authenticate, however the row came to exist.
	if s.isServicePrincipal(ctx, userID) {
		return nil, nil
	}

	user := &User{
		ID:                 userID,
		Username:           username,
		PreferredLanguage:  normalizePreferredLanguage(preferredLanguage),
		IdentityProviderID: identityProviderID,
		TimeFormat:         timeFormat,
		DateFormat:         dateFormat,
	}
	toast.apply(user)
	look.apply(user)
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if email.Valid {
		user.Email = &email.String
	}

	return user, nil
}

// UpdatePreferences stores per-user UI preferences. The input struct
// is the same shape as the auth handler's preferencesRequest; the
// service ignores fields that the user did not include so callers
// can update any subset. The time/date format values are
// normalized to a fixed set so the frontend never has to validate
// them again.
func (s *Service) UpdatePreferences(userID string, prefs UserPreferences) (*User, error) {
	return s.UpdatePreferencesContext(context.Background(), userID, prefs)
}

// UpdatePreferencesContext is UpdatePreferences bound to the caller's context, so a cancelled request stops the preferences update.
func (s *Service) UpdatePreferencesContext(ctx context.Context, userID string, prefs UserPreferences) (*User, error) {
	sets := []string{}
	args := []interface{}{}
	if prefs.PreferredLanguage != nil {
		sets = append(sets, "preferred_language = ?")
		args = append(args, normalizePreferredLanguage(*prefs.PreferredLanguage))
	}
	if prefs.TimeFormat != nil {
		sets = append(sets, "time_format = ?")
		args = append(args, normalizeTimeFormat(*prefs.TimeFormat))
	}
	if prefs.DateFormat != nil {
		sets = append(sets, "date_format = ?")
		args = append(args, normalizeDateFormat(*prefs.DateFormat))
	}
	if prefs.ClearToastOverride {
		// Back to inheriting. All five go at once: a half-cleared row would
		// leave the account following the default for some knobs and its own
		// stale choice for others, which no UI state can describe.
		sets = append(sets,
			"toast_position = NULL",
			"toast_duration_ms = NULL",
			"toast_max_visible = NULL",
			"toast_rich_colors = NULL",
			"toast_close_button = NULL",
		)
	} else {
		if prefs.ToastPosition != nil && coreSettings.ValidToastPosition(*prefs.ToastPosition) {
			sets = append(sets, "toast_position = ?")
			args = append(args, *prefs.ToastPosition)
		}
		if prefs.ToastDurationMs != nil {
			sets = append(sets, "toast_duration_ms = ?")
			args = append(args, clampToastDuration(*prefs.ToastDurationMs))
		}
		if prefs.ToastMaxVisible != nil {
			sets = append(sets, "toast_max_visible = ?")
			args = append(args, clampToastVisible(*prefs.ToastMaxVisible))
		}
		if prefs.ToastRichColors != nil {
			sets = append(sets, "toast_rich_colors = ?")
			args = append(args, *prefs.ToastRichColors)
		}
		if prefs.ToastCloseButton != nil {
			sets = append(sets, "toast_close_button = ?")
			args = append(args, *prefs.ToastCloseButton)
		}
	}
	if prefs.ClearAppearanceOverride {
		// Back to inheriting. All four go at once for the same reason the
		// toast columns do: a half-cleared row would leave the account on the
		// default theme wearing its own font, which no UI state describes.
		sets = append(sets,
			"appearance_theme_id = NULL",
			"appearance_accent = NULL",
			"appearance_font = NULL",
			"appearance_base_font_px = NULL",
		)
	} else {
		if prefs.AppearanceThemeID != nil && coreSettings.ValidAppearanceThemeID(*prefs.AppearanceThemeID) {
			sets = append(sets, "appearance_theme_id = ?")
			args = append(args, *prefs.AppearanceThemeID)
		}
		if prefs.AppearanceAccent != nil && coreSettings.ValidAppearanceAccent(*prefs.AppearanceAccent) {
			sets = append(sets, "appearance_accent = ?")
			args = append(args, *prefs.AppearanceAccent)
		}
		if prefs.AppearanceFont != nil && coreSettings.ValidAppearanceFont(*prefs.AppearanceFont) {
			sets = append(sets, "appearance_font = ?")
			args = append(args, *prefs.AppearanceFont)
		}
		if prefs.AppearanceBaseFontPx != nil {
			sets = append(sets, "appearance_base_font_px = ?")
			args = append(args, coreSettings.ClampAppearanceBaseFontPx(*prefs.AppearanceBaseFontPx))
		}
	}
	if len(sets) == 0 {
		return s.ValidateUserByIDContext(ctx, userID)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	args = append(args, userID)

	if _, err := s.db.ExecContext(ctx,
		"UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id = ?", // SQL identifier: sets only holds fixed "column = ?" fragments built above
		args...,
	); err != nil {
		return nil, fmt.Errorf("updating preferences: %w", err)
	}
	return s.ValidateUserByIDContext(ctx, userID)
}

// UpdateProfile stores editable profile details. The display name
// and email are rejected when the user authenticates through an
// external identity provider (OIDC / SAML) — those values are owned
// by the IdP and can only be changed there.
func (s *Service) UpdateProfile(userID string, displayName string, email string) (*User, error) {
	return s.UpdateProfileContext(context.Background(), userID, displayName, email)
}

// UpdateProfileContext is UpdateProfile bound to the caller's context, so a cancelled request stops the profile update.
func (s *Service) UpdateProfileContext(ctx context.Context, userID string, displayName string, email string) (*User, error) {
	current, err := s.ValidateUserByIDContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	if current.IdentityProviderID != "" {
		return nil, fmt.Errorf("display name and email are managed by your identity provider")
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE users SET display_name = ?, email = ?, updated_at = ? WHERE id = ?",
		nullableString(displayName), nullableString(email), time.Now().UTC().Format(time.RFC3339), userID,
	); err != nil {
		return nil, fmt.Errorf("updating profile: %w", err)
	}
	return s.ValidateUserByIDContext(ctx, userID)
}

// UserPreferences is the set of per-user UI preferences that the
// auth handler accepts via PUT /auth/preferences. Pointer fields
// allow the handler to differentiate "not provided" from
// "explicitly set to empty".
type UserPreferences struct {
	PreferredLanguage *string
	TimeFormat        *string
	DateFormat        *string

	// ClearToastOverride sets every per-user toast column back to NULL so
	// the account follows the app-wide default again. It is how the "use my
	// own notification settings" switch turns off, and it exists because a
	// nil value field already means "leave unchanged" - without a separate
	// flag there would be no way to express "unset this".
	// The five value fields are ignored when it is true.
	ClearToastOverride bool
	ToastPosition      *string
	ToastDurationMs    *int
	ToastMaxVisible    *int
	ToastRichColors    *bool
	ToastCloseButton   *bool

	// ClearAppearanceOverride is the appearance equivalent: it sets every
	// per-user appearance column back to NULL so the account follows the
	// app-wide theme, accent, font and base font size again. The four value
	// fields are ignored when it is true.
	ClearAppearanceOverride bool
	AppearanceThemeID       *string
	AppearanceAccent        *string
	AppearanceFont          *string
	AppearanceBaseFontPx    *int
}

// toastOverrideScan holds the five nullable per-user toast columns as they
// come off a row. Three queries load a user and all three need the same five
// columns, so the scan destinations and the conversion to User live together
// rather than being spelled out at each site.
type toastOverrideScan struct {
	position    sql.NullString
	durationMs  sql.NullInt64
	maxVisible  sql.NullInt64
	richColors  sql.NullBool
	closeButton sql.NullBool
}

// dest returns the scan destinations in the order the SELECTs list the columns.
func (t *toastOverrideScan) dest() []any {
	return []any{&t.position, &t.durationMs, &t.maxVisible, &t.richColors, &t.closeButton}
}

// apply copies the non-NULL columns onto u. A NULL column leaves the field nil,
// which the frontend reads as "inherit the app-wide default".
func (t toastOverrideScan) apply(u *User) {
	if t.position.Valid && coreSettings.ValidToastPosition(t.position.String) {
		v := t.position.String
		u.ToastPosition = &v
	}
	if t.durationMs.Valid {
		v := int(t.durationMs.Int64)
		u.ToastDurationMs = &v
	}
	if t.maxVisible.Valid {
		v := int(t.maxVisible.Int64)
		u.ToastMaxVisible = &v
	}
	if t.richColors.Valid {
		v := t.richColors.Bool
		u.ToastRichColors = &v
	}
	if t.closeButton.Valid {
		v := t.closeButton.Bool
		u.ToastCloseButton = &v
	}
}

// clampToastDuration and clampToastVisible keep a hand-crafted request inside
// the same bounds the settings reader accepts, so a value that survives a write
// is always a value that will be read back.
func clampToastDuration(v int) int {
	if v < coreSettings.ToastMinDurationMs {
		return coreSettings.ToastMinDurationMs
	}
	if v > coreSettings.ToastMaxDurationMs {
		return coreSettings.ToastMaxDurationMs
	}
	return v
}

func clampToastVisible(v int) int {
	if v < coreSettings.ToastMinVisible {
		return coreSettings.ToastMinVisible
	}
	if v > coreSettings.ToastMaxVisible {
		return coreSettings.ToastMaxVisible
	}
	return v
}

func normalizePreferredLanguage(value string) string {
	switch value {
	case "nl", "de", "es", "fr", "pt":
		return value
	default:
		return "en"
	}
}

func normalizeTimeFormat(value string) string {
	switch value {
	case "12h":
		return "12h"
	default:
		return "24h"
	}
}

func normalizeDateFormat(value string) string {
	switch value {
	case "mmddyyyy":
		return "mmddyyyy"
	default:
		return "ddmmyyyy"
	}
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

// AssignAdminRole assigns the Admin system role to a user and adds them to the Admins group (used during initial setup).
func (s *Service) AssignAdminRole(userID string) error {
	return s.AssignAdminRoleContext(context.Background(), userID)
}

// AssignAdminRoleContext is AssignAdminRole bound to the caller's context, so a cancelled request stops the role and group inserts.
func (s *Service) AssignAdminRoleContext(ctx context.Context, userID string) error {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO user_roles (id, user_id, role_id, environment_id, created_at, updated_at)
		 VALUES (?, ?, 'role_admin', NULL, ?, ?)`,
		id, userID, now, now,
	)
	if err != nil {
		return fmt.Errorf("assigning admin role: %w", err)
	}

	gmID := xid.New().String()
	_, err = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO group_members (id, group_id, user_id, created_at, updated_at)
		 VALUES (?, 'grp_admins', ?, ?, ?)`,
		gmID, userID, now, now,
	)
	if err != nil {
		return fmt.Errorf("adding user to admins group: %w", err)
	}
	return nil
}

func splitLast(s, sep string) []string {
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// MustResetPassword reports whether the user still owes a forced
// password rotation.
//
// LoginWithContext clears the flag as a side effect of a successful
// password check, which is fine for a one-step login but loses the
// signal when a second factor defers session creation. The MFA login
// gate re-arms the flag via SetMustResetPassword and reads it back
// here once the second factor lands.
func (s *Service) MustResetPassword(userID string) (bool, error) {
	return s.MustResetPasswordContext(context.Background(), userID)
}

// MustResetPasswordContext is MustResetPassword bound to the caller's context, so a cancelled request stops the flag read.
func (s *Service) MustResetPasswordContext(ctx context.Context, userID string) (bool, error) {
	var mustReset bool
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(must_reset_password, 0) FROM users WHERE id = ?`, userID,
	).Scan(&mustReset)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying must_reset_password: %w", err)
	}
	return mustReset, nil
}

// SetMustResetPassword arms or clears the forced-rotation flag.
func (s *Service) SetMustResetPassword(userID string, mustReset bool) error {
	return s.SetMustResetPasswordContext(context.Background(), userID, mustReset)
}

// SetMustResetPasswordContext is SetMustResetPassword bound to the caller's context, so a cancelled request stops the flag update.
func (s *Service) SetMustResetPasswordContext(ctx context.Context, userID string, mustReset bool) error {
	value := 0
	if mustReset {
		value = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET must_reset_password = ?, updated_at = ? WHERE id = ?`,
		value, time.Now().UTC().Format(time.RFC3339), userID,
	)
	if err != nil {
		return fmt.Errorf("updating must_reset_password: %w", err)
	}
	return nil
}

// appearanceOverrideScan holds the four nullable per-user appearance columns
// as they come off a row. The same three queries that need the toast columns
// need these, so the scan destinations and the conversion to User live
// together rather than being spelled out at each site.
type appearanceOverrideScan struct {
	themeID    sql.NullString
	accent     sql.NullString
	font       sql.NullString
	baseFontPx sql.NullInt64
}

// dest returns the scan destinations in the order the SELECTs list the columns.
func (a *appearanceOverrideScan) dest() []any {
	return []any{&a.themeID, &a.accent, &a.font, &a.baseFontPx}
}

// apply copies the non-NULL columns onto u. A NULL column leaves the field
// nil, which the frontend reads as "inherit the app-wide default". A stored
// value the current build no longer recognises — an accent that was renamed,
// a font that was dropped — is treated the same way, so the account falls
// back to the default rather than rendering nothing.
func (a appearanceOverrideScan) apply(u *User) {
	if a.themeID.Valid && coreSettings.ValidAppearanceThemeID(a.themeID.String) {
		v := a.themeID.String
		u.AppearanceThemeID = &v
	}
	if a.accent.Valid && coreSettings.ValidAppearanceAccent(a.accent.String) {
		v := a.accent.String
		u.AppearanceAccent = &v
	}
	if a.font.Valid && coreSettings.ValidAppearanceFont(a.font.String) {
		v := a.font.String
		u.AppearanceFont = &v
	}
	if a.baseFontPx.Valid {
		v := coreSettings.ClampAppearanceBaseFontPx(int(a.baseFontPx.Int64))
		u.AppearanceBaseFontPx = &v
	}
}
