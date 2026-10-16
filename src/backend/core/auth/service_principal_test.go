// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"errors"
	"testing"
	"time"
)

// TestServicePrincipalCannotSignIn is the "never signs in" half of the runbook
// service principal. The row is given a real password hash and a live session on
// purpose — the worst case, a row edited by hand — so the refusal is shown to
// come from the principal kind and not from an empty hash failing to verify.
func TestServicePrincipalCannotSignIn(t *testing.T) {
	svc, database := newSudoTestService(t)
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id, username, password_hash, is_active, principal_kind, auth_provider)
		 VALUES ('svc-1', 'runbook-bot', ?, 1, 'service', 'service')`, hash); err != nil {
		t.Fatalf("seeding service principal: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('sess-svc', 'svc-1', ?)`,
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seeding session: %v", err)
	}

	res, err := svc.Login("runbook-bot", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Success || res.SessionID != "" {
		t.Fatal("a service principal signed in with a password")
	}
	if res.Error != "Invalid username or password" {
		t.Fatalf("the refusal must read like a wrong password, got %q", res.Error)
	}

	if _, err := svc.CreateSession("svc-1"); !errors.Is(err, ErrServicePrincipal) {
		t.Fatalf("CreateSession for a service principal: want ErrServicePrincipal, got %v", err)
	}
	if user, err := svc.ValidateSession("sess-svc"); err != nil || user != nil {
		t.Fatalf("a pre-existing session for a service principal validated: %v %v", user, err)
	}
	if user, err := svc.ValidateUserByID("svc-1"); err != nil || user != nil {
		t.Fatalf("the API-key owner lookup resolved a service principal: %v %v", user, err)
	}
	if err := svc.SetPassword("svc-1", hash, PasswordPolicy{}); !errors.Is(err, ErrServicePrincipal) {
		t.Fatalf("SetPassword for a service principal: want ErrServicePrincipal, got %v", err)
	}

	human, err := svc.Login("alice", "correct horse battery staple")
	if err != nil || !human.Success {
		t.Fatalf("an ordinary user must still sign in: %+v %v", human, err)
	}
}
