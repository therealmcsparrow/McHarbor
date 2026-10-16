// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// aiRuntimeView is the slice of the host's plugin runtime the
// ai_assistant module actually uses. Mirrors *coreloader.Runtime's
// `AIAdapter` + `AIAdapterSpecs` surface so the bridge can be
// type-asserted against the same runtime instance. The runtime's
// adapter sub-registry is the source of truth — the bridge just
// translates the SDK-typed surface into the module-local pluginDispatch
// shape.
type aiRuntimeView interface {
	AIAdapter(providerType string) (plugin.AIAdapterClient, plugin.AIAdapterSpec, bool)
	AIAdapterSpecs() map[string]plugin.AIAdapterSpec
}

// aiDispatchBridge is the pluginDispatch implementation the
// ai_assistant runtime-facing code casts against. Constructed once at
// boot from main.go and installed via SetPluginRuntime; the bridge
// owns:
//   - the runtime view (SDK-typed AI accessors)
//   - the host DB handle (for reading the plugin's KV bucket)
//   - the encryption service (for unsealing secret-shaped config fields)
//
// Mirrors the existing adapter-bridge pattern in
// modules/storage_locations/plugin_dispatch.go and
// modules/identity/service.go — local interface, runtime held as `any`,
// bridge cast on the consumer side.
type aiDispatchBridge struct {
	rt  aiRuntimeView
	db  *sql.DB
	enc *encryption.Service
}

// NewAIPluginRuntimeBridge wires the bridge. nil-safe: a nil runtime
// yields an empty list (the chat composer's host-side fallback takes
// over). A nil db or nil enc is tolerated; the bridge just returns
// empty config maps instead of decrypting.
func NewAIPluginRuntimeBridge(rt any, db *sql.DB, enc *encryption.Service) *aiDispatchBridge {
	var view aiRuntimeView
	if rt != nil {
		if v, ok := rt.(aiRuntimeView); ok {
			view = v
		}
	}
	return &aiDispatchBridge{rt: view, db: db, enc: enc}
}

// AIProviders returns every installed AI provider plugin, sorted by
// slug for stable UI ordering. The chat composer dropdown and the
// Settings → AI → Providers tab both source from this method.
func (b *aiDispatchBridge) AIProviders() []aiAdapterListItem {
	if b.rt == nil {
		return []aiAdapterListItem{}
	}
	specs := b.rt.AIAdapterSpecs()
	out := make([]aiAdapterListItem, 0, len(specs))
	for _, s := range specs {
		out = append(out, toLocalListItem(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// AIAdapter returns the wrapped plugin client + the plugin's
// decrypted config map. The chat dispatch (`service.go:474`) and the
// "Test connection" button both call this. The config map is loaded
// lazily here so the test and the chat dispatch see the same view.
func (b *aiDispatchBridge) AIAdapter(providerType string) (aiAdapterCaller, map[string]string, bool) {
	if b.rt == nil {
		return nil, nil, false
	}
	cli, spec, ok := b.rt.AIAdapter(providerType)
	if !ok || cli == nil {
		return nil, nil, false
	}
	cfg, _, ok := b.AIConfigFor(providerType)
	if !ok || cfg == nil {
		cfg = map[string]string{}
	}
	return &localAdapterCaller{cli: cli, spec: spec}, cfg, true
}

// AIConfigFor loads the plugin's KV bucket (decrypting any secret
// fields) keyed by its slug. The bool is true when the provider is
// registered, regardless of whether the bucket has any rows — an
// unconfigured plugin still returns an empty map so callers can
// render the not-yet-configured state.
func (b *aiDispatchBridge) AIConfigFor(providerType string) (map[string]string, string, bool) {
	if b.rt == nil {
		return nil, "", false
	}
	specs := b.rt.AIAdapterSpecs()
	var slug string
	for _, s := range specs {
		if s.ProviderType == providerType {
			slug = s.Slug
			break
		}
	}
	if slug == "" {
		return nil, "", false
	}
	cfg, err := b.loadPluginConfig(slug)
	if err != nil {
		// Don't fail loud — the plugin may simply not be configured
		// yet. Return an empty map so the caller can still render
		// the empty state.
		cfg = map[string]string{}
	}
	return cfg, slug, true
}

// loadPluginConfig reads plugin_config rows scoped to the plugin's
// slug and decrypts any rows where is_secret=1. Mirrors the
// host-side read in modules/appstore/plugins/service.go:670-690,
// narrowed to the AI bridge's read-only "give the plugin its config"
// responsibility. The kvAdapter the plugin itself uses
// (loader.go:1607) filters `is_secret=0` and only returns plaintext
// rows — the host reads both with secret-decryption as the source
// of truth for the API key.
func (b *aiDispatchBridge) loadPluginConfig(slug string) (map[string]string, error) {
	if b.db == nil {
		return map[string]string{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), assistantQuickQueryTimeout)
	defer cancel()
	rows, err := b.db.QueryContext(ctx,
		`SELECT key, value_enc, is_secret FROM plugin_config WHERE origin = ?`,
		slug)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	cfg := map[string]string{}
	for rows.Next() {
		var key, value string
		var isSecret bool
		if err := rows.Scan(&key, &value, &isSecret); err != nil {
			return nil, err
		}
		if isSecret && b.enc != nil {
			if plain, err := b.enc.Decrypt(value); err == nil {
				value = plain
			}
		}
		cfg[key] = value
	}
	return cfg, rows.Err()
}

// ---- local adapter wrapper ------------------------------------------------

// localAdapterCaller wraps the SDK's AIAdapterClient and translates
// each call between the ai_assistant module's local types and the
// SDK types. The chat dispatch at service.go:520 calls
// Complete(ctx, config, req) with the local pluginRequest and
// expects *pluginResponse back; the SDK's Complete takes the SDK
// LLMRequest and returns *LLMResponse. The two are mechanically
// equivalent — this wrapper is the only place that knows about the
// translation.
type localAdapterCaller struct {
	cli  plugin.AIAdapterClient
	spec plugin.AIAdapterSpec
}

func (a *localAdapterCaller) Test(ctx context.Context, config map[string]string) (*aiTestResult, error) {
	r, err := a.cli.Test(ctx, config)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return &aiTestResult{Success: false}, nil
	}
	return &aiTestResult{
		Success:   r.Success,
		LatencyMs: r.LatencyMs,
		Reply:     r.Reply,
		Message:   r.Message,
	}, nil
}

func (a *localAdapterCaller) Complete(ctx context.Context, config map[string]string, req pluginRequest) (*pluginResponse, error) {
	r, err := a.cli.Complete(ctx, config, toSDKRequest(req))
	if err != nil {
		return nil, err
	}
	if r == nil {
		return &pluginResponse{}, nil
	}
	return fromSDKResponse(r), nil
}

func (a *localAdapterCaller) Models(ctx context.Context, config map[string]string) ([]aiModelDecl, error) {
	rs, err := a.cli.Models(ctx, config)
	if err != nil {
		return nil, err
	}
	return fromSDKModels(rs), nil
}

// ---- translation helpers --------------------------------------------------

// toLocalListItem converts the SDK spec (the per-provider registration
// record exposed by the plugin runtime) into the ai_assistant-side
// list shape used by `GET /ai-assistant/providers` and the chat
// composer dropdown.
func toLocalListItem(s plugin.AIAdapterSpec) aiAdapterListItem {
	return aiAdapterListItem{
		ProviderType: s.ProviderType,
		Slug:         s.Slug,
		DisplayName:  s.DisplayName,
		Description:  s.Description,
		DefaultModel: s.DefaultModel,
		Models:       fromSDKModels(s.Models),
		Capabilities: fromCaps(s.Capabilities),
		ConfigSchema: fromConfigFields(s.ConfigSchema),
	}
}

func fromCaps(c plugin.AIAdapterCapabilities) aiAdapterListCaps {
	return aiAdapterListCaps{
		EncryptedCredentials: c.EncryptedCredentials,
		SupportsStreaming:    c.SupportsStreaming,
		SupportsTools:        c.SupportsTools,
		SupportsVision:       c.SupportsVision,
		ListModelsDynamic:    c.ListModelsDynamic,
		SupportsOAuth:        c.SupportsOAuth,
	}
}

// fromConfigFields narrows the SDK's ConfigFieldDecl to the local
// configFieldDecl. The SDK's `Default` is `any` (string | int | bool);
// the local shape stores it as a string. `fmt.Sprint` is a safe
// coercion for the three supported types. `Min`/`Max` are dropped —
// the local form doesn't carry them and the AI settings form doesn't
// need them today.
func fromConfigFields(in []plugin.ConfigFieldDecl) []configFieldDecl {
	if len(in) == 0 {
		return nil
	}
	out := make([]configFieldDecl, 0, len(in))
	for _, f := range in {
		var def string
		if f.Default != nil {
			def = fmt.Sprint(f.Default)
		}
		out = append(out, configFieldDecl{
			Key:      f.Key,
			Label:    f.Label,
			Type:     f.Type,
			Required: f.Required,
			Secret:   f.Secret,
			Default:  def,
		})
	}
	return out
}

func toSDKRequest(req pluginRequest) plugin.LLMRequest {
	out := plugin.LLMRequest{
		Model:       req.Model,
		System:      req.System,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if len(req.Messages) > 0 {
		out.Messages = make([]plugin.LLMMessage, len(req.Messages))
		for i, m := range req.Messages {
			out.Messages[i] = plugin.LLMMessage{
				Role:    m.Role,
				Content: m.Content,
				Name:    m.Name,
			}
		}
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]plugin.LLMTool, len(req.Tools))
		for i, t := range req.Tools {
			out.Tools[i] = plugin.LLMTool{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			}
		}
	}
	return out
}

func fromSDKResponse(r *plugin.LLMResponse) *pluginResponse {
	return &pluginResponse{
		Content: r.Content,
		Model:   r.Model,
		Usage: pluginUsage{
			InputTokens:  r.Usage.InputTokens,
			OutputTokens: r.Usage.OutputTokens,
			TotalTokens:  r.Usage.TotalTokens,
		},
		FinishReason: r.FinishReason,
	}
}

func fromSDKModels(in []plugin.ModelDecl) []aiModelDecl {
	if len(in) == 0 {
		return nil
	}
	out := make([]aiModelDecl, len(in))
	for i, m := range in {
		out[i] = aiModelDecl{
			ID:            m.ID,
			DisplayName:   m.DisplayName,
			ContextWindow: m.ContextWindow,
			Capabilities:  m.Capabilities,
		}
	}
	return out
}
