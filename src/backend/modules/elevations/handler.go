// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package elevations

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/pushmsg"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler is the HTTP layer for elevation requests.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler creates the elevations HTTP handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, svc: NewService(app.DB)}
}

// Mount registers elevation request routes. Phase 6 of the RBAC
// roadmap. Lives in handler.go so all the HTTP code is in one place.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/elevations", func(r *apispec.Router) {
			// The requester creates their own request. No
			// roles.manage required — the user is asking for
			// something they don't have.
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/", h.HandleCreate, apispec.Request(CreateRequest{}))

			// The requester can see their own queue; admins can see
			// every pending request for the inbox.
			r.Get("/mine", h.HandleListMine)
			r.Get("/elevatable", h.HandleElevatable)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Put("/elevatable", h.HandleSetElevatable, apispec.Request(SetElevatableRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Get("/pending", h.HandleListPending)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Get("/all", h.HandleListAll)

			// Approve or deny. The requester can also withdraw their
			// own pending request (just calls delete on it).
			r.Route("/{id}", func(r *apispec.Router) {
				r.Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Post("/decide", h.HandleDecide, apispec.Request(DecideRequest{}))
				r.With(rbac.RequireSelfService(app.RBACService)).Delete("/", h.HandleWithdraw)
			})
		})
	})
}

// HandleCreate validates and inserts a new elevation request.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	requester := auth.RequireAuth(r)
	if requester == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var in CreateRequest
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if in.PermissionKey == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if in.RequestedMinutes <= 0 {
		in.RequestedMinutes = 60
	}

	row, err := h.svc.Create(r.Context(), requester.ID, in)
	if err != nil {
		if errors.Is(err, ErrNotElevatable) {
			response.BadRequestCode(w, r, i18n.ErrPermissionNotElevatable)
			return
		}
		if errors.Is(err, ErrUnknownPermission) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		if errors.Is(err, ErrTooManyPending) || errors.Is(err, ErrAlreadyPending) {
			reason := "too_many_pending"
			if errors.Is(err, ErrAlreadyPending) {
				reason = "already_pending"
			}
			response.ErrCodeWithData(w, r, http.StatusTooManyRequests, i18n.ErrRateLimitExceed, map[string]string{"reason": reason})
			return
		}
		h.app.Logger.Error("elevations: create", "error", err, "user", requester.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "elevation",
		EntityID:   row.ID,
		EntityName: row.PermissionKey,
		Details:    "permission=" + in.PermissionKey + " requested_minutes=" + fmt.Sprint(in.RequestedMinutes),
	})
	h.notifyApprovers(requester.Username, &row)

	response.Created(w, row)
}

// HandleElevatable lists every permission with whether it can be requested, so
// the request form offers exactly what Create will accept.
func (h *Handler) HandleElevatable(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	entries, err := h.svc.Elevatable(r.Context())
	if err != nil {
		h.app.Logger.Error("elevations: elevatable catalog", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, entries)
}

// HandleSetElevatable records an admin's decision on whether one key may be
// requested.
func (h *Handler) HandleSetElevatable(w http.ResponseWriter, r *http.Request) {
	actor := auth.RequireAuth(r)
	if actor == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in SetElevatableRequest
	if err := response.DecodeBody(r, &in); err != nil || in.Key == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.svc.SetElevatable(r.Context(), in.Key, in.Elevatable, actor.ID); err != nil {
		if errors.Is(err, ErrUnknownPermission) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		h.app.Logger.Error("elevations: set elevatable", "error", err, "key", in.Key)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "elevation_policy",
		EntityID:   in.Key,
		EntityName: in.Key,
		Details:    "elevatable=" + strconv.FormatBool(in.Elevatable),
	})
	response.OK(w, map[string]any{"key": in.Key, "elevatable": in.Elevatable})
}

// pushNotifier is the part of the push service a new request needs.
type pushNotifier interface {
	Notify(n pushmsg.Notification, aud pushmsg.Audience)
}

// notifyApprovers tells everyone who may decide the request, because an
// approver away from a desk is why requests sit for hours.
func (h *Handler) notifyApprovers(requester string, row *ElevationRequest) {
	svc, ok := h.app.LookupService("push")
	if !ok {
		return
	}
	notifier, ok := svc.(pushNotifier)
	if !ok || row == nil {
		return
	}
	notifier.Notify(pushmsg.Notification{
		Title:    "Elevation requested",
		Body:     fmt.Sprintf("%s asks for %s for %d minutes", requester, row.PermissionKey, row.RequestedMinutes),
		Severity: "warning",
		URL:      "/oncall/approvals",
		Tag:      "elevation-" + row.ID,
		Key:      pushmsg.MsgElevationRequested,
		Args: map[string]string{
			"user": requester, "permission": row.PermissionKey, "minutes": strconv.Itoa(row.RequestedMinutes),
		},
	}, pushmsg.Audience{Permission: rbac.PermUsersManage})
}

// HandleListMine returns the requester's own queue.
func (h *Handler) HandleListMine(w http.ResponseWriter, r *http.Request) {
	requester := auth.RequireAuth(r)
	if requester == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	// Cheap pre-read so the UI sees the right state immediately.
	if err := h.svc.SweepExpired(r.Context()); err != nil {
		h.app.Logger.Warn("elevations: sweep expired", "error", err)
	}

	items, err := h.svc.ListMine(r.Context(), requester.ID)
	if err != nil {
		h.app.Logger.Error("elevations: list mine", "error", err, "user", requester.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]any{"requests": items})
}

// HandleListPending returns the admin inbox.
func (h *Handler) HandleListPending(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.SweepExpired(r.Context()); err != nil {
		h.app.Logger.Warn("elevations: sweep expired", "error", err)
	}
	items, err := h.svc.ListPending(r.Context())
	if err != nil {
		h.app.Logger.Error("elevations: list pending", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]any{"requests": items})
}

// HandleListAll returns every request (admin only — gated by route).
func (h *Handler) HandleListAll(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.SweepExpired(r.Context()); err != nil {
		h.app.Logger.Warn("elevations: sweep expired", "error", err)
	}
	items, err := h.svc.ListAll(r.Context())
	if err != nil {
		h.app.Logger.Error("elevations: list all", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]any{"requests": items})
}

// HandleGet returns a single request.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	caller := auth.RequireAuth(r)
	if caller == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	row, err := h.svc.ByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("elevations: get", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	// Only the requester and those who could decide it may read a request; for
	// anyone else it does not exist, so its permission and reason stay private.
	if row.RequesterID != caller.ID {
		allowed, err := h.app.RBACService.AllowedOn(r, "", rbac.PermUsersManage)
		if err != nil {
			h.app.Logger.Error("elevations: checking the reader", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if !allowed {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
	}
	response.OK(w, row)
}

// HandleDecide approves or denies a pending request. Phase 6.
func (h *Handler) HandleDecide(w http.ResponseWriter, r *http.Request) {
	approver := auth.RequireAuth(r)
	if approver == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var in DecideRequest
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	row, err := h.svc.Decide(r.Context(), chi.URLParam(r, "id"), approver.ID, in.Approve, in.Note)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.NotFoundCode(w, r, i18n.ErrNotFound)
		case errors.Is(err, ErrNotPending):
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		case errors.Is(err, ErrSelfApproval):
			response.ForbiddenCode(w, r, i18n.ErrForbidden)
		default:
			h.app.Logger.Error("elevations: decide", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "decide",
		EntityType: "elevation",
		EntityID:   row.ID,
		EntityName: row.PermissionKey,
		Details: fmt.Sprintf("decision=%v approver=%s",
			map[bool]string{true: "approve", false: "deny"}[in.Approve], approver.ID),
	})

	response.OK(w, row)
}

// HandleWithdraw lets the requester cancel their own pending
// request before an admin acts on it.
func (h *Handler) HandleWithdraw(w http.ResponseWriter, r *http.Request) {
	requester := auth.RequireAuth(r)
	if requester == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	// Read existing; only the requester can withdraw, and only if pending.
	row, err := h.svc.ByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("elevations: withdraw get", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if row.RequesterID != requester.ID {
		response.ForbiddenCode(w, r, i18n.ErrPermissionDenied)
		return
	}
	if row.Status != StatusPending {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Cheap approach: flip to "denied" with a sentinel note so we
	// can reuse the existing decide() write path. A future schema
	// migration can introduce a real "withdrawn" status if needed.
	if _, err := h.svc.Decide(r.Context(), id, requester.ID, false, "withdrawn by requester"); err != nil {
		h.app.Logger.Error("elevations: withdraw", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "withdraw",
		EntityType: "elevation",
		EntityID:   id,
		EntityName: row.PermissionKey,
		Details:    "withdrawn by requester",
	})
	response.OK(w, map[string]any{"id": id, "status": StatusDenied})
}
