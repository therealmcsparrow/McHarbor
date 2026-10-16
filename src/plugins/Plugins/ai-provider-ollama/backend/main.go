// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main exports the Ollama AI provider plugin loaded by McHarbor.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const ollamaProviderType = "ollama"

type ollamaPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *ollamaPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *ollamaPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *ollamaPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *ollamaPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *ollamaPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *ollamaPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *ollamaPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: ollamaProviderType,
		Slug:         p.manifest.Slug,
		Icon:         "plugin.svg",
		DisplayName:  "Ollama",
		Description:  p.manifest.Desc,
		DefaultModel: defaultOllamaModel,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: ollamaConfigSchema(),
	}, newOllamaClient())
	if err != nil {
		return fmt.Errorf("registering ollama adapter: %w", err)
	}
	return nil
}

func ollamaConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultOllamaBaseURL,
			Required:    true,
			Help:        "Absolute HTTP or HTTPS URL of the Ollama server.",
			Placeholder: defaultOllamaBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultOllamaModel,
			Required:    true,
			Help:        "Model used when a chat request does not select one.",
			Placeholder: defaultOllamaModel,
		},
		{
			Key:      "api_key",
			Type:     "secret",
			Label:    "API key",
			Required: false,
			Secret:   true,
			Help:     "Optional bearer token for an authenticated Ollama endpoint. Stored encrypted at rest by McHarbor.",
		},
	}
}

func (p *ollamaPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-ollama.init", map[string]any{"slug": p.manifest.Slug})
	return nil
}

func (p *ollamaPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-ollama.shutdown", nil)
	return nil
}

func (p *ollamaPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Ollama plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*ollamaPlugin)(nil)
var _ plugin.AdapterContributor = (*ollamaPlugin)(nil)

// Plugin is the symbol resolved by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &ollamaPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     "ai-provider-ollama",
		Name:     "Ollama",
		Version:  "2026.10.01",
		Author:   "McSparrow",
		License:  "Apache-2.0",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     "Routes McHarbor AI chat completions through a self-hosted Ollama instance using the native /api/tags and /api/chat endpoints.",
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "audit.emit"},
		Config:   ollamaConfigSchema(),
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}

func main() {}
