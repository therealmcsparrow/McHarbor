// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main exports the Anthropic AI provider plugin loaded by McHarbor.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const anthropicProviderType = "anthropic"

type anthropicPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *anthropicPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *anthropicPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *anthropicPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *anthropicPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *anthropicPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *anthropicPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *anthropicPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: anthropicProviderType,
		Slug:         p.manifest.Slug,
		Icon:         "plugin.svg",
		DisplayName:  "Anthropic Claude",
		Description:  p.manifest.Desc,
		DefaultModel: defaultAnthropicModel,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: anthropicConfigSchema(),
	}, newAnthropicClient())
}

func anthropicConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Anthropic API key from https://console.anthropic.com/settings/keys. Encrypted at rest.",
			Placeholder: "sk-ant-...",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultAnthropicBaseURL,
			Help:        "Absolute HTTP or HTTPS Anthropic API base URL.",
			Placeholder: defaultAnthropicBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultAnthropicModel,
			Help:        "Anthropic model ID used when a request does not select a model.",
			Placeholder: defaultAnthropicModel,
		},
	}
}

func (p *anthropicPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		if err := host.Audit.Emit("plugin.ai-provider-anthropic.init", map[string]any{
			"slug":    p.manifest.Slug,
			"version": p.manifest.Version,
		}); err != nil && host.Logger != nil {
			host.Logger.Warn("failed to audit Anthropic plugin initialization", "error", err)
		}
	}
	return nil
}

func (p *anthropicPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		if err := p.host.Audit.Emit("plugin.ai-provider-anthropic.shutdown", nil); err != nil && p.host.Logger != nil {
			p.host.Logger.Warn("failed to audit Anthropic plugin shutdown", "error", err)
		}
	}
	return nil
}

var _ plugin.Plugin = (*anthropicPlugin)(nil)
var _ plugin.AdapterContributor = (*anthropicPlugin)(nil)

// Plugin is the symbol resolved by Go's plugin loader.
var Plugin plugin.Plugin = &anthropicPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     "ai-provider-anthropic",
		Name:     "Anthropic Claude",
		Version:  "2026.10.01",
		Author:   "McSparrow",
		License:  "Apache-2.0",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     "Routes McHarbor AI chat completions through Anthropic /v1/messages. Claude Haiku 4.5, Sonnet 4.5, Opus 4.5.",
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms: []string{
			"ai.adapter.register",
			"encryption.handle.read",
			"audit.emit",
		},
		Config: anthropicConfigSchema(),
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
