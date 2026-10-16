// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	geminiProviderType = "gemini"
	geminiPluginSlug   = "ai-provider-gemini"
	geminiPluginName   = "Google Gemini"
	geminiVersion      = "2026.10.01"
	geminiDescription  = "Routes McHarbor AI chat completions through Google Gemini's OpenAI-compatible endpoint. Models include Gemini 2.5 Pro, Gemini 2.5 Flash, and Gemini 2.0 Flash."
)

type geminiPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *geminiPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *geminiPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *geminiPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *geminiPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *geminiPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *geminiPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *geminiPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: geminiProviderType,
		Slug:         geminiPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Google Gemini",
		Description:  geminiDescription,
		DefaultModel: defaultGeminiModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       true,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: geminiConfigSchema(),
	}, newGeminiClient())
	if err != nil {
		return fmt.Errorf("registering gemini adapter: %w", err)
	}
	return nil
}

func geminiConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Google Gemini API key from https://aistudio.google.com/app/apikey. Stored encrypted at rest.",
			Placeholder: "Google Gemini API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultGeminiBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultGeminiBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultGeminiModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultGeminiModel,
		},
	}
}

func (p *geminiPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-gemini.init", map[string]any{"slug": geminiPluginSlug})
	return nil
}

func (p *geminiPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-gemini.shutdown", nil)
	return nil
}

func (p *geminiPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Google Gemini plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*geminiPlugin)(nil)
var _ plugin.AdapterContributor = (*geminiPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &geminiPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     geminiPluginSlug,
		Name:     geminiPluginName,
		Version:  geminiVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     geminiDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
