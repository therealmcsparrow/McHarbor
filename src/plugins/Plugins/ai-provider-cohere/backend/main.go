// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	cohereProviderType = "cohere"
	coherePluginSlug   = "ai-provider-cohere"
	coherePluginName   = "Cohere"
	cohereVersion      = "2026.10.01"
	cohereDescription  = "Routes McHarbor AI chat completions through Cohere's OpenAI compatibility API. Models include Command A, Command R+, and Command R."
)

type coherePlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *coherePlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *coherePlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *coherePlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *coherePlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *coherePlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *coherePlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *coherePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: cohereProviderType,
		Slug:         coherePluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "Cohere",
		Description:  cohereDescription,
		DefaultModel: defaultCohereModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: cohereConfigSchema(),
	}, newCohereClient())
	if err != nil {
		return fmt.Errorf("registering cohere adapter: %w", err)
	}
	return nil
}

func cohereConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    true,
			Help:        "Cohere API key from https://dashboard.cohere.com/api-keys. Stored encrypted at rest.",
			Placeholder: "Cohere API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultCohereBaseURL,
			Required:    false,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultCohereBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultCohereModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultCohereModel,
		},
	}
}

func (p *coherePlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-cohere.init", map[string]any{"slug": coherePluginSlug})
	return nil
}

func (p *coherePlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-cohere.shutdown", nil)
	return nil
}

func (p *coherePlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("Cohere plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*coherePlugin)(nil)
var _ plugin.AdapterContributor = (*coherePlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &coherePlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     coherePluginSlug,
		Name:     coherePluginName,
		Version:  cohereVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     cohereDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
