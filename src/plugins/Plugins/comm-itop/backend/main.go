// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the iTop ITSM comm plugin: a notification adapter whose client also walks the
// iTop ticket lifecycle, so the host opens, logs and resolves Incidents or User Requests through it.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// channelType is the communication_channels.channel_type value this plugin owns.
const channelType = "itop"

// ITopPlugin is the singleton the host dispatches to.
type ITopPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the identity the loader checks against plugin.yaml.
func (p *ITopPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is empty: the host owns the channel CRUD and the inbound webhook route.
func (p *ITopPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is empty: channels are managed under Settings → Communication.
func (p *ITopPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is empty: an ITSM plugin contributes no dashboard widgets.
func (p *ITopPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is empty: the host's itsm-* workflow nodes drive every ITSM plugin.
func (p *ITopPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is empty because the host registers adds_permissions from the manifest.
func (p *ITopPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the client; the host discovers the incident interfaces by type
// assertion, so nothing here says "ITSM" beyond the client's method set.
func (p *ITopPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: channelType,
			Slug:        p.manifest.Slug,
			DisplayName: "iTop",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials: true,
			},
			ConfigSchema: itopConfigSchema(),
		},
		NewClient(nil),
	)
}

// Init records the plugin start in the audit log and fails the load when that is impossible.
func (p *ITopPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-itop.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-itop: audit init: %w", err)
	}
	return nil
}

// Shutdown records the plugin stop in the audit log.
func (p *ITopPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-itop.shutdown", nil); err != nil {
		return fmt.Errorf("comm-itop: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*ITopPlugin)(nil)

// Plugin is the symbol plugin.Open looks up; Version must equal plugin.yaml's version.
var Plugin plugin.Plugin = &ITopPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-itop",
		Name:    "iTop",
		Version: "2026.10.01",
	},
}
