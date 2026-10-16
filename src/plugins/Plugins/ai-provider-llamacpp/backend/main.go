// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	llamacppProviderType = "llamacpp"
	llamacppPluginSlug   = "ai-provider-llamacpp"
	llamacppPluginName   = "llama.cpp"
	llamacppVersion      = "2026.10.01"
	llamacppDescription  = "Routes McHarbor AI chat completions through a self-hosted llama.cpp server (llama-server) over its OpenAI-compatible /v1 endpoint. Runs fully offline."
)

type llamacppPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *llamacppPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *llamacppPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *llamacppPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *llamacppPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *llamacppPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *llamacppPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *llamacppPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: llamacppProviderType,
		Slug:         llamacppPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "llama.cpp (local)",
		Description:  llamacppDescription,
		DefaultModel: defaultLlamacppModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: llamacppConfigSchema(),
	}, newLlamacppClient())
	if err != nil {
		return fmt.Errorf("registering llamacpp adapter: %w", err)
	}
	return nil
}

func llamacppConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    false,
			Help:        "Optional bearer token for an authenticated llama.cpp (local) endpoint. Stored encrypted at rest.",
			Placeholder: "llama.cpp (local) API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultLlamacppBaseURL,
			Required:    true,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultLlamacppBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultLlamacppModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultLlamacppModel,
		},
	}
}

func (p *llamacppPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-llamacpp.init", map[string]any{"slug": llamacppPluginSlug})
	return nil
}

func (p *llamacppPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-llamacpp.shutdown", nil)
	return nil
}

func (p *llamacppPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("llama.cpp (local) plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*llamacppPlugin)(nil)
var _ plugin.AdapterContributor = (*llamacppPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &llamacppPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     llamacppPluginSlug,
		Name:     llamacppPluginName,
		Version:  llamacppVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     llamacppDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
