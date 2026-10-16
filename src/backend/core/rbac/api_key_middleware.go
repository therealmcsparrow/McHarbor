// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apikeyusage"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

const apiKeyPrefix = "mch_"

// UsageRecording carries the optional telemetry hooks for the Bearer
// middleware. Both fields may be nil, in which case nothing is recorded.
//
// It exists because a rejection is the one event the usage middleware can
// never see: this middleware answers 401 and the request never reaches it.
// A revoked key still hammering an endpoint is precisely what an operator
// wants in the history, so those attempts are recorded here instead.
type UsageRecording struct {
	Sink           apikeyusage.Sink
	ResolveAddress apikeyusage.AddressResolverFunc
}

// record emits one attributable rejection. Unknown tokens never reach it:
// without a key id there is nothing to attribute the attempt to, and
// inventing a bucket for them would be a table of noise.
func (u *UsageRecording) record(r *http.Request, keyID string, outcome apikeyusage.Outcome, started time.Time) {
	if u == nil || u.Sink == nil || keyID == "" {
		return
	}

	var peerIP, clientIP, source string
	if u.ResolveAddress != nil {
		peerIP, clientIP, source = u.ResolveAddress(r)
	}
	if source == "" {
		source = apikeyusage.SourcePeer
	}

	u.Sink.Record(apikeyusage.Event{
		APIKeyID:       keyID,
		OccurredAt:     started.UTC(),
		Method:         r.Method,
		RoutePattern:   apikeyusage.RoutePatternOf(r),
		StatusCode:     http.StatusUnauthorized,
		Duration:       time.Since(started),
		Outcome:        outcome,
		EnvironmentID:  r.URL.Query().Get("env"),
		PeerIP:         peerIP,
		ClientIP:       clientIP,
		ClientIPSource: source,
		UserAgent:      apikeyusage.SanitizeUserAgent(r.UserAgent()),
	})
}

// APIKeyMiddleware checks for Bearer token auth via API keys.
// If no Bearer header is present, it falls through to the next handler
// (which should be session-based auth middleware).
func APIKeyMiddleware(db *sql.DB, authSvc *auth.Service, usage *UsageRecording) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
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

			var keyID, userID, expiresAt, scopesJSON string
			var isActive bool
			var expiresAtNull sql.NullString
			var keyName string

			err := db.QueryRowContext(r.Context(),
				`SELECT id, user_id, expires_at, is_active, scopes, name FROM api_keys WHERE key_hash = ?`,
				keyHash,
			).Scan(&keyID, &userID, &expiresAtNull, &isActive, &scopesJSON, &keyName)

			if errors.Is(err, sql.ErrNoRows) {
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyInvalid)
				return
			}
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}

			if !isActive {
				usage.record(r, keyID, apikeyusage.OutcomeRevoked, started)
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyRevoked)
				return
			}

			// Check expiration
			if expiresAtNull.Valid {
				expiresAt = expiresAtNull.String
				expires, parseErr := time.Parse(time.RFC3339, expiresAt)
				if parseErr == nil && time.Now().UTC().After(expires) {
					usage.record(r, keyID, apikeyusage.OutcomeExpired, started)
					response.UnauthorizedCode(w, r, i18n.ErrAPIKeyExpired)
					return
				}
			}

			updateCtx, cancel := context.WithTimeout(r.Context(), authzQueryTimeout)
			now := time.Now().UTC().Format(time.RFC3339)
			if _, updateErr := db.ExecContext(updateCtx, "UPDATE api_keys SET last_used_at = ? WHERE id = ?", now, keyID); updateErr != nil {
				slog.Warn("api key middleware: updating last_used_at failed", "keyID", keyID, "error", updateErr)
			}
			cancel()

			// Load the user
			user, err := authSvc.ValidateUserByIDContext(r.Context(), userID)
			if err != nil || user == nil {
				usage.record(r, keyID, apikeyusage.OutcomeOwnerInvalid, started)
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyInvalid)
				return
			}

			principal, err := loadAPIKeyPrincipal(r.Context(), db, keyID, scopesJSON)
			if err != nil {
				slog.Error("api key middleware: loading principal failed", "keyID", keyID, "error", err)
				response.UnauthorizedCode(w, r, i18n.ErrAPIKeyInvalid)
				return
			}
			principal.Name = keyName

			ctx := context.WithValue(r.Context(), auth.UserContextKey, user)
			ctx = WithAPIKey(ctx, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// loadAPIKeyPrincipal reads the key's roles, environment fence, and
// explicit scopes. It is the only place that turns storage into the
// principal the RBAC service consumes.
func loadAPIKeyPrincipal(ctx context.Context, db *sql.DB, keyID, scopesJSON string) (*APIKeyPrincipal, error) {
	queryCtx, cancel := context.WithTimeout(ctx, authzQueryTimeout)
	defer cancel()

	p := &APIKeyPrincipal{KeyID: keyID, RoleIDs: []string{}, Scopes: []Permission{}, EnvironmentIDs: []string{}}

	var rawScopes []string
	if scopesJSON != "" {
		if err := json.Unmarshal([]byte(scopesJSON), &rawScopes); err != nil {
			return nil, fmt.Errorf("parsing api key scopes: %w", err)
		}
	}
	for _, scope := range rawScopes {
		p.Scopes = append(p.Scopes, Permission(scope))
	}

	roleRows, err := db.QueryContext(queryCtx, `SELECT role_id FROM api_key_roles WHERE api_key_id = ? LIMIT 1000`, keyID)
	if err != nil {
		return nil, fmt.Errorf("querying api key roles: %w", err)
	}
	defer closeRows(roleRows)
	for roleRows.Next() {
		var roleID string
		if err := roleRows.Scan(&roleID); err != nil {
			return nil, fmt.Errorf("scanning api key role: %w", err)
		}
		p.RoleIDs = append(p.RoleIDs, roleID)
	}
	if err := roleRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api key roles: %w", err)
	}

	envRows, err := db.QueryContext(queryCtx, `SELECT environment_id FROM api_key_environments WHERE api_key_id = ? LIMIT 1000`, keyID)
	if err != nil {
		return nil, fmt.Errorf("querying api key environments: %w", err)
	}
	defer closeRows(envRows)
	for envRows.Next() {
		var envID string
		if err := envRows.Scan(&envID); err != nil {
			return nil, fmt.Errorf("scanning api key environment: %w", err)
		}
		p.EnvironmentIDs = append(p.EnvironmentIDs, envID)
	}
	if err := envRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api key environments: %w", err)
	}

	return p, nil
}
