// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	groqProviderType = "groq"
	groqPluginSlug   = "ai-provider-groq"
	groqPluginName   = "Groq"
	groqVersion      = "2026.10.01"
	groqDescription  = "Routes McHarbor AI chat completions through Groq's OpenAI-compatible LPU inference API. Serves Llama, Qwen, and Kimi open-weights models at very low latency."
)

type groqPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *groqPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *groqPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *groqPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *groqPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *groqPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *groqPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *groqPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: groqProviderType,
		Slug:         groqPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Groq",
		Description:  groqDescription,
		DefaultModel: defaultGroqModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: groqConfigSchema(),
	}, newGroqClient())
	if err != nil {
		return fmt.Errorf("registering groq adapter: %w", err)
	}
	return nil
}

func groqConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Groq API key from https://console.groq.com/keys. Stored encrypted at rest.",
			Placeholder: "Groq API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultGroqBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultGroqBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultGroqModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultGroqModel,
		},
	}
}

func (p *groqPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-groq.init", map[string]any{"slug": groqPluginSlug})
	return nil
}

func (p *groqPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-groq.shutdown", nil)
	return nil
}

func (p *groqPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Groq plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*groqPlugin)(nil)
var _ plugin.AdapterContributor = (*groqPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &groqPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     groqPluginSlug,
		Name:     groqPluginName,
		Version:  groqVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     groqDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
