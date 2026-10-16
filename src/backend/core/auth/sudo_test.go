// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

type stubTOTP struct{ accept string }

func (s stubTOTP) VerifyForLogin(_ context.Context, _, code string) error {
	if code == s.accept {
		return nil
	}
	return errors.New("invalid code")
}

func newSudoTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "sudo_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashing fixture password: %v", err)
	}
	if _, err := database.Exec(
		"INSERT INTO users (id, username, password_hash, is_active) VALUES ('user-1', 'alice', ?, 1)", hash,
	); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, 'user-1', ?)`, SessionKey("sess-1"),
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seeding session: %v", err)
	}

	return NewService(database), database
}

func TestMintSudoWindowWithPassword(t *testing.T) {
	svc, _ := newSudoTestService(t)
	ctx := context.Background()

	ok, err := svc.HasSudoWindow(ctx, "sess-1", SudoScopeSecretsReveal)
	if err != nil {
		t.Fatalf("checking window: %v", err)
	}
	if ok {
		t.Fatal("a fresh session must not already hold a step-up window")
	}

	expiresAt, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal,
		"correct horse battery staple", "", nil)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if time.Until(expiresAt) > SudoWindowTTL+time.Second {
		t.Errorf("window outlives its TTL: %v", time.Until(expiresAt))
	}

	ok, err = svc.HasSudoWindow(ctx, "sess-1", SudoScopeSecretsReveal)
	if err != nil {
		t.Fatalf("checking window: %v", err)
	}
	if !ok {
		t.Fatal("the minted window is not visible")
	}
}

func TestMintSudoWindowRejectsAWrongPassword(t *testing.T) {
	svc, _ := newSudoTestService(t)
	_, err := svc.MintSudoWindow(context.Background(), "user-1", "sess-1",
		SudoScopeSecretsReveal, "wrong", "", nil)
	if !errors.Is(err, ErrSudoVerificationFailed) {
		t.Fatalf("expected ErrSudoVerificationFailed, got %v", err)
	}
}

func TestMintSudoWindowWithTOTP(t *testing.T) {
	svc, _ := newSudoTestService(t)
	ctx := context.Background()

	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal,
		"", "000000", stubTOTP{accept: "123456"}); !errors.Is(err, ErrSudoVerificationFailed) {
		t.Fatalf("a wrong code must be refused, got %v", err)
	}
	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal,
		"", "123456", stubTOTP{accept: "123456"}); err != nil {
		t.Fatalf("a correct code should mint a window: %v", err)
	}
}

// TestSudoWindowIsScoped pins the property that makes a single primitive safe
// to share: a window minted to delete an environment must not authorise a
// credential disclosure.
func TestSudoWindowIsScoped(t *testing.T) {
	svc, _ := newSudoTestService(t)
	ctx := context.Background()

	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeEnvironmentDelete,
		"correct horse battery staple", "", nil); err != nil {
		t.Fatalf("minting: %v", err)
	}

	ok, err := svc.HasSudoWindow(ctx, "sess-1", SudoScopeSecretsReveal)
	if err != nil {
		t.Fatalf("checking window: %v", err)
	}
	if ok {
		t.Fatal("a window minted for one scope must not satisfy another")
	}
}

// TestSudoWindowIsBoundToTheSession is the point of the whole primitive: it is
// what an approved elevation cannot do, because an elevation follows the user
// and a stolen cookie is still that user.
func TestSudoWindowIsBoundToTheSession(t *testing.T) {
	svc, database := newSudoTestService(t)
	ctx := context.Background()

	if _, err := database.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, 'user-1', ?)`, SessionKey("sess-2"),
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seeding second session: %v", err)
	}
	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal,
		"correct horse battery staple", "", nil); err != nil {
		t.Fatalf("minting: %v", err)
	}

	ok, err := svc.HasSudoWindow(ctx, "sess-2", SudoScopeSecretsReveal)
	if err != nil {
		t.Fatalf("checking window: %v", err)
	}
	if ok {
		t.Fatal("the same user's other session must not inherit the window")
	}
}

func TestSudoWindowExpires(t *testing.T) {
	svc, database := newSudoTestService(t)
	ctx := context.Background()

	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal,
		"correct horse battery staple", "", nil); err != nil {
		t.Fatalf("minting: %v", err)
	}
	if _, err := database.Exec("UPDATE sudo_windows SET expires_at = ?",
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("ageing the window: %v", err)
	}

	ok, err := svc.HasSudoWindow(ctx, "sess-1", SudoScopeSecretsReveal)
	if err != nil {
		t.Fatalf("checking window: %v", err)
	}
	if ok {
		t.Fatal("an expired window must not satisfy the check")
	}
}

func TestRevokeSudoWindows(t *testing.T) {
	svc, _ := newSudoTestService(t)
	ctx := context.Background()

	if _, err := svc.MintSudoWindow(ctx, "user-1", "sess-1", SudoScopeSecretsReveal,
		"correct horse battery staple", "", nil); err != nil {
		t.Fatalf("minting: %v", err)
	}
	if err := svc.RevokeSudoWindows(ctx, "sess-1"); err != nil {
		t.Fatalf("revoking: %v", err)
	}

	ok, err := svc.HasSudoWindow(ctx, "sess-1", SudoScopeSecretsReveal)
	if err != nil {
		t.Fatalf("checking window: %v", err)
	}
	if ok {
		t.Fatal("a revoked window must not survive")
	}
}
