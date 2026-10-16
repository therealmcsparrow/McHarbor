// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// fakeSnapshotClient records the config map the driver is handed.
type fakeSnapshotClient struct{ last map[string]string }

func (c *fakeSnapshotClient) Capability(_ context.Context, cfg map[string]string) (plugin.SnapshotCapability, error) {
	c.last = cfg
	return plugin.SnapshotCapability{Available: true}, nil
}

func (c *fakeSnapshotClient) Create(_ context.Context, cfg map[string]string, _ plugin.SnapshotCreateRequest) (plugin.SnapshotRef, error) {
	c.last = cfg
	return plugin.SnapshotRef{}, nil
}

func (c *fakeSnapshotClient) Remove(_ context.Context, cfg map[string]string, _ plugin.SnapshotRef) error {
	c.last = cfg
	return nil
}

// fakeSnapshotRuntime answers for one plugin driver whose schema declares a secret.
type fakeSnapshotRuntime struct{ client *fakeSnapshotClient }

func (r fakeSnapshotRuntime) SnapshotAdapter(driver string) (plugin.SnapshotAdapterClient, plugin.SnapshotAdapterSpec, bool) {
	if driver != "nas" {
		return nil, plugin.SnapshotAdapterSpec{}, false
	}
	return r.client, plugin.SnapshotAdapterSpec{Driver: "nas", ConfigSchema: []types.ConfigFieldDecl{
		{Key: "host", Type: "string"},
		{Key: "api_key", Type: "secret"},
	}}, true
}

func newSnapshotSecretFixture(t *testing.T) (*Service, *sql.DB, *encryption.Service, *fakeSnapshotClient) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "snap.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeSnapshotClient{}
	svc := NewService(database, nil, t.TempDir(), nil, enc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetPluginStorageAdapter(fakeSnapshotRuntime{client: client})
	return svc, database, enc, client
}

func storedSnapshotConfig(t *testing.T, database *sql.DB, id string) map[string]string {
	t.Helper()
	var raw string
	if err := database.QueryRow(`SELECT config FROM container_backup_snapshot_configs WHERE id = ?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func probeAPIKey(t *testing.T, svc *Service, client *fakeSnapshotClient, id string) string {
	t.Helper()
	if _, err := svc.ProbeSnapshotConfig(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	return client.last["api_key"]
}

// TestSnapshotConfigSecretsAreEncryptedAndWithheld guards a NAS API key on a snapshot
// config: ciphertext at rest, never in a list or write response, kept across a save that
// leaves it blank, and plaintext only on the way into the driver.
func TestSnapshotConfigSecretsAreEncryptedAndWithheld(t *testing.T) {
	svc, database, enc, client := newSnapshotSecretFixture(t)
	ctx := t.Context()

	created, err := svc.CreateSnapshotConfig(ctx, CreateSnapshotConfigInput{
		EnvironmentID: "envA", Name: "nas", Driver: "nas", PluginSlug: "snapshot-nas",
		Config: map[string]string{"host": "nas.local", "api_key": "KEY-ONE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := created.Config["api_key"]; leaked || len(created.SecretsSet) != 1 || created.SecretsSet[0] != "api_key" {
		t.Fatalf("create response = config %v secretsSet %v, want api_key withheld and listed", created.Config, created.SecretsSet)
	}
	if created.Config["host"] != "nas.local" {
		t.Fatalf("an ordinary field was withheld: %v", created.Config)
	}

	stored := storedSnapshotConfig(t, database, created.ID)
	if !encryption.IsEncrypted(stored["api_key"]) {
		t.Fatalf("api_key stored as %q, want ciphertext", stored["api_key"])
	}
	if plain, err := enc.Decrypt(stored["api_key"]); err != nil || plain != "KEY-ONE" {
		t.Fatalf("stored api_key decrypts to (%q, %v)", plain, err)
	}

	list, err := svc.ListSnapshotConfigs(ctx, "envA")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "KEY-ONE") || strings.Contains(string(body), "enc:v1:") {
		t.Fatalf("list response carries the secret: %s", body)
	}

	if got := probeAPIKey(t, svc, client, created.ID); got != "KEY-ONE" {
		t.Fatalf("driver received api_key %q, want the plaintext", got)
	}

	blank := map[string]string{"host": "nas2.local", "api_key": ""}
	updated, err := svc.UpdateSnapshotConfig(ctx, created.ID, UpdateSnapshotConfigInput{Config: &blank})
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := updated.Config["api_key"]; leaked {
		t.Fatalf("update response carries the secret: %v", updated.Config)
	}
	if got := probeAPIKey(t, svc, client, created.ID); got != "KEY-ONE" {
		t.Fatalf("a blank secret on save changed api_key to %q", got)
	}
	omitted := map[string]string{"host": "nas3.local"}
	if _, err := svc.UpdateSnapshotConfig(ctx, created.ID, UpdateSnapshotConfigInput{Config: &omitted}); err != nil {
		t.Fatal(err)
	}
	if got := probeAPIKey(t, svc, client, created.ID); got != "KEY-ONE" || client.last["host"] != "nas3.local" {
		t.Fatalf("an omitted secret on save gave api_key %q host %q", got, client.last["host"])
	}

	rotated := map[string]string{"host": "nas3.local", "api_key": "KEY-TWO"}
	if _, err := svc.UpdateSnapshotConfig(ctx, created.ID, UpdateSnapshotConfigInput{Config: &rotated}); err != nil {
		t.Fatal(err)
	}
	if !encryption.IsEncrypted(storedSnapshotConfig(t, database, created.ID)["api_key"]) {
		t.Fatal("a rotated secret was stored in plaintext")
	}
	if got := probeAPIKey(t, svc, client, created.ID); got != "KEY-TWO" {
		t.Fatalf("rotated api_key reached the driver as %q", got)
	}
}

// TestSnapshotConfigWriteRefusedWithoutDriver: with no driver to name the secret
// fields, a config map cannot be stored safely, so it is not stored at all.
func TestSnapshotConfigWriteRefusedWithoutDriver(t *testing.T) {
	svc, _, _, _ := newSnapshotSecretFixture(t)
	_, err := svc.CreateSnapshotConfig(t.Context(), CreateSnapshotConfigInput{
		EnvironmentID: "envA", Name: "gone", Driver: "uninstalled", PluginSlug: "snapshot-gone",
		Config: map[string]string{"password": "p"},
	})
	if !errors.Is(err, ErrSnapshotDriverUnavailable) {
		t.Fatalf("create for an unloaded driver = %v, want ErrSnapshotDriverUnavailable", err)
	}
}

// TestSnapshotSecretBackfill encrypts a secret stored before encryption existed, leaves
// ciphertext and ordinary fields alone, and is a no-op on the second run.
func TestSnapshotSecretBackfill(t *testing.T) {
	svc, database, enc, _ := newSnapshotSecretFixture(t)
	already, err := enc.Encrypt("KEY-SEALED")
	if err != nil {
		t.Fatal(err)
	}
	for id, cfg := range map[string]string{
		"legacy": `{"host":"nas.local","api_key":"KEY-LEGACY"}`,
		"sealed": `{"host":"nas.local","api_key":"` + already + `"}`,
		"empty":  `{"host":"nas.local","api_key":""}`,
	} {
		if _, err := database.Exec(`INSERT INTO container_backup_snapshot_configs
			(id, environment_id, name, driver, plugin_slug, enabled, use_lxd, config, created_at, updated_at)
			VALUES (?, 'envA', ?, 'nas', 'snapshot-nas', 1, 0, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, id, id, cfg); err != nil {
			t.Fatal(err)
		}
	}

	n, err := svc.BackfillSnapshotSecretEncryption(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("first backfill = (%d, %v), want exactly the legacy row", n, err)
	}
	legacy := storedSnapshotConfig(t, database, "legacy")
	if plain, err := enc.Decrypt(legacy["api_key"]); !encryption.IsEncrypted(legacy["api_key"]) || err != nil || plain != "KEY-LEGACY" {
		t.Fatalf("legacy api_key = %q (decrypts to %q, %v)", legacy["api_key"], plain, err)
	}
	if legacy["host"] != "nas.local" {
		t.Fatalf("an ordinary field changed: %v", legacy)
	}
	if got := storedSnapshotConfig(t, database, "sealed")["api_key"]; got != already {
		t.Fatal("an already-encrypted secret was re-encrypted")
	}
	if got := storedSnapshotConfig(t, database, "empty")["api_key"]; got != "" {
		t.Fatalf("an empty secret became %q", got)
	}

	again, err := svc.BackfillSnapshotSecretEncryption(t.Context())
	if err != nil || again != 0 {
		t.Fatalf("second backfill = (%d, %v), want a no-op", again, err)
	}
}
