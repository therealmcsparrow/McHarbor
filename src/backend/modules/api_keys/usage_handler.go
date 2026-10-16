// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleDetail returns one key's full access picture: assigned roles and what
// each grants, explicit grants, the configured union, and what the key can
// actually do once the owner's rights are applied.
func (h *Handler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	detail, ok := h.loadAccessibleDetail(w, r, user, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	response.OK(w, detail)
}

// HandleUsageSeries returns the request-count graph plus its summary.
func (h *Handler) HandleUsageSeries(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	if _, ok := h.loadAccessibleDetail(w, r, user, id); !ok {
		return
	}

	q := r.URL.Query()
	from, to, err := ParseUsageWindow(q.Get("range"), q.Get("from"), q.Get("to"))
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrAPIKeyRangeInvalid, err)
		return
	}

	series, err := h.svc.UsageSeries(r.Context(), id, from, to)
	if err != nil {
		h.app.Logger.Error("api_keys: usage series error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyUsageFailed, err)
		return
	}

	response.OK(w, series)
}

// HandleUsageEvents returns one page of the request history.
func (h *Handler) HandleUsageEvents(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	if _, ok := h.loadAccessibleDetail(w, r, user, id); !ok {
		return
	}

	page, perPage := response.ParsePagination(r)
	q := r.URL.Query()

	filter := UsageEventFilter{
		Page:          page,
		PerPage:       perPage,
		Method:        strings.ToUpper(strings.TrimSpace(q.Get("method"))),
		StatusClass:   strings.TrimSpace(q.Get("status_class")),
		AuthOutcome:   strings.TrimSpace(q.Get("auth_outcome")),
		EnvironmentID: strings.TrimSpace(q.Get("environment_id")),
	}

	// Bound every free-text filter before it reaches SQL. The values are
	// parameterised anyway; this stops a caller wasting the query planner's
	// time with a megabyte of nonsense.
	if len(filter.Method) > 16 || len(filter.StatusClass) > 8 ||
		len(filter.AuthOutcome) > 32 || len(filter.EnvironmentID) > 64 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if q.Has("from") || q.Has("to") {
		from, to, err := ParseUsageWindow("", q.Get("from"), q.Get("to"))
		if err != nil {
			response.BadRequestCause(w, r, i18n.ErrAPIKeyRangeInvalid, err)
			return
		}
		filter.From, filter.To = from, to
	}

	events, total, err := h.svc.UsageEvents(r.Context(), id, filter)
	if err != nil {
		h.app.Logger.Error("api_keys: usage events error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyUsageFailed, err)
		return
	}

	response.Paginated(w, events, total, page, perPage)
}

// HandleClearUsage destroys all usage history and every deleted-key
// tombstone. Gated by settings.manage, not api_keys.manage: this is a
// data-retention decision about the whole install, not an action on one key.
//
// It deliberately leaves live and revoked credentials alone. An operator
// clearing history is saying "forget what happened", not "break my
// automation".
func (h *Handler) HandleClearUsage(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	result, err := h.svc.ClearUsage(r.Context())
	if err != nil {
		h.app.Logger.Error("api_keys: clear usage error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyUsageClearFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "purge",
		EntityType: "api_key_usage",
		Details: fmt.Sprintf(
			"cleared %d api key usage events and %d deleted-key tombstones",
			result.EventsDeleted, result.TombstonesDeleted,
		),
	})

	response.OK(w, result)
}

// loadAccessibleDetail is the ownership rule for every read surface, live or
// tombstoned: the owner always passes, anyone else needs api_keys.view.
//
// It exists separately from loadAccessible because that one reads api_keys
// only and would 404 a tombstone, taking the detail page and its history
// offline the moment a key is deleted — exactly when the history matters.
func (h *Handler) loadAccessibleDetail(w http.ResponseWriter, r *http.Request, user *auth.User, id string) (*APIKeyDetail, bool) {
	detail, err := h.svc.Detail(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("api_keys: detail error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return nil, false
	}
	if detail == nil {
		response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
		return nil, false
	}
	if detail.UserID == user.ID {
		return detail, true
	}

	viewAll, err := h.svc.CanViewAll(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("api_keys: view permission check failed", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return nil, false
	}
	if !viewAll {
		response.ForbiddenCode(w, r, i18n.ErrAPIKeyForbidden)
		return nil, false
	}
	return detail, true
}
