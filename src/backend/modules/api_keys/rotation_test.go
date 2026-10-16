// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// TestRotateKeepsGrantsAndRevokesTheOldSecret guards the rotation contract: a
// new secret with the same name, scopes, environments and owner; the old key
// revoked; the new plaintext returned once and stored only as a hash.
func TestRotateKeepsGrantsAndRevokesTheOldSecret(t *testing.T) {
	database := openMigratedAPIKeysDB(t)
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('env-1', 'prod')`); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	svc := NewService(database, rbac.NewService(database))
	ctx := context.Background()
	expires := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	created, err := svc.Create(ctx, "user-1", &CreateAPIKeyInput{Name: "ci", Scopes: []string{"volumes.view"}, EnvironmentIDs: []string{"env-1"}, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	old, err := svc.Get(ctx, created.ID)
	if err != nil || old == nil {
		t.Fatalf("loading created key: %v", err)
	}

	rotated, err := svc.Rotate(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID == old.ID || rotated.Key == created.Key || rotated.Name != "ci" || rotated.UserID != "user-1" {
		t.Fatalf("rotated key: %+v", rotated)
	}
	if len(rotated.Scopes) != 1 || rotated.Scopes[0] != "volumes.view" || len(rotated.EnvironmentIDs) != 1 || rotated.EnvironmentIDs[0] != "env-1" {
		t.Fatalf("grants not carried over: %+v", rotated.APIKey)
	}
	if rotated.ExpiresAt == nil {
		t.Fatal("a key with an expiry must keep one")
	}
	after, err := svc.Get(ctx, old.ID)
	if err != nil || after == nil {
		t.Fatalf("reloading rotated key: %v", err)
	}
	if after.IsActive {
		t.Fatal("the old key must be revoked")
	}
	var stored string
	sum := sha256.Sum256([]byte(rotated.Key))
	if err := database.QueryRow(`SELECT key_hash FROM api_keys WHERE id = ?`, rotated.ID).Scan(&stored); err != nil || stored != hex.EncodeToString(sum[:]) {
		t.Fatalf("stored hash: %v", err)
	}
	again, err := svc.Get(ctx, old.ID)
	if err != nil {
		t.Fatalf("reloading revoked key: %v", err)
	}
	if again != nil {
		if _, err := svc.Rotate(ctx, again); !errors.Is(err, ErrNotRotatable) {
			t.Fatalf("rotating a revoked key: %v", err)
		}
	}
}

func TestFlagRotation(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	soon := now.Add(5 * 24 * time.Hour).Format(time.RFC3339)
	later := now.Add(60 * 24 * time.Hour).Format(time.RFC3339)
	items := []APIKey{
		{ID: "old", Status: "active", CreatedAt: now.AddDate(0, 0, -120).Format(time.RFC3339)},
		{ID: "fresh", Status: "active", CreatedAt: now.AddDate(0, 0, -3).Format(time.RFC3339), ExpiresAt: &soon},
		{ID: "fine", Status: "active", CreatedAt: now.AddDate(0, 0, -3).Format(time.RFC3339), ExpiresAt: &later},
		{ID: "revoked", Status: "revoked", CreatedAt: now.AddDate(0, 0, -400).Format(time.RFC3339)},
	}
	FlagRotation(items, now, 0, 0)
	if !items[0].RotationDue || items[0].AgeDays != 120 || items[0].ExpiringSoon {
		t.Fatalf("old: %+v", items[0])
	}
	if items[1].RotationDue || !items[1].ExpiringSoon {
		t.Fatalf("fresh: %+v", items[1])
	}
	if items[2].RotationDue || items[2].ExpiringSoon {
		t.Fatalf("fine: %+v", items[2])
	}
	if items[3].RotationDue {
		t.Fatal("a revoked key has nothing to rotate")
	}
	FlagRotation(items, now, 200, 90)
	if items[0].RotationDue || !items[2].ExpiringSoon {
		t.Fatal("thresholds must come from the request")
	}
}
