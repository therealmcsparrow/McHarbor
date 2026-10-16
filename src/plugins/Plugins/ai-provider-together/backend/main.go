// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	togetherProviderType = "together"
	togetherPluginSlug   = "ai-provider-together"
	togetherPluginName   = "Together AI"
	togetherVersion      = "2026.10.01"
	togetherDescription  = "Routes McHarbor AI chat completions through Together AI's OpenAI-compatible endpoint, which serves a large catalog of hosted open-weights models."
)

type togetherPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *togetherPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *togetherPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *togetherPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *togetherPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *togetherPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *togetherPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *togetherPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: togetherProviderType,
		Slug:         togetherPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Together AI",
		Description:  togetherDescription,
		DefaultModel: defaultTogetherModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: togetherConfigSchema(),
	}, newTogetherClient())
	if err != nil {
		return fmt.Errorf("registering together adapter: %w", err)
	}
	return nil
}

func togetherConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Together AI API key from https://api.together.ai/settings/api-keys. Stored encrypted at rest.",
			Placeholder: "Together AI API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultTogetherBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultTogetherBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultTogetherModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultTogetherModel,
		},
	}
}

func (p *togetherPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-together.init", map[string]any{"slug": togetherPluginSlug})
	return nil
}

func (p *togetherPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-together.shutdown", nil)
	return nil
}

func (p *togetherPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Together AI plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*togetherPlugin)(nil)
var _ plugin.AdapterContributor = (*togetherPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &togetherPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     togetherPluginSlug,
		Name:     togetherPluginName,
		Version:  togetherVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     togetherDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
