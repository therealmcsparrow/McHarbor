// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	deepseekProviderType = "deepseek"
	deepseekPluginSlug   = "ai-provider-deepseek"
	deepseekPluginName   = "DeepSeek"
	deepseekVersion      = "2026.10.01"
	deepseekDescription  = "Routes McHarbor AI chat completions through DeepSeek's OpenAI-compatible /v1/chat/completions endpoint. Models include deepseek-chat (V3) and deepseek-reasoner (R1)."
)

type deepseekPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *deepseekPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *deepseekPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *deepseekPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *deepseekPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *deepseekPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *deepseekPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *deepseekPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: deepseekProviderType,
		Slug:         deepseekPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "DeepSeek",
		Description:  deepseekDescription,
		DefaultModel: defaultDeepseekModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: deepseekConfigSchema(),
	}, newDeepseekClient())
	if err != nil {
		return fmt.Errorf("registering deepseek adapter: %w", err)
	}
	return nil
}

func deepseekConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "DeepSeek API key from https://platform.deepseek.com/api_keys. Stored encrypted at rest.",
			Placeholder: "DeepSeek API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultDeepseekBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultDeepseekBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultDeepseekModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultDeepseekModel,
		},
	}
}

func (p *deepseekPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-deepseek.init", map[string]any{"slug": deepseekPluginSlug})
	return nil
}

func (p *deepseekPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-deepseek.shutdown", nil)
	return nil
}

func (p *deepseekPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("DeepSeek plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*deepseekPlugin)(nil)
var _ plugin.AdapterContributor = (*deepseekPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &deepseekPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     deepseekPluginSlug,
		Name:     deepseekPluginName,
		Version:  deepseekVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     deepseekDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
