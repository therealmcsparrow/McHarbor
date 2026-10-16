// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"database/sql"
	"testing"
)

// seedWebhook inserts a webhook with an encrypted signing secret, which is the
// shape every adopted credential has: an `enc:v1:` blob in a module's own
// column and a NULL pointer beside it.
func seedWebhook(t *testing.T, svc *Service, database *sql.DB, id, name, plaintext string) {
	t.Helper()
	ciphertext, err := svc.enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypting fixture: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO webhooks (id, name, url, secret, events, is_active, created_at, updated_at)
		 VALUES (?, ?, 'https://example.test', ?, '[]', 1,
		         strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		id, name, ciphertext,
	); err != nil {
		t.Fatalf("seeding webhook: %v", err)
	}
}

func webhookSecretID(t *testing.T, database *sql.DB, id string) sql.NullString {
	t.Helper()
	var secretID sql.NullString
	if err := database.QueryRow("SELECT secret_secret_id FROM webhooks WHERE id = ?", id).
		Scan(&secretID); err != nil {
		t.Fatalf("reading secret pointer: %v", err)
	}
	return secretID
}

func TestAdoptLegacyColumnIsIdempotent(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	spec := LegacyCatalog()[0] // webhooks

	seedWebhook(t, svc, database, "wh-1", "Deploy notifier", "sig_abc123")

	if err := svc.AdoptLegacyColumns(ctx, spec); err != nil {
		t.Fatalf("first adoption pass: %v", err)
	}
	first := webhookSecretID(t, database, "wh-1")
	if !first.Valid || first.String == "" {
		t.Fatal("the first pass did not link the adopted secret")
	}

	// A second boot must find nothing to do and must not mint a second copy.
	if err := svc.AdoptLegacyColumns(ctx, spec); err != nil {
		t.Fatalf("second adoption pass: %v", err)
	}
	second := webhookSecretID(t, database, "wh-1")
	if second.String != first.String {
		t.Errorf("the second pass re-adopted the row: %q -> %q", first.String, second.String)
	}

	var count int
	if err := database.QueryRow(
		"SELECT COUNT(*) FROM secrets WHERE origin_module = 'webhooks'").Scan(&count); err != nil {
		t.Fatalf("counting adopted secrets: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly one adopted secret, got %d", count)
	}
}

func TestAdoptedSecretIsSystemOwnedAndUnrevealable(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	seedWebhook(t, svc, database, "wh-1", "Deploy notifier", "sig_abc123")
	if err := svc.AdoptLegacyColumns(ctx, LegacyCatalog()[0]); err != nil {
		t.Fatalf("adoption: %v", err)
	}
	secretID := webhookSecretID(t, database, "wh-1")

	sec, err := svc.ByID(ctx, secretID.String)
	if err != nil {
		t.Fatalf("loading adopted secret: %v", err)
	}
	if !sec.System {
		t.Error("an adopted credential must be marked system-owned")
	}
	if sec.OriginModule == nil || *sec.OriginModule != "webhooks" {
		t.Errorf("origin_module = %v, want webhooks", sec.OriginModule)
	}

	// The owning module reads it; the reveal endpoint does not.
	if _, err := svc.Reveal(ctx, secretID.String); err == nil {
		t.Error("a system secret must not be revealable through the ordinary flow")
	}
	value, err := svc.SystemValue(ctx, secretID.String)
	if err != nil {
		t.Fatalf("the owning module must be able to read it back: %v", err)
	}
	if value != "sig_abc123" {
		t.Errorf("round-tripped to %q", value)
	}
}

// TestReadThroughFallsBackToTheLegacyColumn is the property that makes a
// half-migrated database work, and the reason the legacy columns are not
// dropped in the same release as the backfill.
func TestReadThroughFallsBackToTheLegacyColumn(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	legacy, err := svc.enc.Encrypt("legacy_value")
	if err != nil {
		t.Fatalf("encrypting: %v", err)
	}

	t.Run("no pointer reads the legacy column", func(t *testing.T) {
		got, err := svc.ReadThrough(ctx, sql.NullString{}, legacy)
		if err != nil {
			t.Fatalf("read-through: %v", err)
		}
		if got != "legacy_value" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("a dangling pointer still reads the legacy column", func(t *testing.T) {
		// A secret deleted out from under a module must not take that
		// module's feature down while its own column still holds a good value.
		got, err := svc.ReadThrough(ctx, sql.NullString{String: "gone", Valid: true}, legacy)
		if err != nil {
			t.Fatalf("read-through: %v", err)
		}
		if got != "legacy_value" {
			t.Errorf("got %q, want the legacy fallback", got)
		}
	})

	t.Run("an empty legacy column with no pointer is not an error", func(t *testing.T) {
		got, err := svc.ReadThrough(ctx, sql.NullString{}, "")
		if err != nil {
			t.Fatalf("read-through: %v", err)
		}
		if got != "" {
			t.Errorf("got %q", got)
		}
	})
}

// TestPruneRemovesOrphansOnly covers the write rule: a module that rotates its
// credential writes the legacy column and clears the pointer, and the next
// boot's sweep collects the copy left behind — without touching a user's own
// secret or a still-linked one.
func TestPruneRemovesOrphansOnly(t *testing.T) {
	svc, database := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()
	catalog := []LegacyColumn{LegacyCatalog()[0]}

	seedWebhook(t, svc, database, "wh-1", "Kept", "sig_kept")
	seedWebhook(t, svc, database, "wh-2", "Rotated", "sig_old")
	if err := svc.AdoptLegacyColumns(ctx, catalog...); err != nil {
		t.Fatalf("adoption: %v", err)
	}

	userSecret, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "MY_OWN", Scope: ScopeGlobal, Value: "mine",
	})
	if err != nil {
		t.Fatalf("creating a user secret: %v", err)
	}

	// Simulate the module rotating wh-2: new value in its own column, pointer
	// cleared.
	if _, err := database.Exec(
		"UPDATE webhooks SET secret = 'enc:v1:whatever', secret_secret_id = NULL WHERE id = 'wh-2'",
	); err != nil {
		t.Fatalf("simulating rotation: %v", err)
	}

	pruned, err := svc.PruneOrphanedSystemSecrets(ctx, catalog)
	if err != nil {
		t.Fatalf("pruning: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 orphan pruned, got %d", pruned)
	}

	if _, err := svc.ByID(ctx, userSecret.ID); err != nil {
		t.Errorf("a user's own secret must never be pruned: %v", err)
	}
	kept := webhookSecretID(t, database, "wh-1")
	if _, err := svc.ByID(ctx, kept.String); err != nil {
		t.Errorf("a still-linked adopted secret must not be pruned: %v", err)
	}
}

func TestSystemSecretName(t *testing.T) {
	cases := []struct{ prefix, label, id, want string }{
		{"WEBHOOK", "Deploy notifier", "d1f2", "WEBHOOK_DEPLOY_NOTIFIER"},
		{"REGISTRY", "ghcr.io / team", "d1f2", "REGISTRY_GHCR_IO_TEAM"},
		{"ENV_CA", "", "d1f2g3", "ENV_CA_D1F2G3"},
		{"COMM", "   ", "d1f2g3", "COMM_D1F2G3"},
	}
	for _, tc := range cases {
		got := systemSecretName(tc.prefix, tc.label, tc.id)
		if got != tc.want {
			t.Errorf("systemSecretName(%q, %q) = %q, want %q", tc.prefix, tc.label, got, tc.want)
		}
		if err := ValidateName(got); err != nil {
			t.Errorf("%q is not a valid secret name: %v", got, err)
		}
	}

	// A label long enough to overflow still produces something valid.
	long := systemSecretName("STORAGE_CLIENTSECRET", "a-very-long-storage-location-name-that-keeps-going-and-going", "d1f2")
	if err := ValidateName(long); err != nil {
		t.Errorf("a long label produced an invalid name %q: %v", long, err)
	}
}
