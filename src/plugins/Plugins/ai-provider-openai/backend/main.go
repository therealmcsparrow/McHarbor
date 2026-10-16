// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main exports the OpenAI AI provider plugin symbol loaded by
// McHarbor's plugin runtime.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const openAIProviderType = "openai"

type openAIPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *openAIPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *openAIPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *openAIPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *openAIPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *openAIPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *openAIPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *openAIPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(
		plugin.AIAdapterSpec{
			ProviderType: openAIProviderType,
			Slug:         p.manifest.Slug,
			Icon:         "plugin.svg",
			DisplayName:  "OpenAI",
			Description:  p.manifest.Desc,
			DefaultModel: defaultOpenAIModel,
			Capabilities: plugin.AIAdapterCapabilities{
				EncryptedCredentials: true,
				SupportsStreaming:    false,
				SupportsTools:        true,
				SupportsVision:       false,
				ListModelsDynamic:    true,
				SupportsOAuth:        false,
			},
			ConfigSchema: openAIConfigSchema(),
		},
		newOpenAIClient(),
	)
	if err != nil {
		return fmt.Errorf("ai-provider-openai: register adapter: %w", err)
	}
	return nil
}

func openAIConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:      "api_key",
			Type:     "secret",
			Label:    "API Key",
			Required: true,
			Secret:   true,
			Help:     "OpenAI API key. Stored encrypted at rest and sent only as a Bearer token.",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Required:    true,
			Default:     defaultOpenAIBaseURL,
			Placeholder: defaultOpenAIBaseURL,
			Help:        "Absolute HTTP or HTTPS API base URL.",
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default Model",
			Required:    false,
			Default:     defaultOpenAIModel,
			Placeholder: defaultOpenAIModel,
			Help:        "Model used when a request does not select one explicitly.",
		},
	}
}

func (p *openAIPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.emitAudit("plugin.ai-provider-openai.init", map[string]any{
		"slug":    p.manifest.Slug,
		"version": p.manifest.Version,
	})
	return nil
}

func (p *openAIPlugin) Shutdown(_ context.Context) error {
	p.emitAudit("plugin.ai-provider-openai.shutdown", nil)
	return nil
}

func (p *openAIPlugin) emitAudit(event string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(event, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("OpenAI plugin audit event failed", "event", event, "error", err)
	}
}

var _ plugin.Plugin = (*openAIPlugin)(nil)
var _ plugin.AdapterContributor = (*openAIPlugin)(nil)

// Plugin is the symbol resolved by plugin.Open.
var Plugin plugin.Plugin = &openAIPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     "ai-provider-openai",
		Name:     "OpenAI",
		Version:  "2026.10.01",
		Author:   "McSparrow",
		License:  "Apache-2.0",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     "Routes McHarbor AI chat completions through OpenAI /v1/chat/completions. GPT-4o, GPT-4o mini, o-series.",
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms: []string{
			"ai.adapter.register",
			"encryption.handle.read",
			"audit.emit",
		},
		Config: openAIConfigSchema(),
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}

func main() {}
