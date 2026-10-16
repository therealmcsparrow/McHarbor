// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	vllmProviderType = "vllm"
	vllmPluginSlug   = "ai-provider-vllm"
	vllmPluginName   = "vLLM"
	vllmVersion      = "2026.10.01"
	vllmDescription  = "Routes McHarbor AI chat completions through a self-hosted vLLM OpenAI-compatible server. Suited to GPU inference of open-weights models on the operator's own cluster."
)

type vllmPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *vllmPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *vllmPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *vllmPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *vllmPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *vllmPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *vllmPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *vllmPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: vllmProviderType,
		Slug:         vllmPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "vLLM (self-hosted)",
		Description:  vllmDescription,
		DefaultModel: defaultVllmModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: vllmConfigSchema(),
	}, newVllmClient())
	if err != nil {
		return fmt.Errorf("registering vllm adapter: %w", err)
	}
	return nil
}

func vllmConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    false,
			Help:        "Optional bearer token for an authenticated vLLM (self-hosted) endpoint. Stored encrypted at rest.",
			Placeholder: "vLLM (self-hosted) API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultVllmBaseURL,
			Required:    true,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultVllmBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultVllmModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultVllmModel,
		},
	}
}

func (p *vllmPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-vllm.init", map[string]any{"slug": vllmPluginSlug})
	return nil
}

func (p *vllmPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-vllm.shutdown", nil)
	return nil
}

func (p *vllmPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("vLLM (self-hosted) plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*vllmPlugin)(nil)
var _ plugin.AdapterContributor = (*vllmPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &vllmPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     vllmPluginSlug,
		Name:     vllmPluginName,
		Version:  vllmVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     vllmDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
