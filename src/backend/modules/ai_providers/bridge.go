// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package ai_providers stores AI provider connections and OAuth sign-in state and bridges them to the provider plugins.
package ai_providers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// AIHandle's doc comment (core/plugin/services/services.go) promises:
// "the host enforces per-plugin rate limits (default 60/min) and a
// per-call timeout (default 30s)". These constants are that promise's
// two defaults. aiCompletionTimeout bounds a single Complete call;
// aiRateLimitPerMinute / aiRateLimitWindow bound how often one plugin
// (keyed by the slug core/pluginruntime's aiAdapter forwards — never
// supplied by the plugin itself, so it cannot be spoofed) may call
// Complete.
const (
	aiCompletionTimeout  = 30 * time.Second
	aiRateLimitPerMinute = 60
	aiRateLimitWindow    = time.Minute
)

// HostBridge implements pluginruntime.AICompleterSource — the
// host-side callback surface every plugin's host.AI calls eventually
// reach (via core/pluginruntime's aiAdapter, which wraps whatever is
// installed as pluginruntime.HostServices.AI). Row resolution
// (ai_provider_connections) and provider dispatch (provider_type ->
// whichever plugin registered an AIAdapterClient for it) both already
// live in this package's Service — HostBridge only adds the
// SDK-shaped translation and the loader-local type conformance Task 2
// requires.
//
// HostBridge holds *router.AppDeps rather than *sql.DB /
// *encryption.Service / the plugin runtime directly, on purpose: at
// the point bootPluginRuntime (main.go) constructs
// pluginruntime.HostServices to pass into pluginruntime.Boot,
// app.PluginAdapters — the *pluginruntime.Runtime that
// Service.lookupAIAdapter needs to resolve a provider_type to a
// registered plugin adapter — does not exist yet; Boot has not
// returned. Every real AI call happens later, once a plugin's own
// request handler invokes host.AI, by which time app.PluginAdapters
// is fully populated (same as every other consumer in this codebase,
// e.g. Handler.NewHandler's `NewService(app.DB, app.Encryption,
// app.PluginAdapters)`). Building a throwaway *Service per call keeps
// a single source of truth for row resolution instead of introducing
// a second, late-bound setter.
type HostBridge struct {
	app *router.AppDeps

	// rateMu guards callLog, the per-plugin-slug sliding-window call
	// log backing the rate limit. One HostBridge is constructed once
	// at boot and lives for the process lifetime, so this state
	// persists across every plugin's calls.
	rateMu  sync.Mutex
	callLog map[string][]time.Time
}

// NewHostBridge builds the AICompleterSource wired into
// pluginruntime.HostServices.AI. Call it once at boot with the same
// *router.AppDeps whose PluginAdapters field the runtime being booted
// will populate.
func NewHostBridge(app *router.AppDeps) *HostBridge {
	return &HostBridge{app: app, callLog: make(map[string][]time.Time)}
}

// allowCall enforces the per-plugin rate limit (default 60/min,
// sliding window) AIHandle's doc comment promises. pluginSlug is
// host-supplied (core/pluginruntime's aiAdapter forwards the slug it
// was constructed with in buildHostContext) so a plugin can never
// inflate another plugin's quota or evade its own. An empty slug
// (a caller that bypasses the loader) is not rate limited — there is
// no identity to key a limit on — but every real plugin call is
// routed through aiAdapter, which always has one.
func (b *HostBridge) allowCall(pluginSlug string) error {
	if pluginSlug == "" {
		return nil
	}
	now := time.Now()
	cutoff := now.Add(-aiRateLimitWindow)

	b.rateMu.Lock()
	defer b.rateMu.Unlock()

	kept := b.callLog[pluginSlug][:0]
	for _, t := range b.callLog[pluginSlug] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= aiRateLimitPerMinute {
		b.callLog[pluginSlug] = kept
		return fmt.Errorf("ai rate limit exceeded for plugin %s: max %d completions per %s", pluginSlug, aiRateLimitPerMinute, aiRateLimitWindow)
	}
	b.callLog[pluginSlug] = append(kept, now)
	return nil
}

// service constructs a *Service bound to the current app.PluginAdapters
// value. Cheap: it holds no state of its own beyond the three
// references NewService stores.
func (b *HostBridge) service() *Service {
	return NewService(b.app.DB, b.app.Encryption, b.app.PluginAdapters)
}

// Available reports whether the host can currently dispatch a
// completion: at least one enabled ai_provider_connections row exists
// AND a plugin has registered an AIAdapterClient for that row's
// provider_type. A configured-but-unloaded row deliberately does not
// count — that produces an honest "not configured" instead of a
// runtime dispatch failure later.
func (b *HostBridge) Available() bool {
	svc := b.service()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conns, err := svc.List(ctx)
	if err != nil {
		return false
	}
	for _, c := range conns {
		if !c.Enabled {
			continue
		}
		if _, ok := svc.lookupAIAdapter(c.ProviderType); ok {
			return true
		}
	}
	return false
}

// Connections lists enabled AI connections a plugin may pin
// AIRequest.ConnectionID to. Deliberately never includes api_key or
// base_url — this list crosses into a plugin and then a browser, so
// it is treated as public from the moment it leaves the database.
func (b *HostBridge) Connections() []pluginruntime.AIConnectionInfo {
	svc := b.service()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conns, err := svc.List(ctx)
	if err != nil {
		return []pluginruntime.AIConnectionInfo{}
	}
	out := make([]pluginruntime.AIConnectionInfo, 0, len(conns))
	for _, c := range conns {
		if !c.Enabled {
			continue
		}
		out = append(out, pluginruntime.AIConnectionInfo{
			ID:           c.ID,
			Name:         c.Name,
			ProviderType: c.ProviderType,
			IsDefault:    c.IsDefault,
		})
	}
	return out
}

// Complete enforces the per-plugin rate limit (default 60/min, keyed
// by pluginSlug), resolves connectionID to a connection row (empty =
// the host's default/favourite; a named id that is missing, disabled,
// or unknown is a hard error — never a silent fallback to a different
// connection), and dispatches to whichever plugin registered an
// AIAdapterClient for that row's provider_type. This is the same
// dispatch primitive modules/ai_assistant's aiDispatchBridge /
// localAdapterCaller uses under the hood
// (plugin.AIAdapterClient.Complete(ctx, config, plugin.LLMRequest)) —
// reached here directly via the pre-existing
// Service.lookupAIAdapter, not duplicated as a second path.
func (b *HostBridge) Complete(ctx context.Context, pluginSlug, connectionID string, req pluginruntime.AICompletionRequest) (pluginruntime.AICompletionResponse, error) {
	if err := b.allowCall(pluginSlug); err != nil {
		return pluginruntime.AICompletionResponse{}, err
	}

	svc := b.service()

	row, cfg, err := svc.Resolve(ctx, connectionID)
	if err != nil {
		return pluginruntime.AICompletionResponse{}, fmt.Errorf("resolving ai connection: %w", err)
	}

	cli, ok := svc.lookupAIAdapter(row.ProviderType)
	if !ok {
		return pluginruntime.AICompletionResponse{}, fmt.Errorf("%w: provider_type %s", ErrPluginNotLoaded, row.ProviderType)
	}

	model := req.Model
	if model == "" {
		model = row.DefaultModel
	}

	messages := make([]plugin.LLMMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, plugin.LLMMessage{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		})
	}

	var temperature float32
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	var maxTokens int
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	sdkReq := plugin.LLMRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	// Enforce the AIHandle doc's documented per-call timeout default.
	// A caller-supplied ctx with a tighter deadline still wins, since
	// context.WithTimeout only ever shortens the effective deadline.
	callCtx, cancel := context.WithTimeout(ctx, aiCompletionTimeout)
	defer cancel()

	resp, err := cli.Complete(callCtx, cfg, sdkReq)
	if err != nil {
		// Never wrap or log cfg (it carries the decrypted api_key) —
		// only the provider_type and the error the plugin returned.
		return pluginruntime.AICompletionResponse{}, fmt.Errorf("ai provider %s completion failed: %w", row.ProviderType, err)
	}
	if resp == nil {
		return pluginruntime.AICompletionResponse{}, fmt.Errorf("ai provider %s returned an empty response", row.ProviderType)
	}

	return pluginruntime.AICompletionResponse{
		Content:  resp.Content,
		Model:    resp.Model,
		Provider: row.ProviderType,
		Usage: pluginruntime.AICompletionUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}, nil
}
