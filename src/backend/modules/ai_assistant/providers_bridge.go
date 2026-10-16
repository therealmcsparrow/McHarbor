// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"errors"
)

// Chat routing needs one thing from the AI-provider connections the operator
// configured: which provider type and model a connection id resolves to, plus
// the decrypted per-connection config the plugin adapter is called with. The
// ai_providers module owns those rows, so rather than importing it (a module
// must not depend on another module) this package declares the contract and
// main.go injects the bridge built in internal/bootstrap.

// ProviderConnection is the resolved routing target for one chat turn.
type ProviderConnection struct {
	ID           string
	Name         string
	ProviderType string
	BaseURL      string
	DefaultModel string
	Enabled      bool
}

// ProviderResolver resolves a connection id to its provider connection and
// the decrypted config map the plugin adapter expects. An empty id resolves
// to the operator's default connection.
type ProviderResolver interface {
	Resolve(ctx context.Context, connectionID string) (*ProviderConnection, map[string]string, error)
	// Default returns the operator's default connection, or nil when none
	// is configured. The AI kill switch reads it to decide whether the
	// assistant is available at all.
	Default(ctx context.Context) (*ProviderConnection, error)
}

// ErrProviderPluginNotLoaded is returned when a connection resolves to a
// provider type whose plugin adapter is not loaded, so the caller can tell
// "misconfigured" from "provider refused the request".
var ErrProviderPluginNotLoaded = errors.New("ai provider plugin not loaded")

// noProviderResolver is the fallback used when no bridge is injected: every
// resolve fails cleanly instead of nil-pointer panicking mid-chat.
type noProviderResolver struct{}

func (noProviderResolver) Resolve(context.Context, string) (*ProviderConnection, map[string]string, error) {
	return nil, nil, ErrProviderPluginNotLoaded
}

func (noProviderResolver) Default(context.Context) (*ProviderConnection, error) {
	return nil, nil
}
