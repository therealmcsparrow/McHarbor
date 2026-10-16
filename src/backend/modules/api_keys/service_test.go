// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// openMigratedAPIKeysDB runs the real migration set, so the test cannot drift
// from the schema the service queries the way a hand-written fixture does.
func openMigratedAPIKeysDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "apikeys.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash) VALUES ('user-1', 'carlo', 'x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return database
}

// TestCreateReturnsTheSecretOnlyOnce guards the credential contract: the
// plaintext is in the create result, only its hash is stored, and reads never carry it.
func TestCreateReturnsTheSecretOnlyOnce(t *testing.T) {
	database := openMigratedAPIKeysDB(t)
	svc := NewService(database, rbac.NewService(database))
	ctx := context.Background()

	result, err := svc.Create(ctx, "user-1", &CreateAPIKeyInput{Name: "ci", Scopes: []string{"volumes.view"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(result.Key, keyPrefix) || result.KeyPrefix != result.Key[:12]+"..." {
		t.Fatalf("key %q / prefix %q do not match the documented shape", result.Key, result.KeyPrefix)
	}

	var storedHash string
	if err := database.QueryRow(`SELECT key_hash FROM api_keys WHERE id = ?`, result.ID).Scan(&storedHash); err != nil {
		t.Fatalf("reading hash: %v", err)
	}
	sum := sha256.Sum256([]byte(result.Key))
	if storedHash != hex.EncodeToString(sum[:]) || strings.Contains(storedHash, result.Key) {
		t.Fatal("the stored value must be the SHA-256 of the key, never the key")
	}

	got, err := svc.Get(ctx, result.ID)
	if err != nil || got == nil {
		t.Fatalf("Get: %v %v", got, err)
	}
	if got.Name != "ci" || !got.IsActive || len(got.Scopes) != 1 || got.Scopes[0] != "volumes.view" {
		t.Fatalf("Get = %+v", got)
	}
}

// TestLifecycleRefusesDeletingAnActiveKey keeps delete behind revoke, so a
// live credential is never destroyed without first being switched off.
func TestLifecycleRefusesDeletingAnActiveKey(t *testing.T) {
	database := openMigratedAPIKeysDB(t)
	svc := NewService(database, rbac.NewService(database))
	ctx := context.Background()

	created, err := svc.Create(ctx, "user-1", &CreateAPIKeyInput{Name: "old", Scopes: []string{"volumes.view"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	name := "renamed"
	updated, err := svc.Update(ctx, created.ID, &UpdateAPIKeyInput{Name: &name})
	if err != nil || updated.Name != "renamed" || len(updated.Scopes) != 1 {
		t.Fatalf("Update = %+v, %v (nil scopes must leave grants untouched)", updated, err)
	}

	if err := svc.Delete(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Delete of an active key = %v, want sql.ErrNoRows", err)
	}
	if err := svc.Revoke(ctx, created.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	revoked, err := svc.Get(ctx, created.ID)
	if err != nil || revoked == nil || revoked.IsActive {
		t.Fatalf("after Revoke = %+v, %v", revoked, err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete of a revoked key: %v", err)
	}
	if gone, err := svc.Get(ctx, created.ID); err != nil || gone != nil {
		t.Fatalf("after Delete = %+v, %v", gone, err)
	}
	if err := svc.Revoke(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Revoke(missing) = %v, want sql.ErrNoRows", err)
	}
}

// TestValidateGrantsRejectsEmptyAndUnknownGrants covers the two refusals that
// need no role setup: a key with no grants, and a scope no catalog defines.
func TestValidateGrantsRejectsEmptyAndUnknownGrants(t *testing.T) {
	database := openMigratedAPIKeysDB(t)
	svc := NewService(database, rbac.NewService(database))
	ctx := context.Background()

	if err := svc.ValidateGrants(ctx, "system", nil, nil, nil, true); !errors.Is(err, ErrGrantsRequired) {
		t.Fatalf("empty grants = %v, want ErrGrantsRequired", err)
	}
	if err := svc.ValidateGrants(ctx, "system", nil, []string{"no.such_key"}, nil, true); !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("unknown scope = %v, want ErrGrantInvalid", err)
	}
	if err := svc.ValidateGrants(ctx, "system", nil, []string{"*"}, nil, true); err != nil {
		t.Fatalf("system caller granting the wildcard = %v, want nil", err)
	}
}
