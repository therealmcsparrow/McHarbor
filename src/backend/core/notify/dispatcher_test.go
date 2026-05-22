// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notify

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	_ "modernc.org/sqlite"
)

func TestDispatcherCapabilitiesUsesCanonicalTables(t *testing.T) {
	db, enc := openDispatcherTestDB(t)
	createDispatcherTables(t, db)

	insertCommunicationChannel(t, db, "channel-1", "Slack", "slack", true, true)
	insertCommunicationChannel(t, db, "channel-2", "Teams", "teams", true, false)
	insertCommunicationChannel(t, db, "channel-3", "Telegram", "telegram", false, true)
	insertEmailServer(t, db, "email-1", "SMTP", true, true, "", "")

	dispatcher := NewDispatcher(db, enc)

	capabilities, err := dispatcher.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities returned error: %v", err)
	}

	expected := []string{"email", "internal", "slack", "telegram"}
	if len(capabilities) != len(expected) {
		t.Fatalf("expected %d capabilities, got %d: %#v", len(expected), len(capabilities), capabilities)
	}
	for i, capability := range expected {
		if capabilities[i] != capability {
			t.Fatalf("expected capability[%d] = %q, got %q", i, capability, capabilities[i])
		}
	}
}

func TestResolveEmailTargetPrefersDefaultAndDecryptsSecrets(t *testing.T) {
	db, enc := openDispatcherTestDB(t)
	createDispatcherTables(t, db)

	encryptedPassword, err := enc.Encrypt("smtp-password")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	insertEmailServer(t, db, "email-1", "Default SMTP", true, true, encryptedPassword, "")
	insertEmailServer(t, db, "email-2", "Backup SMTP", false, true, "", "")

	dispatcher := NewDispatcher(db, enc)

	target, err := dispatcher.resolveEmailTarget(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveEmailTarget returned error: %v", err)
	}

	if target.ID != "email-1" {
		t.Fatalf("expected default email server to be selected, got %q", target.ID)
	}
	if target.Password != "smtp-password" {
		t.Fatalf("expected decrypted password, got %q", target.Password)
	}
}

func TestResolveChannelTargetSupportsTypeOverrideAndDefaultFallback(t *testing.T) {
	db, enc := openDispatcherTestDB(t)
	createDispatcherTables(t, db)

	encryptedWebhook, err := enc.Encrypt("https://hooks.slack.test")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	insertCommunicationChannel(t, db, "channel-default", "Telegram", "telegram", true, true)
	insertCommunicationChannelWithSecret(t, db, "channel-slack", "Slack", "slack", false, true, encryptedWebhook)

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
	createDispatcherTables(t, db)

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

func openDispatcherTestDB(t *testing.T) (*sql.DB, *encryption.Service) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "dispatcher-test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("encryption.New returned error: %v", err)
	}

	return db, enc
}

func createDispatcherTables(t *testing.T, db *sql.DB) {
	t.Helper()

	schema := `
CREATE TABLE email_servers (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	server_type TEXT NOT NULL,
	is_default INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	host TEXT,
	port INTEGER,
	encryption TEXT,
	auth_method TEXT,
	username TEXT,
	password TEXT,
	client_id TEXT,
	client_secret TEXT,
	tenant_id TEXT,
	from_address TEXT NOT NULL,
	from_name TEXT
);
CREATE TABLE communication_channels (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	channel_type TEXT NOT NULL,
	method TEXT NOT NULL DEFAULT '',
	is_default INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	webhook_url TEXT,
	server_url TEXT,
	token TEXT,
	topic TEXT,
	chat_id TEXT,
	phone_number_id TEXT,
	recipient_phone TEXT,
	sender_number TEXT,
	recipients TEXT,
	username TEXT,
	password TEXT,
	priority TEXT
);
CREATE TABLE in_app_notifications (
	id TEXT PRIMARY KEY,
	level TEXT NOT NULL DEFAULT 'info',
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	action TEXT,
	entity_type TEXT,
	entity_id TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating schema: %v", err)
	}
}

func insertEmailServer(t *testing.T, db *sql.DB, id, name string, isDefault, enabled bool, password, clientSecret string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO email_servers (
			id, name, server_type, is_default, enabled, host, port, encryption, auth_method,
			username, password, client_id, client_secret, tenant_id, from_address, from_name
		) VALUES (?, ?, 'smtp', ?, ?, 'smtp.example.com', 587, 'starttls', 'plain', 'mailer', ?, 'client-id', ?, 'tenant-id', 'noreply@example.com', 'McHarbor')`,
		id, name, boolInt(isDefault), boolInt(enabled), password, clientSecret,
	); err != nil {
		t.Fatalf("inserting email server: %v", err)
	}
}

func insertCommunicationChannel(t *testing.T, db *sql.DB, id, name, channelType string, isDefault, enabled bool) {
	t.Helper()

	insertCommunicationChannelWithSecret(t, db, id, name, channelType, isDefault, enabled, "")
}

func insertCommunicationChannelWithSecret(t *testing.T, db *sql.DB, id, name, channelType string, isDefault, enabled bool, webhookURL string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO communication_channels (
			id, name, channel_type, method, is_default, enabled, webhook_url, server_url, token, topic,
			chat_id, phone_number_id, recipient_phone, sender_number, recipients, username, password, priority
		) VALUES (?, ?, ?, '', ?, ?, ?, '', '', '', '', '', '', '', '', '', '', '')`,
		id, name, channelType, boolInt(isDefault), boolInt(enabled), webhookURL,
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
