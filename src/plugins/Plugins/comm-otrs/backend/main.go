// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the comm-otrs plugin: one channel type for OTRS, Znuny and OTOBO, because all
// three still expose the same GenericInterface ticket operations.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// otrsChannelType is the communication_channels.channel_type value this plugin owns.
const otrsChannelType = "otrs"

// CommOTRSPlugin is the singleton the host loads via plugin.Open.
type CommOTRSPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the manifest the host validates against plugin.yaml.
func (p *CommOTRSPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; inbound status arrives on the host-owned incident webhook route.
func (p *CommOTRSPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; the plugin surfaces as a channel type, not a page.
func (p *CommOTRSPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; the plugin contributes no dashboard widgets.
func (p *CommOTRSPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; the host's incident workflow nodes cover workflow use.
func (p *CommOTRSPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by adds_permissions in plugin.yaml.
func (p *CommOTRSPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the otrs channel type; the client also implements the optional
// incident interfaces, which the host discovers by type assertion.
func (p *CommOTRSPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType:  otrsChannelType,
			Slug:         p.manifest.Slug,
			DisplayName:  "OTRS / Znuny / OTOBO",
			Icon:         "plugin.svg",
			Description:  p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{EncryptedCredentials: true},
			ConfigSchema: otrsConfigSchema(),
		},
		NewClient(),
	)
}

// Init keeps the host context so lifecycle events are audited.
func (p *CommOTRSPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-otrs.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-otrs: audit init: %w", err)
	}
	return nil
}

// Shutdown audits the unload; a failed audit is returned so the host logs it.
func (p *CommOTRSPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-otrs.shutdown", nil); err != nil {
		return fmt.Errorf("comm-otrs: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*CommOTRSPlugin)(nil)

// Plugin is the symbol the McHarbor plugin runtime loads; Version must equal plugin.yaml's.
var Plugin plugin.Plugin = &CommOTRSPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-otrs",
		Name:    "OTRS / Znuny / OTOBO",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "OTRS-family ITSM channel. Opens, updates and closes tickets in OTRS, Znuny or OTOBO through a GenericInterface REST web service and reads their state back.",
	},
}
