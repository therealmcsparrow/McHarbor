// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
)

// ctxKey is the unexported context key for the AI assistant user id.
type ctxKey struct{}

// withUserID stamps the authenticated user ID into the context so the
// service can scope conversations to the right user.
func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// userIDFromContext retrieves the user ID previously stamped by
// withUserID.
func userIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok && v != "" {
		return v
	}
	return "system"
}
