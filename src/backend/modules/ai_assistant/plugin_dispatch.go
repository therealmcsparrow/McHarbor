// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import "context"

// This file holds the SDK-shaped types the ai_assistant module uses
// when dispatching chat requests to AI provider plugins. They mirror
// `plugin.AIAdapterClient` / `plugin.LLMRequest` / etc. but live here
// (rather than imported from core/plugin) so the plugin runtime can
// inject a thin test-double adapter and the ai_assistant module stays
// decoupled from the runtime's compiled types.

// aiAdapterCaller is the slice of plugin.AIAdapterClient we use: Test
// for the admin UI's "Test connection" button, Complete for chat.
// Streaming is a follow-up; the v1 SDK only exposes Complete().
type aiAdapterCaller interface {
	Test(ctx context.Context, config map[string]string) (*aiTestResult, error)
	Complete(ctx context.Context, config map[string]string, req pluginRequest) (*pluginResponse, error)
	Models(ctx context.Context, config map[string]string) ([]aiModelDecl, error)
}

// pluginRequest is the wire shape handed to the plugin's Complete().
// Mirrors plugin.LLMRequest.
type pluginRequest struct {
	Model       string       `json:"model,omitempty"`
	System      string       `json:"system,omitempty"`
	Messages    []pluginMsg  `json:"messages"`
	MaxTokens   int          `json:"maxTokens,omitempty"`
	Temperature float32      `json:"temperature,omitempty"`
	Tools       []pluginTool `json:"tools,omitempty"`
}

// pluginMsg is one chat turn. Mirrors plugin.LLMMessage.
type pluginMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// pluginTool is one function-calling tool. Mirrors plugin.LLMTool.
type pluginTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// pluginResponse is the assembled provider reply. Mirrors plugin.LLMResponse.
type pluginResponse struct {
	Content      string      `json:"content"`
	Model        string      `json:"model"`
	Usage        pluginUsage `json:"usage"`
	FinishReason string      `json:"finishReason"`
}

// pluginUsage is the per-token accounting. Mirrors plugin.LLMUsage.
type pluginUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// aiTestResult mirrors plugin.AITestResult — surfaced by the AI
// chat composer's "Test connection" button (in the plugin's settings
// page, not the host's chat composer).
type aiTestResult struct {
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latencyMs"`
	Reply     string `json:"reply,omitempty"`
	Message   string `json:"message,omitempty"`
}

// aiModelDecl mirrors plugin.ModelDecl — drives the model dropdown
// in the AI chat composer.
type aiModelDecl struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"displayName"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

// aiAdapterListItem is the AI entry surfaced in the host's
// `GET /ai-assistant/providers` endpoint and the chat composer
// dropdown. Mirrors plugin.AIAdapterSpec with slug+providerType.
type aiAdapterListItem struct {
	ProviderType string            `json:"providerType"`
	Slug         string            `json:"slug"`
	DisplayName  string            `json:"displayName"`
	Description  string            `json:"description,omitempty"`
	DefaultModel string            `json:"defaultModel,omitempty"`
	Models       []aiModelDecl     `json:"models,omitempty"`
	Capabilities aiAdapterListCaps `json:"capabilities"`
	ConfigSchema []configFieldDecl `json:"configSchema,omitempty"`
}

// configFieldDecl mirrors plugin.ConfigFieldDecl (the smallest
// subset of fields the AI composer / chat panel needs to render the
// plugin's settings page).
type configFieldDecl struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// aiAdapterListCaps mirrors the booleans in plugin.AIAdapterCapabilities.
type aiAdapterListCaps struct {
	EncryptedCredentials bool `json:"encryptedCredentials"`
	SupportsStreaming    bool `json:"supportsStreaming"`
	SupportsTools        bool `json:"supportsTools"`
	SupportsVision       bool `json:"supportsVision"`
	ListModelsDynamic    bool `json:"listModelsDynamic"`
	SupportsOAuth        bool `json:"supportsOAuth"`
}
