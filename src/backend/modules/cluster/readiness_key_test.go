// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"testing"
	"time"
)

// TestKeyGateReadsThePinnedKeyFromConfig guards the ENCRYPTION_KEY_FILE form: that key never
// appears in the environment, so a gate reading os.Getenv reported it as generated per node.
func TestKeyGateReadsThePinnedKeyFromConfig(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")
	database := migrated(t)
	hasher := fixedHasher("k")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO cluster_state (id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"key-check", keyCheckName, hasher.StableHash("mcharbor-key-check"), now, now); err != nil {
		t.Fatal(err)
	}
	svc := NewService(database, nil, quiet(), nil)
	svc.SetKeyHasher(hasher)

	gate := func() Gate {
		for _, g := range svc.Readiness(context.Background(), ModeActiveActive).Gates {
			if g.ID == "encryptionKey" {
				return g
			}
		}
		t.Fatal("no encryptionKey gate")
		return Gate{}
	}

	svc.SetKeyPinned(true)
	if g := gate(); g.Status != GatePass {
		t.Fatalf("a key supplied through config must pass with ENCRYPTION_KEY unset, got %s: %s", g.Status, g.Detail)
	}
	svc.SetKeyPinned(false)
	if g := gate(); g.Status != GateFail {
		t.Fatalf("a generated key must still fail, got %s", g.Status)
	}
}
