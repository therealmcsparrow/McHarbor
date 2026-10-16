// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the PagerDuty incident channel: a notification adapter whose client also
// implements the incident lifecycle, writing through Events API v2 and reading back via v3 webhooks.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// channelType is the communication_channels.channel_type value this plugin owns.
const channelType = "pagerduty"

// PagerDutyPlugin is the singleton the host dispatches to.
type PagerDutyPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the identity the loader checks against plugin.yaml.
func (p *PagerDutyPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is empty: the host owns the channel CRUD and the inbound webhook route.
func (p *PagerDutyPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is empty: channels are managed under Settings → Communication.
func (p *PagerDutyPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is empty: an incident channel contributes no dashboard widgets.
func (p *PagerDutyPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is empty: the host's itsm-* workflow nodes drive every incident channel.
func (p *PagerDutyPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is empty because the host registers adds_permissions from the manifest.
func (p *PagerDutyPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the client; the host discovers the incident interfaces by type
// assertion, so the lifecycle, webhook and poll support ride on the client's method set.
func (p *PagerDutyPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: channelType,
			Slug:        p.manifest.Slug,
			DisplayName: "PagerDuty",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials: true,
			},
			ConfigSchema: pagerdutyConfigSchema(),
		},
		NewClient(),
	)
}

// Init records the plugin start in the audit log and fails the load when that is impossible.
func (p *PagerDutyPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-pagerduty.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-pagerduty: audit init: %w", err)
	}
	return nil
}

// Shutdown records the plugin stop in the audit log.
func (p *PagerDutyPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-pagerduty.shutdown", nil); err != nil {
		return fmt.Errorf("comm-pagerduty: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*PagerDutyPlugin)(nil)

// Plugin is the symbol plugin.Open looks up; Version must equal plugin.yaml's version.
var Plugin plugin.Plugin = &PagerDutyPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-pagerduty",
		Name:    "PagerDuty",
		Version: "2026.10.01",
	},
}
