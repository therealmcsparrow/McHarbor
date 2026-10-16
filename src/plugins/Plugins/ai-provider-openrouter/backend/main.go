// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	openrouterProviderType = "openrouter"
	openrouterPluginSlug   = "ai-provider-openrouter"
	openrouterPluginName   = "OpenRouter"
	openrouterVersion      = "2026.10.01"
	openrouterDescription  = "Routes McHarbor AI chat completions through OpenRouter, a single OpenAI-compatible gateway in front of hundreds of hosted models from many vendors."
)

type openrouterPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *openrouterPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *openrouterPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *openrouterPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *openrouterPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *openrouterPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *openrouterPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *openrouterPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: openrouterProviderType,
		Slug:         openrouterPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "OpenRouter",
		Description:  openrouterDescription,
		DefaultModel: defaultOpenrouterModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       true,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: openrouterConfigSchema(),
	}, newOpenrouterClient())
	if err != nil {
		return fmt.Errorf("registering openrouter adapter: %w", err)
	}
	return nil
}

func openrouterConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "OpenRouter API key from https://openrouter.ai/keys. Stored encrypted at rest.",
			Placeholder: "OpenRouter API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultOpenrouterBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultOpenrouterBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultOpenrouterModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultOpenrouterModel,
		},
	}
}

func (p *openrouterPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-openrouter.init", map[string]any{"slug": openrouterPluginSlug})
	return nil
}

func (p *openrouterPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-openrouter.shutdown", nil)
	return nil
}

func (p *openrouterPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("OpenRouter plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*openrouterPlugin)(nil)
var _ plugin.AdapterContributor = (*openrouterPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &openrouterPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     openrouterPluginSlug,
		Name:     openrouterPluginName,
		Version:  openrouterVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     openrouterDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
