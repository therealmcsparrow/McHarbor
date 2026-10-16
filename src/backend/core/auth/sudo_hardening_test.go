// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

// TestSessionIDsAreUnguessable guards the cookie credential: it used to be an
// xid (time, machine, pid, counter), from which one session id predicts others.
func TestSessionIDsAreUnguessable(t *testing.T) {
	svc, _ := newSudoTestService(t)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id, err := svc.CreateSession("user-1")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil || len(raw) != 32 {
			t.Fatalf("session id %q is not 32 random bytes, base64url: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate session id %q", id)
		}
		seen[id] = true
	}
}

// TestSudoRefusesSomeoneElsesSession: the window binds to the cookie's session,
// so a user cannot open one on a session that is not theirs.
func TestSudoRefusesSomeoneElsesSession(t *testing.T) {
	svc, database := newSudoTestService(t)
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash, is_active) VALUES ('user-2', 'bob', 'x', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, 'user-2', ?)`, SessionKey("sess-bob"),
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	_, err := svc.MintSudoWindow(context.Background(), "user-1", "sess-bob", SudoScopeSecretsReveal,
		"correct horse battery staple", "", nil)
	if !errors.Is(err, ErrSudoVerificationFailed) {
		t.Fatalf("a window on another user's session: got %v, want ErrSudoVerificationFailed", err)
	}
}

// TestSudoFailuresLockTheAccount: wrong step-up passwords count like failed
// logins, and a locked account cannot open a window even with the right one.
func TestSudoFailuresLockTheAccount(t *testing.T) {
	svc, database := newSudoTestService(t)
	ctx := context.Background()
	policy := LoadLockoutPolicy(ctx, database)
	for i := 0; i < policy.MaxAttempts+1; i++ {
		if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal, "wrong", "", nil); !errors.Is(err, ErrSudoVerificationFailed) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if locked, err := svc.IsLocked("user-1"); err != nil || locked == 0 {
		t.Fatalf("repeated step-up failures must lock the account: locked=%v err=%v", locked, err)
	}
	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal, "correct horse battery staple", "", nil); !errors.Is(err, ErrSudoVerificationFailed) {
		t.Fatalf("a locked account opened a window: %v", err)
	}
}
