// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Scope is the user-driven context every tool call carries. The
// ConversationID is empty for one-shot (skill / curl) calls.
type Scope struct {
	UserID         string `json:"userId"`
	EnvID          string `json:"envId,omitempty"`
	StackName      string `json:"stackName,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	DryRun         bool   `json:"-"`
}

// Tool is a single LLM-callable action. Tools are registered at
// startup and exposed to the model via the per-user filtered catalog.
type Tool struct {
	// Name uniquely identifies the tool. Format: "<module>.<verb>",
	// e.g. "containers.list", "audit.search".
	Name string

	// Description is shown to the model in the system prompt.
	Description string

	// InputSchema is a JSON Schema object describing the arguments
	// shape. Stored as map[string]any so we don't pull in a JSON-Schema
	// validator dependency; the LLM sees the schema verbatim.
	InputSchema map[string]any

	// Required permissions; all must hold. Empty = available to any
	// authenticated user. Wildcard permission is checked first.
	Required []rbac.Permission

	// ReadOnly is true for tools that never mutate state. The action
	// plane (Phase 4) skips the consent dialog when this is true.
	ReadOnly bool

	// RequiresConsent forces the action plane to show the consent
	// dialog regardless of ReadOnly. Default false.
	RequiresConsent bool

	// MaxBytes is the truncation budget for the handler output. Tools
	// returning more than this are wrapped in a truncated envelope
	// with a `next_tool` hint.
	MaxBytes int

	// Timeout caps the handler execution. Defaults to 30s.
	Timeout time.Duration

	// Handler executes the tool. Returns the value (will be
	// JSON-serialised and redacted). Errors map to JSON-RPC codes
	// (ErrNotFound → -32003, ErrForbidden → -32002, etc.).
	Handler func(ctx context.Context, sc Scope, args json.RawMessage) (any, error)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]*Tool)
)

// Register adds a tool to the global registry. Tools are wired in init()
// blocks, so an invalid or duplicate registration is a programming error;
// it is logged at startup and the offending tool is skipped rather than
// crashing the process.
func Register(t *Tool) {
	if t == nil || t.Name == "" {
		slog.Error("ai_tools: tool must have a Name; registration skipped")
		return
	}
	if t.Handler == nil {
		slog.Error("ai_tools: tool has no Handler; registration skipped", "tool", t.Name)
		return
	}
	if t.Timeout == 0 {
		t.Timeout = 30 * time.Second
	}
	if t.MaxBytes == 0 {
		t.MaxBytes = 64 * 1024
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[t.Name]; exists {
		slog.Error("ai_tools: duplicate tool registration ignored", "tool", t.Name)
		return
	}
	registry[t.Name] = t
}

// Get returns a tool by name.
func Get(name string) (*Tool, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	t, ok := registry[name]
	return t, ok
}

// All returns every registered tool. Callers that need a per-user
// filtered view should use CatalogFor.
func All() []*Tool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]*Tool, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	return out
}

// Names returns all registered tool names in sorted order. Used by
// tests and admin tooling.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}

// ResetForTest clears the registry. Test-only.
func ResetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = make(map[string]*Tool)
}
