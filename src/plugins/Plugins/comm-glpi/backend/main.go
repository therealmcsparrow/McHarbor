// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the GLPI comm plugin: a notification adapter whose client also implements the
// incident lifecycle, so McHarbor alerts open, follow up and solve GLPI tickets.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// channelType is the communication_channels.channel_type value this plugin owns.
const channelType = "glpi"

// GLPIPlugin is the singleton the host dispatches to.
type GLPIPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the identity the loader checks against plugin.yaml.
func (p *GLPIPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is empty: the host owns the channel CRUD and the inbound webhook route.
func (p *GLPIPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is empty: channels are managed under Settings → Communication.
func (p *GLPIPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is empty: an ITSM plugin contributes no dashboard widgets.
func (p *GLPIPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is empty: the host's itsm-* workflow nodes drive every ITSM plugin.
func (p *GLPIPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is empty because the host registers adds_permissions from the manifest.
func (p *GLPIPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the client; the host discovers the incident interfaces by type
// assertion, so nothing here says "ITSM" beyond the client's method set.
func (p *GLPIPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: channelType,
			Slug:        p.manifest.Slug,
			DisplayName: "GLPI",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials: true,
			},
			ConfigSchema: glpiConfigSchema(),
		},
		NewClient(nil),
	)
}

// Init records the plugin start in the audit log and fails the load when that is impossible.
func (p *GLPIPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-glpi.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-glpi: audit init: %w", err)
	}
	return nil
}

// Shutdown records the plugin stop in the audit log.
func (p *GLPIPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-glpi.shutdown", nil); err != nil {
		return fmt.Errorf("comm-glpi: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*GLPIPlugin)(nil)

// Plugin is the symbol plugin.Open looks up; Version must equal plugin.yaml's version.
var Plugin plugin.Plugin = &GLPIPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-glpi",
		Name:    "GLPI",
		Version: "2026.10.01",
	},
}
