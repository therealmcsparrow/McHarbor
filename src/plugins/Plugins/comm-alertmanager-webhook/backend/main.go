// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is a one-way on-call channel that speaks the Prometheus Alertmanager webhook v4
// format, so any receiver that ingests Alertmanager opens and resolves one incident per condition.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// channelType is the communication_channels.channel_type value this plugin owns.
const channelType = "alertmanager_webhook"

// AlertmanagerWebhookPlugin is the singleton the host dispatches to.
type AlertmanagerWebhookPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the identity the loader checks against plugin.yaml.
func (p *AlertmanagerWebhookPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is empty: the protocol is one-way, so there is no inbound route to own.
func (p *AlertmanagerWebhookPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is empty: channels are managed under Settings → Communication.
func (p *AlertmanagerWebhookPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is empty: a communication channel contributes no dashboard widgets.
func (p *AlertmanagerWebhookPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is empty: the host's incident workflow nodes drive every incident-capable channel.
func (p *AlertmanagerWebhookPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is empty because the host registers adds_permissions from the manifest.
func (p *AlertmanagerWebhookPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the client; the host finds incident.LifecycleClient on it by type
// assertion, and finds neither WebhookParser nor Poller because the protocol has no return path.
func (p *AlertmanagerWebhookPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: channelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Alertmanager webhook",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials: true,
			},
			ConfigSchema: amConfigSchema(),
		},
		NewClient(),
	)
}

// Init records the plugin start in the audit log and fails the load when that is impossible.
func (p *AlertmanagerWebhookPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-alertmanager-webhook.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-alertmanager-webhook: audit init: %w", err)
	}
	return nil
}

// Shutdown records the plugin stop in the audit log.
func (p *AlertmanagerWebhookPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-alertmanager-webhook.shutdown", nil); err != nil {
		return fmt.Errorf("comm-alertmanager-webhook: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*AlertmanagerWebhookPlugin)(nil)

// Plugin is the symbol plugin.Open looks up; Version must equal plugin.yaml's version.
var Plugin plugin.Plugin = &AlertmanagerWebhookPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-alertmanager-webhook",
		Name:    "Alertmanager webhook",
		Version: "2026.10.01",
	},
}
