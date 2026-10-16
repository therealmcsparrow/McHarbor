// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"context"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type schemaSource struct {
	spec plugin.NotificationAdapterSpec
}

func (s schemaSource) NotificationAdapter(string) (plugin.NotificationAdapterClient, plugin.NotificationAdapterSpec, bool) {
	return nil, s.spec, true
}

type nopClient struct{}

func (nopClient) Test(context.Context, map[string]string) error { return nil }
func (nopClient) Send(context.Context, map[string]string, plugin.NotificationMessage) error {
	return nil
}

// TestRedactionHonoursSchemaSecrets guards plugins whose credential field is named
// neither password, secret nor token: the schema marks it, and that must suffice.
func TestRedactionHonoursSchemaSecrets(t *testing.T) {
	s := &Service{pluginRuntime: schemaSource{spec: plugin.NotificationAdapterSpec{ConfigSchema: []plugin.ConfigFieldDecl{
		{Key: "routing_key", Type: "secret"},
		{Key: "receiver_url", Type: "string", Secret: true},
		{Key: "region", Type: "string"},
	}}}}
	cfg := map[string]string{"routing_key": "R0UT1NG", "receiver_url": "https://x/tok", "region": "eu", "api_token": "t"}
	var ch CommunicationChannel
	ch.ChannelType = "pagerduty"
	applyChannelConfig(&ch, cfg, false, s.schemaSecretKeys(ch.ChannelType))
	if len(ch.Config) != 1 || ch.Config["region"] != "eu" {
		t.Fatalf("redacted config = %v, want only region", ch.Config)
	}
	applyChannelConfig(&ch, cfg, true, s.schemaSecretKeys(ch.ChannelType))
	if ch.Config["routing_key"] != "R0UT1NG" {
		t.Fatal("includeSecrets must still return the full config")
	}
}

// TestEmptyLegacyFieldKeepsPluginConfig guards plugin forms that use a legacy key name
// inside config: the empty top-level field must not delete it.
func TestEmptyLegacyFieldKeepsPluginConfig(t *testing.T) {
	cfg := channelConfigFromInput(CreateChannelInput{Config: map[string]string{"webhook_url": "https://r/x", "topic": "t"}})
	if cfg["webhook_url"] != "https://r/x" || cfg["topic"] != "t" {
		t.Fatalf("config = %v", cfg)
	}
	if cfg := channelConfigFromInput(CreateChannelInput{WebhookURL: "https://legacy"}); cfg["webhook_url"] != "https://legacy" {
		t.Fatalf("legacy field ignored: %v", cfg)
	}
}

var _ plugin.NotificationAdapterClient = nopClient{}
