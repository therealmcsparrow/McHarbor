// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Zammad ITSM comm plugin: a notification adapter whose client also opens,
// notes and closes Zammad tickets through the host's incident lifecycle.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// channelType is the communication_channels.channel_type value this plugin owns.
const channelType = "zammad"

// ZammadPlugin is the singleton the host dispatches to.
type ZammadPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the identity the loader checks against plugin.yaml.
func (p *ZammadPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is empty: the host owns the channel CRUD and the inbound webhook route.
func (p *ZammadPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is empty: channels are managed under Settings → Communication.
func (p *ZammadPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is empty: an ITSM plugin contributes no dashboard widgets.
func (p *ZammadPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is empty: the host's itsm-* workflow nodes drive every ITSM plugin.
func (p *ZammadPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is empty because the host registers adds_permissions from the manifest.
func (p *ZammadPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the client; the host discovers the incident interfaces by type
// assertion on it, so the lifecycle needs no registration of its own.
func (p *ZammadPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: channelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Zammad",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials: true,
			},
			ConfigSchema: zammadConfigSchema(),
		},
		NewClient(nil),
	)
}

// Init records the plugin start in the audit log and fails the load when that is impossible.
func (p *ZammadPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-zammad.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-zammad: audit init: %w", err)
	}
	return nil
}

// Shutdown records the plugin stop in the audit log.
func (p *ZammadPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-zammad.shutdown", nil); err != nil {
		return fmt.Errorf("comm-zammad: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*ZammadPlugin)(nil)

// Plugin is the symbol plugin.Open looks up; Version must equal plugin.yaml's version.
var Plugin plugin.Plugin = &ZammadPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-zammad",
		Name:    "Zammad",
		Version: "2026.10.01",
	},
}
