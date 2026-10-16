// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
)

func claimPigeon(action string) (string, bool) {
	if action == "send-pigeon" {
		return "pigeon", true
	}
	return "", false
}

func seedChannels(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`INSERT INTO communication_channels (id, name, channel_type, method, is_default, enabled, created_at, updated_at) VALUES
		('chat-default', 'Chat', 'chat', '', 1, 1, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z'),
		('mail-1', 'Mail', 'pigeon', '', 0, 1, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z')`); err != nil {
		t.Fatalf("seed channels: %v", err)
	}
}

func channelTestService(t *testing.T) (*Service, *bytes.Buffer) {
	t.Helper()
	database := openAliasTestDB(t)
	seedChannels(t, database)
	logs := &bytes.Buffer{}
	return &Service{db: database, logger: slog.New(slog.NewTextHandler(logs, nil)),
		notifier: corenotify.NewDispatcher(database, nil)}, logs
}

// TestAStoredPerProviderSendRunsOnTheClaimingPluginsChannel: a canvas saved while the
// host shipped send-<provider> nodes runs as the generic send node on the channel type
// the loaded comm plugin claims; unclaimed, it is an unknown action and passes through.
func TestAStoredPerProviderSendRunsOnTheClaimingPluginsChannel(t *testing.T) {
	s, logs := channelTestService(t)
	s.channelAlias = claimPigeon
	node := &CanvasNode{ID: "n1", Action: "send-pigeon", Config: map[string]interface{}{"message": "disk full"}}

	port, out, err := s.ExecuteNode(context.Background(), node, NewMsg(nil), &FlowContext{}, "")
	if err != nil {
		t.Fatalf("want the send node's own error port, got %v", err)
	}
	payload, _ := out["payload"].(map[string]interface{})
	if port != "error" || payload["channelType"] != "pigeon" {
		t.Fatalf("want a send attempt on the pigeon channel, got %q %+v", port, payload)
	}
	// No adapter is registered in this test, so the attempt stops at the dispatcher; what
	// matters is that it picked the pigeon channel, not the estate's chat default.
	if !strings.Contains(logs.String(), "channel type pigeon") {
		t.Fatalf("want the pigeon channel resolved, logs:\n%s", logs.String())
	}

	s.channelAlias = func(string) (string, bool) { return "", false }
	logs.Reset()
	unclaimed := &CanvasNode{ID: "n2", Action: "send-pigeon", Config: map[string]interface{}{"message": "disk full"}}
	port, _, err = s.ExecuteNode(context.Background(), unclaimed, NewMsg(nil), &FlowContext{}, "")
	if err != nil || port != "output" || strings.Contains(logs.String(), "communication send") {
		t.Fatalf("an unclaimed send must degrade to a pass-through, got %q %v\n%s", port, err, logs.String())
	}
}

// TestSendEmailWithoutToUsesTheChannelsRecipients: in configured-server mode an empty "To"
// sends to the recipients saved on the channel, while custom SMTP has no channel to fall
// back on and still refuses it.
func TestSendEmailWithoutToUsesTheChannelsRecipients(t *testing.T) {
	s, logs := channelTestService(t)
	configured := &CanvasNode{ID: "e1", Action: "send-email", Config: map[string]interface{}{
		"delivery_mode": "configured", "server_id": "mail-1", "body": "hello"}}
	port, _, err := s.ExecuteNode(context.Background(), configured, NewMsg(nil), &FlowContext{}, "")
	if err != nil || port != "error" || !strings.Contains(logs.String(), "channel type pigeon") {
		t.Fatalf("an empty To must reach the channel, not be refused, got %q %v\n%s", port, err, logs.String())
	}
	if strings.Contains(logs.String(), "recipient override") {
		t.Fatalf("an empty To must not be sent as an override, logs:\n%s", logs.String())
	}

	custom := &CanvasNode{ID: "e2", Action: "send-email", Config: map[string]interface{}{
		"delivery_mode": "custom", "smtp_host": "mail.example.com", "from_address": "a@example.com", "body": "hello"}}
	if _, _, err := s.ExecuteNode(context.Background(), custom, NewMsg(nil), &FlowContext{}, ""); err == nil ||
		!strings.Contains(err.Error(), "recipient is required") {
		t.Fatalf("custom SMTP must still require To, got %v", err)
	}
}

// TestSendEmailUsesOnlyChannelsAPluginDeclaresEmail: with no server picked, the default
// email channel is one whose plugin declares email; with no such plugin the node fails
// saying so, and a stored server id keeps resolving exactly as it did.
func TestSendEmailUsesOnlyChannelsAPluginDeclaresEmail(t *testing.T) {
	s, logs := channelTestService(t)
	node := func(serverID string) *CanvasNode {
		return &CanvasNode{ID: "e1", Action: "send-email", Config: map[string]interface{}{
			"delivery_mode": "configured", "server_id": serverID, "to": []interface{}{"ops@example.com"}, "body": "hello"}}
	}

	s.emailChannels = func() []string { return nil }
	if _, _, err := s.ExecuteNode(context.Background(), node(""), NewMsg(nil), &FlowContext{}, ""); err == nil ||
		!strings.Contains(err.Error(), "no installed communication plugin can send email") {
		t.Fatalf("want the missing email plugin named, got %v", err)
	}

	s.emailChannels = func() []string { return []string{"pigeon"} }
	if port, _, err := s.ExecuteNode(context.Background(), node(""), NewMsg(nil), &FlowContext{}, ""); err != nil || port != "error" {
		t.Fatalf("want a send attempt, got %q %v", port, err)
	}
	if !strings.Contains(logs.String(), "channel type pigeon") {
		t.Fatalf("want the declared email channel, never the chat default, logs:\n%s", logs.String())
	}

	s.emailChannels = func() []string { return nil }
	logs.Reset()
	if port, _, err := s.ExecuteNode(context.Background(), node("mail-1"), NewMsg(nil), &FlowContext{}, ""); err != nil || port != "error" ||
		!strings.Contains(logs.String(), "channel type pigeon") {
		t.Fatalf("a stored server id must keep resolving by id, got %q %v\n%s", port, err, logs.String())
	}
}
