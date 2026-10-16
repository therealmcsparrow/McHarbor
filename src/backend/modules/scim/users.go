// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// HandleUsers returns the SCIM 2.0 /Users collection. Supports the
// query parameters RFC 7644 §3.4.2 defines:
//
//	filter   SCIM filter expression
//	startIndex  1-based offset (default 1)
//	count       page size (default 25, max 100)
//
// Authentication has already been validated by BearerAuth — the
// endpoint that called us is in the request context.
func (h *Handler) HandleUsers(w http.ResponseWriter, r *http.Request) {
	startIndex := 1
	count := 25
	if v := r.URL.Query().Get("startIndex"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			startIndex = n
		}
	}
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			count = n
		}
	}
	if count > 100 {
		count = 100
	}

	filterStr := r.URL.Query().Get("filter")
	var fl *Filter
	if strings.TrimSpace(filterStr) != "" {
		parsed, err := Parse(filterStr)
		if err != nil {
			h.app.Logger.Error("scim: parse filter", "error", err, "filter", filterStr)
			response.BadRequestCode(w, r, i18nErrSCIMFilterInvalid())
			return
		}
		fl = parsed
	}

	users, total, err := h.loadUsersForSCIM(r, startIndex, count)
	if err != nil {
		h.app.Logger.Error("scim: list users", "error", err)
		response.InternalErrorCode(w, r, i18nErrInternalServer())
		return
	}

	// Apply filter in-memory. The dataset is bounded by `count` so
	// filtering after pagination is acceptable for SCIM 2.0 (RFC
	// 7644 §3.4.2.4 says totalResults reflects filtered count).
	filtered := make([]*User, 0, len(users))
	for _, u := range users {
		if fl == nil || fl.MatchUser(u) {
			filtered = append(filtered, u)
		}
	}

	resp := ListResponse[*User]{
		Schemas:      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		TotalResults: total,
		ItemsPerPage: len(filtered),
		StartIndex:   startIndex,
		Resources:    filtered,
	}
	writeSCIMJSON(w, http.StatusOK, resp)
}

// HandleUserByID returns a single SCIM user resource by id.
func (h *Handler) HandleUserByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/scim/v2/Users/")
	id = strings.Trim(id, "/")
	if id == "" {
		response.BadRequestCode(w, r, i18nErrInvalidBody())
		return
	}
	u, err := h.loadOneUserForSCIM(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18nErrNotFound())
		return
	}
	if err != nil {
		h.app.Logger.Error("scim: get user", "error", err)
		response.InternalErrorCode(w, r, i18nErrInternalServer())
		return
	}
	writeSCIMJSON(w, http.StatusOK, u)
}

// loadUsersForSCIM reads a page of users from the users table and
// wraps each row in a User. Columns we don't have (displayName,
// locale, timezone, title) are omitted from the JSON.
func (h *Handler) loadUsersForSCIM(r *http.Request, startIndex, count int) ([]*User, int, error) {
	ctx := r.Context()
	var total int
	if err := h.app.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id NOT IN ('system', '__legacy_owner__')`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := startIndex - 1
	rows, err := h.app.DB.QueryContext(ctx, `
		SELECT id, username, COALESCE(email, ''), COALESCE(display_name, ''),
		       '', '',
		       CASE WHEN is_active = 1 AND (disabled_at IS NULL OR disabled_at = '') THEN 1 ELSE 0 END,
		       created_at, updated_at
		  FROM users
		 WHERE id NOT IN ('system', '__legacy_owner__')
		 ORDER BY username
		 LIMIT ? OFFSET ?`, count, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var out []*User
	for rows.Next() {
		var (
			id, username, email, displayName string
			givenName, familyName            string
			active                           bool
			createdAt, updatedAt             string
		)
		if err := rows.Scan(&id, &username, &email, &displayName,
			&givenName, &familyName, &active, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, buildSCIMUser(id, username, email, givenName, familyName, active, createdAt, updatedAt))
	}
	return out, total, rows.Err()
}

func (h *Handler) loadOneUserForSCIM(r *http.Request, id string) (*User, error) {
	var (
		username, email, displayName string
		givenName, familyName        string
		active                       bool
		createdAt, updatedAt         string
	)
	err := h.app.DB.QueryRowContext(r.Context(), `
		SELECT username, COALESCE(email, ''), COALESCE(display_name, ''),
		       '', '',
		       CASE WHEN is_active = 1 AND (disabled_at IS NULL OR disabled_at = '') THEN 1 ELSE 0 END,
		       created_at, updated_at
		  FROM users WHERE id = ? AND id NOT IN ('system', '__legacy_owner__')`, id,
	).Scan(&username, &email, &displayName, &givenName, &familyName, &active, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return buildSCIMUser(id, username, email, givenName, familyName, active, createdAt, updatedAt), nil
}

// buildSCIMUser constructs the resource envelope, including the
// RFC 7643 Meta block with location/version derived from id.
func buildSCIMUser(id, username, email, given, family string, active bool, createdAt, updatedAt string) *User {
	u := &User{
		Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:       id,
		UserName: username,
		Active:   active,
		Name: &Name{
			GivenName:  given,
			FamilyName: family,
		},
		Meta: &Meta{
			ResourceType: "User",
			Created:      createdAt,
			LastModified: updatedAt,
			Location:     fmt.Sprintf("/api/scim/v2/Users/%s", id),
			Version:      updatedAt,
		},
	}
	if email != "" {
		u.Emails = []Email{{Value: email, Primary: true, Type: "work"}}
	}
	return u
}

// (writeSCIMJSON lives in protocol.go)
