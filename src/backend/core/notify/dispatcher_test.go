// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	_ "modernc.org/sqlite"
)

func TestDispatcherCapabilitiesUsesCanonicalTables(t *testing.T) {
	db, enc := openDispatcherTestDB(t)

	insertCommunicationChannel(t, db, "channel-1", "Slack", "slack", true, true)
	// Not the default: idx_comm_channels_default_unique allows exactly one, and
	// Slack holds it. This row is here to prove a disabled channel contributes
	// no capability, which its default flag has nothing to do with.
	insertCommunicationChannel(t, db, "channel-2", "Teams", "teams", false, false)
	insertCommunicationChannel(t, db, "channel-3", "Telegram", "telegram", false, true)

	dispatcher := NewDispatcher(db, enc)

	capabilities, err := dispatcher.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities returned error: %v", err)
	}

	// "internal" is synthesised; every other capability is a channel row. Email
	// used to be synthesised too, from a since-retired email_servers table, and
	// is now an ordinary channel_type like any other.
	expected := []string{"internal", "slack", "telegram"}
	if len(capabilities) != len(expected) {
		t.Fatalf("expected %d capabilities, got %d: %#v", len(expected), len(capabilities), capabilities)
	}
	for i, capability := range expected {
		if capabilities[i] != capability {
			t.Fatalf("expected capability[%d] = %q, got %q", i, capability, capabilities[i])
		}
	}
}

func TestResolveChannelTargetSupportsTypeOverrideAndDefaultFallback(t *testing.T) {
	db, enc := openDispatcherTestDB(t)

	insertCommunicationChannel(t, db, "channel-default", "Telegram", "telegram", true, true)
	insertCommunicationChannelWithConfig(t, db, enc, "channel-slack", "Slack", "slack", false, true,
		map[string]string{"webhook_url": "https://hooks.slack.test"})

	dispatcher := NewDispatcher(db, enc)

	defaultTarget, err := dispatcher.resolveChannelTarget(context.Background(), "", "")
	if err != nil {
		t.Fatalf("resolveChannelTarget default returned error: %v", err)
	}
	if defaultTarget.ID != "channel-default" {
		t.Fatalf("expected default communication channel, got %q", defaultTarget.ID)
	}

	typeTarget, err := dispatcher.resolveChannelTarget(context.Background(), "", "slack")
	if err != nil {
		t.Fatalf("resolveChannelTarget by type returned error: %v", err)
	}
	if typeTarget.ID != "channel-slack" {
		t.Fatalf("expected slack communication channel, got %q", typeTarget.ID)
	}
	if typeTarget.WebhookURL != "https://hooks.slack.test" {
		t.Fatalf("expected decrypted webhook url, got %q", typeTarget.WebhookURL)
	}

	canonicalTarget, err := dispatcher.resolveChannelTarget(context.Background(), "", " Slack ")
	if err != nil {
		t.Fatalf("resolveChannelTarget normalized type returned error: %v", err)
	}
	if canonicalTarget.ID != "channel-slack" {
		t.Fatalf("expected normalized slack communication channel, got %q", canonicalTarget.ID)
	}
}

func TestSendChannelCreatesInternalNotification(t *testing.T) {
	db, enc := openDispatcherTestDB(t)

	dispatcher := NewDispatcher(db, enc)

	delivery, err := dispatcher.SendChannel(context.Background(), ChannelRequest{
		ChannelType: "internal",
		Title:       "Workflow notice",
		Message:     "The workflow finished successfully",
	})
	if err != nil {
		t.Fatalf("SendChannel internal returned error: %v", err)
	}
	if delivery.TargetType != "internal" {
		t.Fatalf("expected internal target type, got %q", delivery.TargetType)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM in_app_notifications`).Scan(&count); err != nil {
		t.Fatalf("counting in-app notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 in-app notification, got %d", count)
	}

	aliasDelivery, err := dispatcher.SendChannel(context.Background(), ChannelRequest{
		ChannelType: "alert",
		Title:       "Workflow alert",
		Message:     "Alias path should also write in-app notifications",
	})
	if err != nil {
		t.Fatalf("SendChannel alert alias returned error: %v", err)
	}
	if aliasDelivery.TargetType != "internal" {
		t.Fatalf("expected alert alias to resolve to internal target type, got %q", aliasDelivery.TargetType)
	}
}

// openDispatcherTestDB opens a database and runs the real migrations against it.
//
// The schema is deliberately not written out here. An earlier hand-written copy
// fell two migrations behind the application's own — 072 replaced the per-channel
// columns with a single config_enc blob, 115 added config_secret_id — and the
// tests failed against a table shape that had not existed for months.
func openDispatcherTestDB(t *testing.T) (*sql.DB, *encryption.Service) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "dispatcher-test.db")
	db, err := coredb.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite returned error: %v", err)
	}
	if err := coredb.Migrate(db, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})

	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("encryption.New returned error: %v", err)
	}

	return db, enc
}

func insertCommunicationChannel(t *testing.T, db *sql.DB, id, name, channelType string, isDefault, enabled bool) {
	t.Helper()

	insertChannelRow(t, db, id, name, channelType, isDefault, enabled, nil)
}

// insertCommunicationChannelWithConfig writes a channel whose settings live in
// config_enc, encrypted exactly the way the communications module writes them —
// a JSON map, then AES-256-GCM. Passing the ciphertext of a bare value instead
// would exercise a shape the resolver never meets.
func insertCommunicationChannelWithConfig(
	t *testing.T, db *sql.DB, enc *encryption.Service,
	id, name, channelType string, isDefault, enabled bool, config map[string]string,
) {
	t.Helper()

	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshalling channel config: %v", err)
	}
	ciphertext, err := enc.Encrypt(string(raw))
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	insertChannelRow(t, db, id, name, channelType, isDefault, enabled, &ciphertext)
}

func insertChannelRow(
	t *testing.T, db *sql.DB,
	id, name, channelType string, isDefault, enabled bool, configEnc *string,
) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO communication_channels (
			id, name, channel_type, method, is_default, enabled, config_enc, created_at, updated_at
		) VALUES (?, ?, ?, '', ?, ?, ?, ?, ?)`,
		id, name, channelType, boolInt(isDefault), boolInt(enabled), configEnc, now, now,
	); err != nil {
		t.Fatalf("inserting communication channel: %v", err)
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
