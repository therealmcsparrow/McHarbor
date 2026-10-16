// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	localaiProviderType = "localai"
	localaiPluginSlug   = "ai-provider-localai"
	localaiPluginName   = "LocalAI"
	localaiVersion      = "2026.10.01"
	localaiDescription  = "Routes McHarbor AI chat completions through a self-hosted LocalAI instance over its OpenAI-compatible /v1 endpoint. Runs GGUF and other local backends offline."
)

type localaiPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *localaiPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *localaiPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *localaiPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *localaiPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *localaiPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *localaiPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *localaiPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: localaiProviderType,
		Slug:         localaiPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "LocalAI (self-hosted)",
		Description:  localaiDescription,
		DefaultModel: defaultLocalaiModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: localaiConfigSchema(),
	}, newLocalaiClient())
	if err != nil {
		return fmt.Errorf("registering localai adapter: %w", err)
	}
	return nil
}

func localaiConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    false,
			Help:        "Optional bearer token for an authenticated LocalAI (self-hosted) endpoint. Stored encrypted at rest.",
			Placeholder: "LocalAI (self-hosted) API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultLocalaiBaseURL,
			Required:    true,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultLocalaiBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultLocalaiModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultLocalaiModel,
		},
	}
}

func (p *localaiPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-localai.init", map[string]any{"slug": localaiPluginSlug})
	return nil
}

func (p *localaiPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-localai.shutdown", nil)
	return nil
}

func (p *localaiPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("LocalAI (self-hosted) plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*localaiPlugin)(nil)
var _ plugin.AdapterContributor = (*localaiPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &localaiPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     localaiPluginSlug,
		Name:     localaiPluginName,
		Version:  localaiVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     localaiDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
