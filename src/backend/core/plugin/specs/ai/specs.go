// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package ai defines the AI-provider-family adapter spec types
// (sub-registry, Spec struct, Client interface, capabilities, the
// LLM message / tool / response / usage shapes, and the per-provider
// ModelDecl used for model catalogs).
//
// Why split: see the package doc in `core/plugin/contract`.
//
// Imported by:
//   - host modules that route AI requests
//     (ai_assistant, workflows with AI nodes)
//   - AI provider plugins (ai-provider-anthropic,
//     ai-provider-openai, ai-provider-ollama, etc.)
package ai

import (
	"context"
	"encoding/json"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// AIAdapterRegistry is the sub-registry for AI provider plugins
// (Anthropic, OpenAI, Ollama, Mistral, etc.). A plugin registers one
// entry per provider type it handles. Registering the same
// ProviderType twice from the same plugin (or two different plugins)
// is an error — the host uses the ProviderType as the lookup key,
// matching the ai_provider_connections.provider_type column.
//
// As of the 2026-08-01 conversion, ALL AI providers ship as plugins.
// The host retains only the ai_provider_connections CRUD surface
// (modules/ai_providers) and the chat dispatcher in
// modules/ai_assistant; per-provider-type runtime work is delegated
// to whichever plugin owns the ProviderType.
type AIAdapterRegistry interface {
	// Register adds an adapter. The host dispatches an
	// ai_provider_connections row with provider_type == spec.ProviderType
	// to client. The client must be safe for concurrent use — the
	// host invokes Test/Complete/Models from parallel goroutines
	// during chat fan-out and the admin UI's parallel "test
	// connection" sweep.
	Register(spec AIAdapterSpec, client AIAdapterClient) error
}

// AIAdapterSpec describes one AI provider type a plugin handles.
// The host uses the spec to render the admin UI's provider_type
// dropdown, the chat composer dropdown, and to route Test /
// Complete / Models at runtime.
type AIAdapterSpec struct {
	// ProviderType is the unique key the host uses to dispatch
	// providers. Must match a value stored in the host's
	// ai_provider_connections.provider_type column. Common values
	// include "anthropic", "openai", "ollama", "mistral".
	ProviderType string

	// Slug is the plugin slug (e.g. "ai-provider-anthropic"). The
	// host uses it to look up the plugin's KV config bucket when
	// dispatching Test/Complete/Models. Required.
	Slug string

	// Icon is the asset filename inside the plugin's icons/
	// directory (resolved by the host's /api/plugins/loader/icon
	// route at render time). Empty string is valid — renderers
	// fall back to their default tile (IconBolt). Plugins ship a
	// single `plugin.svg` brand icon, so this is "plugin.svg" for
	// every AI provider today; the field exists so the admin UI's
	// provider tiles and connection cards stop rendering one
	// hardcoded icon for every provider type.
	//
	// The host trusts the value because it is declared in the
	// plugin's own manifest — an attacker who can edit a plugin's
	// manifest already controls the .so bundle.
	Icon string

	// DisplayName is the user-facing label shown in the admin UI's
	// provider_type dropdown and on the chat composer.
	DisplayName string

	// Description is the longer one-paragraph description the admin
	// UI surfaces in the connection card. Optional.
	Description string

	// DefaultModel is the model ID the chat composer pre-fills when
	// the operator picks this provider for the first time. Optional;
	// plugins that surface a dynamic model catalog leave this empty.
	DefaultModel string

	// Models is the static model catalog the admin UI shows when
	// ListModelsDynamic=false. Plugins that prefer dynamic discovery
	// leave this empty and rely on Models() to populate the dropdown
	// at runtime.
	Models []ModelDecl

	// Capabilities describes what the adapter supports. The host
	// uses these to hide unsupported actions in the admin UI and to
	// short-circuit unsupported calls with ErrUnsupported.
	Capabilities AIAdapterCapabilities

	// ConfigSchema is the list of form fields the admin UI renders
	// when an operator picks this provider type. The host stores
	// these in the plugin_config table scoped by the Slug.
	ConfigSchema []types.ConfigFieldDecl

	// OAuthDefaults, when set together with Capabilities.SupportsOAuth,
	// ships the non-secret half of this provider's OAuth device-flow
	// config (client ID + endpoints + scopes) so the operator signs in
	// with no hand-entry — the legitimate "zero-config" sign-in. The
	// host merges it with any per-connection value the operator typed
	// (the typed value wins) and fills it in at sign-in time, so a
	// later plugin update to these defaults reaches every existing
	// connection. A client secret is never carried here; see the
	// OAuthDefaults type doc. Nil = the operator brings their own
	// client ID, the pre-existing behaviour.
	OAuthDefaults *types.OAuthDefaults
}

// AIAdapterCapabilities declares what the adapter supports. The host
// consults these before invoking the corresponding method on
// AIAdapterClient.
type AIAdapterCapabilities struct {
	// EncryptedCredentials indicates the adapter accepts credential
	// fields the host should encrypt at rest (API keys, OAuth
	// tokens). Plugins that need only public metadata (e.g. an
	// Ollama endpoint URL) leave this false.
	EncryptedCredentials bool

	// SupportsStreaming indicates the adapter can stream tokens via
	// Complete's response writer. The host uses this to choose the
	// non-streaming or streaming chat path.
	SupportsStreaming bool

	// SupportsTools indicates the adapter handles tool/function-
	// calling in its Completions request. Plugins without tool
	// support ignore the Tools field on LLMRequest.
	SupportsTools bool

	// SupportsVision indicates the adapter accepts image content in
	// LLMMessage.Content as data URLs. Plugins without vision
	// support reject image content.
	SupportsVision bool

	// ListModelsDynamic indicates the adapter implements Models()
	// to fetch a fresh model catalog from the upstream API. When
	// false, the host uses spec.Models as the dropdown source.
	ListModelsDynamic bool

	// SupportsOAuth indicates the adapter implements an OAuth flow
	// for delegated access (rare for AI today; reserved for future
	// providers). The host uses this to render an OAuth setup
	// wizard instead of a plain credentials form.
	SupportsOAuth bool
}

// AITestResult is what AIAdapterClient.Test returns. The host renders
// it directly in the "Test connection" panel: success/failure, the
// round-trip latency, an optional reply snippet (some providers echo
// a one-line greeting on success), and a human-readable status
// message.
type AITestResult struct {
	Success   bool
	LatencyMs int
	Reply     string
	Message   string
}

// ModelDecl describes a single model an AI provider supports. The
// host renders these in the chat composer model dropdown. Plugins
// that prefer dynamic discovery return a fresh []ModelDecl from the
// Models() method; plugins with a static catalog populate
// AIAdapterSpec.Models instead.
type ModelDecl struct {
	ID            string
	DisplayName   string
	ContextWindow int
	Capabilities  []string
}

// LLMRequest is the payload the host hands to
// AIAdapterClient.Complete. Plugins translate this into their
// upstream provider's native shape (Anthropic /v1/messages,
// OpenAI-compatible /v1/chat/completions, Ollama /api/chat, etc.).
type LLMRequest struct {
	Model       string
	System      string
	Messages    []LLMMessage
	MaxTokens   int
	Temperature float32
	Tools       []LLMTool
}

// LLMMessage is one chat turn. Plugins that don't support vision
// receive text-only content; plugins that declared
// SupportsVision=true may receive image content (data URLs) in
// Content.
type LLMMessage struct {
	Role    string
	Content string
	Name    string
}

// LLMTool describes one function-calling tool the host exposes to the
// model. Plugins that declared SupportsTools=false ignore this.
//
// The host owns the wire-format conversion (OpenAI's `tools` array,
// Anthropic's `tools` array, Gemini's `functionDeclarations`, etc.).
// Plugins receive LLMTool and translate it into their provider's
// native shape verbatim.
type LLMTool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// LLMToolCall is a structured tool invocation the model emits. Plugins
// that declared SupportsTools=true MUST populate ToolCalls whenever
// the provider returned a non-text response (OpenAI `tool_calls`,
// Anthropic `tool_use` blocks, Gemini `functionCall`, etc.). Plugins
// that haven't been updated to the structured contract leave
// ToolCalls nil and the host falls back to its text-mode parser.
type LLMToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// LLMResponse is the assembled reply a plugin returns from
// Complete. Plugins that stream tokens instead of returning a single
// response should still synthesize the final LLMResponse when the
// stream ends so non-streaming callers can use a uniform shape.
type LLMResponse struct {
	Content      string
	ToolCalls    []LLMToolCall
	Model        string
	Usage        LLMUsage
	FinishReason string
}

// LLMUsage is the per-token accounting the host surfaces to the
// admin UI for cost / quota tracking. Plugins that don't get usage
// from their upstream provider return zero values; the host renders
// an "unavailable" state in that case.
type LLMUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// AIAdapterClient is the runtime contract for plugin-provided AI
// providers. The host calls these methods from:
//   - the admin UI's "Test connection" button (Test),
//   - the chat composer dispatch in modules/ai_assistant (Complete),
//   - the model dropdown refresh in the admin UI (Models).
//
// All methods receive a config map carrying the plugin's
// plugin_config rows (api_key, base_url, model, …) keyed by column
// name. Plugins decide which keys they need; the host does not
// pre-validate the map.
//
// Implementations MUST be safe for concurrent use. The host invokes
// Test/Complete/Models from parallel goroutines during chat fan-out.
type AIAdapterClient interface {
	// Test verifies the provider is reachable and the credentials
	// work. The host calls this from the admin UI's "Test
	// connection" button. Implementations should validate every
	// config field they will use at runtime and return a
	// descriptive AITestResult on failure (Success=false plus a
	// Message). A successful test may include a one-line Reply so
	// the admin UI can show "provider answered".
	Test(ctx context.Context, config map[string]string) (*AITestResult, error)

	// Complete runs a chat completion against the configured
	// provider. Plugins translate the LLMRequest into their
	// upstream provider's native shape (Anthropic /v1/messages,
	// OpenAI-compatible /v1/chat/completions, etc.) and return the
	// assembled reply. Plugins that declared SupportsStreaming=true
	// MAY implement streaming internally; the host still receives a
	// single LLMResponse on return.
	Complete(ctx context.Context, config map[string]string, req LLMRequest) (*LLMResponse, error)

	// Models returns the fresh model catalog the provider supports.
	// Plugins that declared ListModelsDynamic=true use this to keep
	// the dropdown in sync with the upstream provider's offerings.
	// Plugins that declared ListModelsDynamic=false may return an
	// empty slice or the static catalog from spec.Models.
	Models(ctx context.Context, config map[string]string) ([]ModelDecl, error)
}
