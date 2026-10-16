// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	aiassistant "github.com/therealmcsparrow/mcharbor/modules/ai_assistant"
	"github.com/therealmcsparrow/mcharbor/modules/incidents"
)

// selfHostedProviders run inference on hardware the operator runs; the opt-in
// says so, because "sent to your own Ollama" and "sent to a vendor" differ.
var selfHostedProviders = map[string]bool{"ollama": true, "lmstudio": true, "llamacpp": true, "localai": true, "vllm": true}

type aiAdapterSource interface {
	AIAdapter(providerType string) (plugin.AIAdapterClient, plugin.AIAdapterSpec, bool)
}

// incidentModel narrates through the operator's default AI connection. It
// sends one user message and reads back text: no tools, no history.
type incidentModel struct {
	resolver aiassistant.ProviderResolver
	adapters any
}

// NewIncidentModel returns the narrator's model: the default AI provider
// connection, available exactly when the assistant's kill switch says AI is on.
func NewIncidentModel(db *sql.DB, enc *encryption.Service, pluginAdapters any) incidents.Model {
	return incidentModel{resolver: NewAIProviderResolver(db, enc, pluginAdapters), adapters: pluginAdapters}
}

func (m incidentModel) adapter(ctx context.Context) (*aiassistant.ProviderConnection, map[string]string, plugin.AIAdapterClient, plugin.AIAdapterSpec, error) {
	conn, config, err := m.resolver.Resolve(ctx, "")
	if err != nil || conn == nil || !conn.Enabled {
		return nil, nil, nil, plugin.AIAdapterSpec{}, incidents.ErrNoModel
	}
	src, ok := m.adapters.(aiAdapterSource)
	if !ok || src == nil {
		return nil, nil, nil, plugin.AIAdapterSpec{}, incidents.ErrNoModel
	}
	client, spec, loaded := src.AIAdapter(conn.ProviderType)
	if !loaded || client == nil {
		return nil, nil, nil, plugin.AIAdapterSpec{}, incidents.ErrNoModel
	}
	return conn, config, client, spec, nil
}

func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// Info names where evidence would go, without contacting the provider.
func (m incidentModel) Info(ctx context.Context) (incidents.ModelInfo, error) {
	conn, _, _, spec, err := m.adapter(ctx)
	if err != nil {
		return incidents.ModelInfo{}, err
	}
	model := conn.DefaultModel
	if model == "" {
		model = spec.DefaultModel
	}
	return incidents.ModelInfo{
		Provider: conn.ProviderType, Connection: conn.Name, Model: model,
		BaseURL: hostOnly(conn.BaseURL), Local: selfHostedProviders[conn.ProviderType],
	}, nil
}

// Complete sends the prompt and returns the model's text.
func (m incidentModel) Complete(ctx context.Context, system, prompt string, maxTokens int) (string, error) {
	conn, config, client, spec, err := m.adapter(ctx)
	if err != nil {
		return "", err
	}
	model := conn.DefaultModel
	if model == "" {
		model = spec.DefaultModel
	}
	resp, err := client.Complete(ctx, config, plugin.LLMRequest{
		Model: model, System: system, MaxTokens: maxTokens, Temperature: 0.1,
		Messages: []plugin.LLMMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", conn.ProviderType, err)
	}
	if resp == nil {
		return "", fmt.Errorf("%s: empty response", conn.ProviderType)
	}
	return resp.Content, nil
}
