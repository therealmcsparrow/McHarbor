// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	mistralProviderType = "mistral"
	mistralPluginSlug   = "ai-provider-mistral"
	mistralPluginName   = "Mistral"
	mistralVersion      = "2026.10.01"
	mistralDescription  = "Routes McHarbor AI chat completions through Mistral's OpenAI-compatible /v1/chat/completions endpoint. Models include Mistral Large, Mistral Medium, and Mixtral open-weights line."
)

type mistralPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *mistralPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *mistralPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *mistralPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *mistralPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *mistralPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *mistralPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *mistralPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: mistralProviderType,
		Slug:         mistralPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Mistral AI",
		Description:  mistralDescription,
		DefaultModel: defaultMistralModel,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: mistralConfigSchema(),
	}, newMistralClient())
}

func mistralConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Mistral API key from https://console.mistral.ai/api-keys. Stored encrypted at rest.",
			Placeholder: "Mistral API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultMistralBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultMistralBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultMistralModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultMistralModel,
		},
	}
}

func (p *mistralPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		if err := host.Audit.Emit("plugin.ai-provider-mistral.init", map[string]any{"slug": mistralPluginSlug}); err != nil && host.Logger != nil {
			host.Logger.Warn("failed to emit Mistral plugin init audit event", "error", err)
		}
	}
	return nil
}

func (p *mistralPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		if err := p.host.Audit.Emit("plugin.ai-provider-mistral.shutdown", nil); err != nil && p.host.Logger != nil {
			p.host.Logger.Warn("failed to emit Mistral plugin shutdown audit event", "error", err)
		}
	}
	return nil
}

var _ plugin.Plugin = (*mistralPlugin)(nil)
var _ plugin.AdapterContributor = (*mistralPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &mistralPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     mistralPluginSlug,
		Name:     mistralPluginName,
		Version:  mistralVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     mistralDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
