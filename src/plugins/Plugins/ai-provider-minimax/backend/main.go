// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	miniMaxProviderType = "minimax"
	miniMaxSlug         = "ai-provider-minimax"
)

type miniMaxPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *miniMaxPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *miniMaxPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *miniMaxPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *miniMaxPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *miniMaxPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *miniMaxPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *miniMaxPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: miniMaxProviderType,
		Slug:         miniMaxSlug,
		Icon:         "plugin.svg",
		DisplayName:  "MiniMax",
		Description:  p.manifest.Desc,
		DefaultModel: defaultMiniMaxModel,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: miniMaxConfigSchema(),
	}, newMiniMaxClient())
}

func miniMaxConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "API key from the MiniMax platform. Stored encrypted at rest.",
			Placeholder: "Enter your MiniMax API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultMiniMaxBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS OpenAI-compatible API base URL, including /v1.",
			Placeholder: defaultMiniMaxBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultMiniMaxModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultMiniMaxModel,
		},
	}
}

func (p *miniMaxPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit == nil {
		return nil
	}
	if err := host.Audit.Emit("plugin.ai-provider-minimax.init", map[string]any{
		"slug":    p.manifest.Slug,
		"version": p.manifest.Version,
	}); err != nil {
		return fmt.Errorf("emitting minimax plugin init audit event: %w", err)
	}
	return nil
}

func (p *miniMaxPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit == nil {
		return nil
	}
	if err := p.host.Audit.Emit("plugin.ai-provider-minimax.shutdown", nil); err != nil {
		return fmt.Errorf("emitting minimax plugin shutdown audit event: %w", err)
	}
	return nil
}

var _ plugin.Plugin = (*miniMaxPlugin)(nil)
var _ plugin.AdapterContributor = (*miniMaxPlugin)(nil)

var Plugin plugin.Plugin = &miniMaxPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     miniMaxSlug,
		Name:     "MiniMax",
		Version:  "2026.10.01",
		Author:   "McSparrow",
		License:  "Apache-2.0",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     "Routes McHarbor AI chat completions through MiniMax's OpenAI-compatible /v1/chat/completions endpoint. Models include MiniMax-Text-01 and MiniMax-VL.",
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}

func main() {}
