// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// HandleGroups returns the SCIM 2.0 /Groups collection. McHarbor
// roles map onto SCIM groups (members of the same role share the
// same set of permissions).
func (h *Handler) HandleGroups(w http.ResponseWriter, r *http.Request) {
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

	groups, total, err := h.loadGroupsForSCIM(r.Context(), startIndex, count)
	if err != nil {
		h.app.Logger.Error("scim: list groups", "error", err)
		response.InternalErrorCode(w, r, i18nErrInternalServer())
		return
	}
	resp := ListResponse[*Group]{
		Schemas:      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		TotalResults: total,
		ItemsPerPage: len(groups),
		StartIndex:   startIndex,
		Resources:    groups,
	}
	writeSCIMJSON(w, http.StatusOK, resp)
}

// HandleGroupByID returns a single SCIM group by id (role id).
func (h *Handler) HandleGroupByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/scim/v2/Groups/")
	id = strings.Trim(id, "/")
	if id == "" {
		response.BadRequestCode(w, r, i18nErrInvalidBody())
		return
	}
	g, err := h.loadOneGroupForSCIM(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18nErrNotFound())
		return
	}
	if err != nil {
		h.app.Logger.Error("scim: get group", "error", err)
		response.InternalErrorCode(w, r, i18nErrInternalServer())
		return
	}
	writeSCIMJSON(w, http.StatusOK, g)
}

func (h *Handler) loadGroupsForSCIM(ctx context.Context, startIndex, count int) ([]*Group, int, error) {
	var total int
	if err := h.app.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := startIndex - 1

	groupRows, err := h.app.DB.QueryContext(ctx, `
		SELECT id, name, COALESCE(description, ''), created_at, updated_at
		  FROM roles
		 ORDER BY name
		 LIMIT ? OFFSET ?`, count, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { closeutil.Report(groupRows.Close(), "groupRows") }()

	var out []*Group
	for groupRows.Next() {
		var (
			id, name, description, createdAt, updatedAt string
		)
		if err := groupRows.Scan(&id, &name, &description, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		members, err := h.loadGroupMembers(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, buildSCIMGroup(id, name, description, members, createdAt, updatedAt))
	}
	return out, total, groupRows.Err()
}

func (h *Handler) loadOneGroupForSCIM(ctx context.Context, id string) (*Group, error) {
	var name, description, createdAt, updatedAt string
	err := h.app.DB.QueryRowContext(ctx, `
		SELECT name, COALESCE(description, ''), created_at, updated_at
		  FROM roles WHERE id = ?`, id,
	).Scan(&name, &description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	members, err := h.loadGroupMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildSCIMGroup(id, name, description, members, createdAt, updatedAt), nil
}

// loadGroupMembers resolves the user ids in user_roles for the
// supplied role id. SCIM groups expose a `members` array of
// `{value, display}` objects.
func (h *Handler) loadGroupMembers(ctx context.Context, roleID string) ([]Member, error) {
	rows, err := h.app.DB.QueryContext(ctx, `
		SELECT u.id, u.username
		  FROM users u
		  JOIN user_roles ur ON ur.user_id = u.id
		 WHERE ur.role_id = ?
		 GROUP BY u.id, u.username
		 LIMIT 1000`, roleID)
	if err != nil {
		return nil, err
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []Member
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, Member{Value: id, Display: name})
	}
	return out, rows.Err()
}

func buildSCIMGroup(id, name, description string, members []Member, createdAt, updatedAt string) *Group {
	return &Group{
		Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		ID:          id,
		DisplayName: name,
		Description: description,
		Members:     members,
		Meta: &Meta{
			ResourceType: "Group",
			Created:      createdAt,
			LastModified: updatedAt,
			Location:     "/api/scim/v2/Groups/" + id,
			Version:      updatedAt,
		},
	}
}
