// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Request Tracker comm plugin: a notification adapter whose client also
// implements the incident lifecycle, so the host opens, comments on and resolves RT tickets.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// channelType is the communication_channels.channel_type value this plugin owns.
const channelType = "request_tracker"

// RequestTrackerPlugin is the singleton the host dispatches to.
type RequestTrackerPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the identity the loader checks against plugin.yaml.
func (p *RequestTrackerPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is empty: the host owns the channel CRUD and the inbound webhook route.
func (p *RequestTrackerPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is empty: channels are managed under Settings → Communication.
func (p *RequestTrackerPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is empty: an ITSM plugin contributes no dashboard widgets.
func (p *RequestTrackerPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is empty: the host's itsm-* workflow nodes drive every ITSM plugin.
func (p *RequestTrackerPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is empty because the host registers adds_permissions from the manifest.
func (p *RequestTrackerPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the client; the host discovers the incident interfaces by type
// assertion, so nothing here says "ITSM" beyond the client's method set.
func (p *RequestTrackerPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: channelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Request Tracker",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials: true,
			},
			ConfigSchema: rtConfigSchema(),
		},
		NewClient(nil),
	)
}

// Init records the plugin start in the audit log and fails the load when that is impossible.
func (p *RequestTrackerPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.comm-request-tracker.init", map[string]any{"slug": p.manifest.Slug}); err != nil {
		return fmt.Errorf("comm-request-tracker: audit init: %w", err)
	}
	return nil
}

// Shutdown records the plugin stop in the audit log.
func (p *RequestTrackerPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.comm-request-tracker.shutdown", nil); err != nil {
		return fmt.Errorf("comm-request-tracker: audit shutdown: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*RequestTrackerPlugin)(nil)

// Plugin is the symbol plugin.Open looks up; Version must equal plugin.yaml's version.
var Plugin plugin.Plugin = &RequestTrackerPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-request-tracker",
		Name:    "Request Tracker",
		Version: "2026.10.01",
	},
}
