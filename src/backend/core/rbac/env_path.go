// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// PinEnvToPath makes the environment named by the route parameter param the
// request's `?env=` scope, so a grant on one environment cannot be presented to act
// on another. Mount it with Use on any route group whose path names an environment.
//
// It overrides rather than rejects a different `?env=`: the frontend's envUrl may
// append the currently selected environment, and the path is what the handler acts on.
func PinEnvToPath(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := next.(*Probe); ok {
			p.pinEnv(param)
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			envID := chi.URLParam(r, param)
			if envID == "" {
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}
			q := r.URL.Query()
			q.Set("env", envID)
			u := *r.URL
			u.RawQuery = q.Encode()
			pinned := r.WithContext(r.Context())
			pinned.URL = &u
			next.ServeHTTP(w, pinned)
		})
	}
}

// EnvOfRow runs a module's single-row environment query for id, the shared half
// of every EnvResolver. query selects one column, COALESCE'd to ” for a global
// row; no row means found is false.
func EnvOfRow(ctx context.Context, db *sql.DB, query, id string) (envID string, found bool, err error) {
	err = db.QueryRowContext(ctx, query, id).Scan(&envID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolving the row's environment: %w", err)
	}
	return envID, true, nil
}

// EnvResolver reports the environment the row a request addresses lives in:
// envID "" for a global row, found false when there is no such row.
type EnvResolver func(r *http.Request) (envID string, found bool, err error)

// PinEnvToResource makes the addressed row's own environment the request's
// `?env=` scope, so the gates below it decide on where the row really lives
// rather than on whichever environment the caller named. A global row clears
// `?env=`, which leaves only a global grant able to pass. A missing row passes
// through untouched for the handler to answer 404.
func PinEnvToResource(resolve EnvResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := next.(*Probe); ok {
			p.pinEnv("resource")
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			envID, found, err := resolve(r)
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !found {
				next.ServeHTTP(w, r)
				return
			}
			q := r.URL.Query()
			if envID == "" {
				q.Del("env")
			} else {
				q.Set("env", envID)
			}
			u := *r.URL
			u.RawQuery = q.Encode()
			pinned := r.WithContext(r.Context())
			pinned.URL = &u
			next.ServeHTTP(w, pinned)
		})
	}
}
