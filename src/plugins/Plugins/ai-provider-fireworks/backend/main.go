// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	fireworksProviderType = "fireworks"
	fireworksPluginSlug   = "ai-provider-fireworks"
	fireworksPluginName   = "Fireworks AI"
	fireworksVersion      = "2026.10.01"
	fireworksDescription  = "Routes McHarbor AI chat completions through Fireworks AI's OpenAI-compatible inference API for hosted open-weights models."
)

type fireworksPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *fireworksPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *fireworksPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *fireworksPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *fireworksPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *fireworksPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *fireworksPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *fireworksPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: fireworksProviderType,
		Slug:         fireworksPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Fireworks AI",
		Description:  fireworksDescription,
		DefaultModel: defaultFireworksModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: fireworksConfigSchema(),
	}, newFireworksClient())
	if err != nil {
		return fmt.Errorf("registering fireworks adapter: %w", err)
	}
	return nil
}

func fireworksConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Fireworks AI API key from https://fireworks.ai/account/api-keys. Stored encrypted at rest.",
			Placeholder: "Fireworks AI API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultFireworksBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultFireworksBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultFireworksModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultFireworksModel,
		},
	}
}

func (p *fireworksPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-fireworks.init", map[string]any{"slug": fireworksPluginSlug})
	return nil
}

func (p *fireworksPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-fireworks.shutdown", nil)
	return nil
}

func (p *fireworksPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Fireworks AI plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*fireworksPlugin)(nil)
var _ plugin.AdapterContributor = (*fireworksPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &fireworksPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     fireworksPluginSlug,
		Name:     fireworksPluginName,
		Version:  fireworksVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     fireworksDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
