// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const (
	ldapProvider = "prov-ldap"
	ldapDN       = "uid=dana,ou=people,dc=example,dc=test"
	ldapUserID   = "u-dana"
	ldapClientIP = "203.0.113.40"
)

// fakeProvisioner links one directory entry to one account, standing in for the
// identity module so these tests see only the auth module's accounting.
type fakeProvisioner struct {
	linked      bool
	provisioned int
}

func (f *fakeProvisioner) ExternalProviderConfig(context.Context, string, string) (map[string]string, error) {
	return map[string]string{"url": "ldap://directory.test"}, nil
}

func (f *fakeProvisioner) LinkedUserID(_ context.Context, _, externalID, _ string) (string, error) {
	if f.linked && externalID == ldapDN {
		return ldapUserID, nil
	}
	return "", nil
}

func (f *fakeProvisioner) ProvisionExternalUser(_ context.Context, _, externalID, _, _, _ string, _ []string) (string, error) {
	f.provisioned++
	if externalID != ldapDN {
		return "", nil
	}
	f.linked = true
	return ldapUserID, nil
}

// fakeMFA reports the account as enrolled or not and hands out one challenge.
type fakeMFA struct {
	noMFA
	enabled bool
}

func (f fakeMFA) IsEnabled(context.Context, string) (bool, error) { return f.enabled, nil }
func (f fakeMFA) CreateChallenge(context.Context, string) (string, error) {
	if !f.enabled {
		return "", ErrMFANotEnrolled
	}
	return "challenge-1", nil
}

func newExternalLoginFixture(t *testing.T, mfaEnabled bool, maxPerIP int) (*ExternalLogin, *fakeProvisioner, *sql.DB) {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "external_login.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	if _, err := database.Exec(`UPDATE settings SET value = ? WHERE key = ?`, fmt.Sprint(maxPerIP), coreauth.SettingLockoutIPMaxAttempts); err != nil {
		t.Fatalf("setting per-address threshold: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO identity_providers (id, name, provider_type, enabled, client_id, client_secret) VALUES (?, 'Corp LDAP', 'ldap', 1, '', '')`, ldapProvider,
	); err != nil {
		t.Fatalf("seeding provider: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id, username, password_hash, is_active, external_id, identity_provider_id)
		 VALUES (?, 'ldap_dana', 'x', 1, ?, ?)`, ldapUserID, ldapDN, ldapProvider,
	); err != nil {
		t.Fatalf("seeding linked user: %v", err)
	}
	app := &router.AppDeps{
		Config:      &config.Config{},
		AuthService: coreauth.NewService(database),
		AuditLog:    audit.NewLogger(database, "test-secret"),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	provisioner := &fakeProvisioner{linked: true}
	return NewExternalLogin(app, fakeMFA{enabled: mfaEnabled}, provisioner), provisioner, database
}

func externalRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/identity-ldap/auth", nil)
	req.RemoteAddr = ldapClientIP + ":5555"
	req.Header.Set("User-Agent", "ldap-test/1.0")
	return req
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body.String(), err)
	}
	return body
}

func queryInt(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == coreauth.SessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestExternalLoginLockedAddressIsRefusedBeforeTheDirectory(t *testing.T) {
	login, _, database := newExternalLoginFixture(t, false, 2)
	ctx := t.Context()
	for i := 0; i < 2; i++ {
		login.FailExternalLogin(ctx, httptest.NewRecorder(), externalRequest(), ldapProvider, "ghost", "", ExternalReasonInvalidCredentials)
	}

	rec := httptest.NewRecorder()
	err := login.BeginExternalLogin(ctx, rec, externalRequest(), ldapProvider, "dana")
	if !errors.Is(err, ErrExternalLoginRefused) {
		t.Fatalf("BeginExternalLogin from a locked address = %v; want ErrExternalLoginRefused so the plugin never binds", err)
	}
	if rec.Code != http.StatusLocked {
		t.Fatalf("status = %d; want 423 like a password login from a locked address", rec.Code)
	}
	if body := decodeBody(t, rec); body["locked"] != true {
		t.Fatalf("refusal body %v does not carry locked:true", body)
	}
	if n := queryInt(t, database, `SELECT COUNT(*) FROM login_attempts WHERE ip_address = ? AND reason = 'ip_locked' AND ip_source <> ''`, ldapClientIP); n != 1 {
		t.Fatalf("ip_locked attempts with an ip_source = %d; want 1", n)
	}

	fresh := httptest.NewRecorder()
	other := externalRequest()
	other.RemoteAddr = "198.51.100.9:5555"
	if err := login.BeginExternalLogin(ctx, fresh, other, ldapProvider, "dana"); err != nil {
		t.Fatalf("an unlocked address was refused: %v", err)
	}
}

func TestExternalLoginFailedBindCountsAgainstAddressAndAccount(t *testing.T) {
	login, _, database := newExternalLoginFixture(t, false, 30)
	ctx := t.Context()

	rec := httptest.NewRecorder()
	login.FailExternalLogin(ctx, rec, externalRequest(), ldapProvider, "dana", ldapDN, ExternalReasonInvalidCredentials)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	if body := decodeBody(t, rec); body["error"] != "Invalid username or password" {
		t.Fatalf("error = %v; want the local login's generic message", body["error"])
	}
	if n := queryInt(t, database, `SELECT password_failed_count FROM users WHERE id = ?`, ldapUserID); n != 1 {
		t.Fatalf("account failure count = %d; want 1", n)
	}
	if n := queryInt(t, database, `SELECT COUNT(*) FROM login_attempts WHERE ip_address = ? AND succeeded = 0 AND reason = 'invalid_credentials' AND user_id = ? AND ip_source <> ''`, ldapClientIP, ldapUserID); n != 1 {
		t.Fatalf("attributed failed attempts = %d; want 1 (the per-address counter reads these rows)", n)
	}

	for i := 0; i < 4; i++ {
		login.FailExternalLogin(ctx, httptest.NewRecorder(), externalRequest(), ldapProvider, "dana", ldapDN, ExternalReasonInvalidCredentials)
	}
	gate := httptest.NewRecorder()
	if err := login.CheckExternalAccount(ctx, gate, externalRequest(), ldapProvider, "dana", ldapDN, ""); !errors.Is(err, ErrExternalLoginRefused) {
		t.Fatalf("CheckExternalAccount after five failures = %v; want a refusal before the user's bind", err)
	}
	if gate.Code != http.StatusLocked {
		t.Fatalf("locked account status = %d; want 423", gate.Code)
	}
}

func TestExternalLoginOutageIsNotCounted(t *testing.T) {
	login, _, database := newExternalLoginFixture(t, false, 30)
	rec := httptest.NewRecorder()
	login.FailExternalLogin(t.Context(), rec, externalRequest(), ldapProvider, "dana", ldapDN, ExternalReasonUnavailable)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", rec.Code)
	}
	if n := queryInt(t, database, `SELECT COUNT(*) FROM login_attempts`); n != 0 {
		t.Fatalf("an outage wrote %d login_attempts rows; it must not count toward lockout", n)
	}
	if n := queryInt(t, database, `SELECT password_failed_count FROM users WHERE id = ?`, ldapUserID); n != 0 {
		t.Fatalf("an outage counted against the account: %d", n)
	}
}

func TestExternalLoginWithMFAReturnsTheChallengeNotASession(t *testing.T) {
	login, _, database := newExternalLoginFixture(t, true, 30)
	rec := httptest.NewRecorder()
	if err := login.CompleteExternalLogin(t.Context(), rec, externalRequest(), ldapProvider, ldapDN, "dana", "", "Dana", nil); err != nil {
		t.Fatalf("CompleteExternalLogin: %v", err)
	}
	body := decodeBody(t, rec)
	if body["mfaRequired"] != true || body["challengeId"] != "challenge-1" {
		t.Fatalf("body %v; want the MFA challenge", body)
	}
	if c := sessionCookie(rec); c != nil {
		t.Fatalf("an MFA account got a session cookie before its second factor: %v", c)
	}
	if n := queryInt(t, database, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, ldapUserID); n != 0 {
		t.Fatalf("%d sessions left behind for an MFA account", n)
	}
}

func TestExternalLoginWithoutMFASetsTheSessionCookie(t *testing.T) {
	login, _, database := newExternalLoginFixture(t, false, 30)
	if _, err := database.Exec(`UPDATE users SET password_failed_count = 3 WHERE id = ?`, ldapUserID); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := login.CompleteExternalLogin(t.Context(), rec, externalRequest(), ldapProvider, ldapDN, "dana", "", "Dana", nil); err != nil {
		t.Fatalf("CompleteExternalLogin: %v", err)
	}
	cookie := sessionCookie(rec)
	if cookie == nil || !cookie.HttpOnly {
		t.Fatalf("no httpOnly session cookie set: %v", rec.Result().Cookies())
	}
	if n := queryInt(t, database, `SELECT COUNT(*) FROM sessions WHERE id = ? AND user_id = ?`, coreauth.SessionKey(cookie.Value), ldapUserID); n != 1 {
		t.Fatalf("the cookie names no session for the linked account")
	}
	body := decodeBody(t, rec)
	user, _ := body["user"].(map[string]any)
	if user["id"] != ldapUserID {
		t.Fatalf("body %v; want the flat {user} shape the login page reads", body)
	}
	if n := queryInt(t, database, `SELECT password_failed_count FROM users WHERE id = ?`, ldapUserID); n != 0 {
		t.Fatalf("a successful sign-in left the failure streak at %d", n)
	}
	if n := queryInt(t, database, `SELECT COUNT(*) FROM audit_logs WHERE action = 'login' AND entity_id = ?`, ldapUserID); n != 1 {
		t.Fatalf("login audit entries = %d; want 1", n)
	}
}

func TestExternalLoginRefusesADisabledAccount(t *testing.T) {
	login, _, database := newExternalLoginFixture(t, false, 30)
	if _, err := database.Exec(`UPDATE users SET is_active = 0 WHERE id = ?`, ldapUserID); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := login.CompleteExternalLogin(t.Context(), rec, externalRequest(), ldapProvider, ldapDN, "dana", "", "Dana", nil); !errors.Is(err, ErrExternalLoginRefused) {
		t.Fatalf("CompleteExternalLogin for a disabled account = %v; want a refusal", err)
	}
	if rec.Code != http.StatusForbidden || sessionCookie(rec) != nil {
		t.Fatalf("disabled account: status %d cookie %v; want 403 and no session", rec.Code, sessionCookie(rec))
	}
}
