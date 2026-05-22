// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

const apiKeyPrefix = "mch_"

// APIKeyMiddleware checks for Bearer token auth via API keys.
// If no Bearer header is present, it falls through to the next handler
// (which should be session-based auth middleware).
func APIKeyMiddleware(db *sql.DB, authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				// No Bearer header — fall through to session auth
				next.ServeHTTP(w, r)
				return
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if !strings.HasPrefix(token, apiKeyPrefix) {
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyInvalid)
				return
			}

			// Hash the token for lookup
			hash := sha256.Sum256([]byte(token))
			keyHash := hex.EncodeToString(hash[:])

			var keyID, userID, expiresAt string
			var isActive bool
			var expiresAtNull sql.NullString

			err := db.QueryRow(
				`SELECT id, user_id, expires_at, is_active FROM api_keys WHERE key_hash = ?`,
				keyHash,
			).Scan(&keyID, &userID, &expiresAtNull, &isActive)

			if err == sql.ErrNoRows {
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyInvalid)
				return
			}
			if err != nil {
				response.InternalErrorCode(w, r, i18n.ErrInternalServer)
				return
			}

			if !isActive {
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyRevoked)
				return
			}

			// Check expiration
			if expiresAtNull.Valid {
				expiresAt = expiresAtNull.String
				expires, parseErr := time.Parse(time.RFC3339, expiresAt)
				if parseErr == nil && time.Now().UTC().After(expires) {
					response.UnauthorizedCode(w, r, i18n.ErrAPIKeyExpired)
					return
				}
			}

			// Update last_used_at in background
			go func() {
				now := time.Now().UTC().Format(time.RFC3339)
				db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", now, keyID)
			}()

			// Load the user
			user, err := authSvc.ValidateUserByID(userID)
			if err != nil || user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyInvalid)
				return
			}

			ctx := context.WithValue(r.Context(), auth.UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
