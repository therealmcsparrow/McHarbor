// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	perplexityProviderType = "perplexity"
	perplexityPluginSlug   = "ai-provider-perplexity"
	perplexityPluginName   = "Perplexity"
	perplexityVersion      = "2026.10.01"
	perplexityDescription  = "Routes McHarbor AI chat completions through Perplexity's OpenAI-compatible Sonar API, which answers with live web search grounding."
)

type perplexityPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *perplexityPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *perplexityPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *perplexityPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *perplexityPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *perplexityPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *perplexityPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *perplexityPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: perplexityProviderType,
		Slug:         perplexityPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Perplexity",
		Description:  perplexityDescription,
		DefaultModel: defaultPerplexityModel,
		Models:       perplexityModels(),
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        false,
			SupportsVision:       false,
			ListModelsDynamic:    false,
			SupportsOAuth:        false,
		},
		ConfigSchema: perplexityConfigSchema(),
	}, newPerplexityClient())
	if err != nil {
		return fmt.Errorf("registering perplexity adapter: %w", err)
	}
	return nil
}

// perplexityModels is the static model catalog surfaced in the composer's
// model dropdown. Perplexity publishes no /models endpoint, so the
// list is versioned with the plugin.
func perplexityModels() []plugin.ModelDecl {
	return []plugin.ModelDecl{
		{ID: "sonar", DisplayName: "Sonar", ContextWindow: 128000, Capabilities: []string{"chat"}},
		{ID: "sonar-pro", DisplayName: "Sonar Pro", ContextWindow: 200000, Capabilities: []string{"chat"}},
		{ID: "sonar-reasoning", DisplayName: "Sonar Reasoning", ContextWindow: 128000, Capabilities: []string{"chat"}},
		{ID: "sonar-reasoning-pro", DisplayName: "Sonar Reasoning Pro", ContextWindow: 128000, Capabilities: []string{"chat"}},
		{ID: "sonar-deep-research", DisplayName: "Sonar Deep Research", ContextWindow: 128000, Capabilities: []string{"chat"}},
	}
}

func perplexityConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Perplexity API key from https://www.perplexity.ai/settings/api. Stored encrypted at rest.",
			Placeholder: "Perplexity API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultPerplexityBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultPerplexityBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultPerplexityModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultPerplexityModel,
		},
	}
}

func (p *perplexityPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-perplexity.init", map[string]any{"slug": perplexityPluginSlug})
	return nil
}

func (p *perplexityPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-perplexity.shutdown", nil)
	return nil
}

func (p *perplexityPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Perplexity plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*perplexityPlugin)(nil)
var _ plugin.AdapterContributor = (*perplexityPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &perplexityPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     perplexityPluginSlug,
		Name:     perplexityPluginName,
		Version:  perplexityVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     perplexityDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
