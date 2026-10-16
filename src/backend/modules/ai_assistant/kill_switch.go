// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// ErrAIDisabled is the in-process error returned by RequireAIEnabled
// when no enabled default provider connection exists. The
// HTTP middleware maps it to 503 + i18n.ErrAIDisabled. We keep the
// Go-level sentinel so callers (and tests) can branch on it without
// reaching into the i18n layer.
var ErrAIDisabled = errors.New("ai assistant disabled")

// AIEnabledChecker is the minimal surface RequireAIEnabled needs
// from the service. We define it as an interface so the middleware
// can be tested with a stub checker (and so we don't have to drag
// the full *Service dependency into the route wiring).
type AIEnabledChecker interface {
	IsAIEnabled(ctx context.Context) (bool, error)
}

// IsAIEnabled returns true when an enabled default provider connection
// exists. Provider connections are the host-owned runtime switch; the
// removed ai_assistant_settings singleton is no longer consulted.
func (s *Service) IsAIEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.providers == nil {
		return false, nil
	}
	connection, err := s.providers.Default(ctx)
	if err != nil {
		return false, err
	}
	if connection == nil || !connection.Enabled {
		return false, nil
	}
	runtime, ok := s.pluginRuntime.(aiPluginRuntime)
	if !ok || runtime == nil {
		return false, nil
	}
	adapter, _, loaded := runtime.AIAdapter(connection.ProviderType)
	return loaded && adapter != nil, nil
}

// RequireAIEnabled returns middleware that short-circuits requests
// with 503 + i18n.ErrAIDisabled when the AI assistant is disabled
// at the global level. Per §9.6 of the AI integration spec, this is
// the hard kill switch: setting `enabled = 0` (or never having a
// settings row) immediately stops the chat / tools / action routes
// from serving traffic. The route carries an i18n-translated error
// so the UI can render a clear "AI assistant is disabled" message
// instead of a generic failure.
//
// The middleware is intentionally fail-closed: any failure to read
// the settings row is treated as "disabled". Admin routes (`/settings`,
// `/providers`, OAuth) intentionally do NOT mount this middleware
// so the admin can still flip the toggle back on.
func RequireAIEnabled(checker AIEnabledChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if checker == nil {
				response.ErrCode(w, r, http.StatusServiceUnavailable, i18n.ErrAIDisabled)
				return
			}
			enabled, err := checker.IsAIEnabled(r.Context())
			if err != nil {
				// Log via the standard slog if available; the response
				// helper already covers the i18n code. We don't
				// surface the raw error to the client.
				_ = err
			}
			if !enabled {
				response.ErrCode(w, r, http.StatusServiceUnavailable, i18n.ErrAIDisabled)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
