// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	lmstudioProviderType = "lmstudio"
	lmstudioPluginSlug   = "ai-provider-lmstudio"
	lmstudioPluginName   = "LM Studio"
	lmstudioVersion      = "2026.10.01"
	lmstudioDescription  = "Routes McHarbor AI chat completions through a local LM Studio server over its OpenAI-compatible /v1 endpoint. Runs fully offline on the operator's own hardware."
)

type lmstudioPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *lmstudioPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *lmstudioPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

func (p *lmstudioPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

func (p *lmstudioPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

func (p *lmstudioPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

func (p *lmstudioPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

func (p *lmstudioPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	err := reg.AI().Register(plugin.AIAdapterSpec{
		ProviderType: lmstudioProviderType,
		Slug:         lmstudioPluginSlug,
		Icon:         "plugin.svg",
		DisplayName:  "LM Studio (local)",
		Description:  lmstudioDescription,
		DefaultModel: defaultLmstudioModel,
		Models:       nil,
		Capabilities: plugin.AIAdapterCapabilities{
			EncryptedCredentials: true,
			SupportsStreaming:    false,
			SupportsTools:        true,
			SupportsVision:       false,
			ListModelsDynamic:    true,
			SupportsOAuth:        false,
		},
		ConfigSchema: lmstudioConfigSchema(),
	}, newLmstudioClient())
	if err != nil {
		return fmt.Errorf("registering lmstudio adapter: %w", err)
	}
	return nil
}

func lmstudioConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_key",
			Type:        "secret",
			Label:       "API key",
			Secret:      true,
			Required:    false,
			Help:        "Optional bearer token for an authenticated LM Studio (local) endpoint. Stored encrypted at rest.",
			Placeholder: "LM Studio (local) API key",
		},
		{
			Key:         "base_url",
			Type:        "string",
			Label:       "Base URL",
			Default:     defaultLmstudioBaseURL,
			Required:    true,
			Help:        "Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions.",
			Placeholder: defaultLmstudioBaseURL,
		},
		{
			Key:         "default_model",
			Type:        "string",
			Label:       "Default model",
			Default:     defaultLmstudioModel,
			Required:    false,
			Help:        "Model used when a request does not select one explicitly.",
			Placeholder: defaultLmstudioModel,
		},
	}
}

func (p *lmstudioPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.audit("plugin.ai-provider-lmstudio.init", map[string]any{"slug": lmstudioPluginSlug})
	return nil
}

func (p *lmstudioPlugin) Shutdown(_ context.Context) error {
	p.audit("plugin.ai-provider-lmstudio.shutdown", nil)
	return nil
}

func (p *lmstudioPlugin) audit(action string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if err := p.host.Audit.Emit(action, fields); err != nil && p.host.Logger != nil {
		p.host.Logger.Warn("LM Studio (local) plugin audit event failed", "action", action, "error", err)
	}
}

var _ plugin.Plugin = (*lmstudioPlugin)(nil)
var _ plugin.AdapterContributor = (*lmstudioPlugin)(nil)

// Plugin is the symbol loaded by the McHarbor plugin runtime.
var Plugin plugin.Plugin = &lmstudioPlugin{
	manifest: plugin.Manifest{
		Schema:   "mcharbor.plugin/v1",
		Slug:     lmstudioPluginSlug,
		Name:     lmstudioPluginName,
		Version:  lmstudioVersion,
		Author:   "McSparrow",
		License:  "McHarbor",
		Homepage: "https://www.mcharbor.eu/docs/plugins",
		Desc:     lmstudioDescription,
		Mcharbor: ">=2.1.0 <3.0.0",
		Go:       ">=1.23",
		Perms:    []string{"ai.adapter.register", "encryption.handle.read", "audit.emit"},
		Caps: plugin.Capabilities{
			OutboundHTTP: true,
		},
	},
}
