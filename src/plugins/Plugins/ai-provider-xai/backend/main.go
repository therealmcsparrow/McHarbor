// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	xaiProviderType = "xai"
	xaiPluginSlug   = "ai-provider-xai"
	xaiPluginName   = "xAI Grok"
	xaiVersion      = "2026.10.01"
	xaiDescription  = "Routes McHarbor AI chat completions through xAI's OpenAI-compatible /v1/chat/completions endpoint. Models include Grok 4 and Grok 4 Fast."
)

type xaiPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *xaiPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *xaiPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *xaiPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *xaiPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *xaiPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *xaiPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *xaiPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: xaiProviderType,
		Slug:         xaiPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "xAI Grok",
		Description:  xaiDescription,
		DefaultModel: defaultXaiModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       true,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: xaiConfigSchema(),
	}, newXaiClient())
	if err != nil {
		return fmt.Errorf("registering xai adapter: %w", err)
	}
	return nil
}

func xaiConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "xAI Grok API key from https://console.x.ai/. Stored encrypted at rest.",
			Placeholder: "xAI Grok API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultXaiBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultXaiBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultXaiModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultXaiModel,
		},
	}
}

func (p *xaiPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-xai.init", map[string]any{"slug": xaiPluginSlug})
	return nil
}

func (p *xaiPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-xai.shutdown", nil)
	return nil
}

func (p *xaiPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("xAI Grok plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*xaiPlugin)(nil)
var _ plugin.AdapterContributor = (*xaiPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &xaiPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     xaiPluginSlug,
		Name:     xaiPluginName,
		Version:  xaiVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     xaiDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
