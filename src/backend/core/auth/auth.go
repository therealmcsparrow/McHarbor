// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
	"golang.org/x/crypto/argon2"
)

const (
	SessionCookie   = "mcharbor_session"
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
}

// AuthResult is returned from login/register.
type AuthResult struct {
	Success   bool   `json:"success"`
	SessionID string `json:"sessionId,omitempty"`
	User      *User  `json:"user,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Service handles authentication operations.
type Service struct {
	db          *sql.DB
	authEnabled bool
}

// NewService creates a new auth service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
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
	var id, passwordHash, preferredLanguage, identityProviderID string
	var timeFormat, dateFormat string
	var displayName, email sql.NullString
	var isActive bool

	err := s.db.QueryRow(
		`SELECT id, password_hash, display_name, email, is_active,
		        COALESCE(preferred_language, 'en'),
		        COALESCE(identity_provider_id, ''),
		        COALESCE(time_format, '24h'),
		        COALESCE(date_format, 'ddmmyyyy')
		 FROM users WHERE username = ?`,
		username,
	).Scan(&id, &passwordHash, &displayName, &email, &isActive, &preferredLanguage, &identityProviderID, &timeFormat, &dateFormat)

	if err == sql.ErrNoRows {
		return &AuthResult{Success: false, Error: "Invalid username or password"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}

	if !isActive {
		return &AuthResult{Success: false, Error: "Account is disabled"}, nil
	}

	valid, err := VerifyPassword(password, passwordHash)
	if err != nil || !valid {
		return &AuthResult{Success: false, Error: "Invalid username or password"}, nil
	}

	if _, err := s.db.Exec("UPDATE users SET last_login = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return nil, fmt.Errorf("updating last login: %w", err)
	}

	// Create session
	sessionID, err := s.CreateSession(id)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	user := &User{
		ID:                  id,
		Username:            username,
		PreferredLanguage:   normalizePreferredLanguage(preferredLanguage),
		IdentityProviderID:  identityProviderID,
		TimeFormat:          timeFormat,
		DateFormat:          dateFormat,
	}
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if email.Valid {
		user.Email = &email.String
	}

	return &AuthResult{Success: true, SessionID: sessionID, User: user}, nil
}

// Register creates a new user with username/password.
func (s *Service) Register(username, password string, email *string) (*AuthResult, error) {
	// Check if username exists
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count); err != nil {
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

	_, err = s.db.Exec(
		"INSERT INTO users (id, username, password_hash, email, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, username, hash, email, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting user: %w", err)
	}

	// Auto-assign Admin role if this is the first user
	var userCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}
	if userCount == 1 {
		if err := s.AssignAdminRole(id); err != nil {
			return nil, err
		}
	}

	sessionID, err := s.CreateSession(id)
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
	id := xid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(SessionDuration).Format(time.RFC3339)

	_, err := s.db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, userID, expiresAt, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return id, nil
}

// ValidateSession checks if a session ID is valid and returns the user.
func (s *Service) ValidateSession(sessionID string) (*User, error) {
	var userID, expiresAt string
	err := s.db.QueryRow(
		"SELECT user_id, expires_at FROM sessions WHERE id = ?", sessionID,
	).Scan(&userID, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		s.DestroySession(sessionID)
		return nil, nil
	}
	if time.Now().UTC().After(expires) {
		s.DestroySession(sessionID)
		return nil, nil
	}

	var username, preferredLanguage, identityProviderID string
	var timeFormat, dateFormat string
	var displayName, email sql.NullString
	var isActive bool

	err = s.db.QueryRow(
		`SELECT username, display_name, email, is_active,
		        COALESCE(preferred_language, 'en'),
		        COALESCE(identity_provider_id, ''),
		        COALESCE(time_format, '24h'),
		        COALESCE(date_format, 'ddmmyyyy')
		 FROM users WHERE id = ?`, userID,
	).Scan(&username, &displayName, &email, &isActive, &preferredLanguage, &identityProviderID, &timeFormat, &dateFormat)

	if err == sql.ErrNoRows || !isActive {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}

	user := &User{
		ID:                 userID,
		Username:           username,
		PreferredLanguage:  normalizePreferredLanguage(preferredLanguage),
		IdentityProviderID: identityProviderID,
		TimeFormat:         timeFormat,
		DateFormat:         dateFormat,
	}
	if displayName.Valid {
		user.DisplayName = &displayName.String
	}
	if email.Valid {
		user.Email = &email.String
	}

	return user, nil
}

// DestroySession removes a session.
func (s *Service) DestroySession(sessionID string) {
	if _, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", sessionID); err != nil {
		return
	}
}

// HasAnyUser returns true if at least one user exists.
func (s *Service) HasAnyUser() bool {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return false
	}
	return count > 0
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
	var username, preferredLanguage, identityProviderID string
	var timeFormat, dateFormat string
	var displayName, email sql.NullString
	var isActive bool

	err := s.db.QueryRow(
		`SELECT username, display_name, email, is_active,
		        COALESCE(preferred_language, 'en'),
		        COALESCE(identity_provider_id, ''),
		        COALESCE(time_format, '24h'),
		        COALESCE(date_format, 'ddmmyyyy')
		 FROM users WHERE id = ?`, userID,
	).Scan(&username, &displayName, &email, &isActive, &preferredLanguage, &identityProviderID, &timeFormat, &dateFormat)

	if err == sql.ErrNoRows || !isActive {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}

	user := &User{
		ID:                 userID,
		Username:           username,
		PreferredLanguage:  normalizePreferredLanguage(preferredLanguage),
		IdentityProviderID: identityProviderID,
		TimeFormat:         timeFormat,
		DateFormat:         dateFormat,
	}
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
	if len(sets) == 0 {
		return s.ValidateUserByID(userID)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	args = append(args, userID)

	if _, err := s.db.Exec(
		"UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id = ?",
		args...,
	); err != nil {
		return nil, fmt.Errorf("updating preferences: %w", err)
	}
	return s.ValidateUserByID(userID)
}

// UpdateProfile stores editable profile details. The display name
// and email are rejected when the user authenticates through an
// external identity provider (OIDC / SAML) — those values are owned
// by the IdP and can only be changed there.
func (s *Service) UpdateProfile(userID string, displayName string, email string) (*User, error) {
	current, err := s.ValidateUserByID(userID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	if current.IdentityProviderID != "" {
		return nil, fmt.Errorf("display name and email are managed by your identity provider")
	}
	if _, err := s.db.Exec(
		"UPDATE users SET display_name = ?, email = ?, updated_at = ? WHERE id = ?",
		nullableString(displayName), nullableString(email), time.Now().UTC().Format(time.RFC3339), userID,
	); err != nil {
		return nil, fmt.Errorf("updating profile: %w", err)
	}
	return s.ValidateUserByID(userID)
}

// UserPreferences is the set of per-user UI preferences that the
// auth handler accepts via PUT /auth/preferences. Pointer fields
// allow the handler to differentiate "not provided" from
// "explicitly set to empty".
type UserPreferences struct {
	PreferredLanguage *string
	TimeFormat        *string
	DateFormat        *string
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
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO user_roles (id, user_id, role_id, environment_id, created_at, updated_at)
		 VALUES (?, ?, 'role_admin', NULL, ?, ?)`,
		id, userID, now, now,
	)
	if err != nil {
		return fmt.Errorf("assigning admin role: %w", err)
	}

	gmID := xid.New().String()
	_, err = s.db.Exec(
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
