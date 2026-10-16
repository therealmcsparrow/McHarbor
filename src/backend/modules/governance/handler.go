// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package governance

import (
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the policy simulator and the conflict report.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler reads routes from the app's spec registry, which holds every gate
// exactly as it was mounted.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, svc: NewService(app.DB, app.RBACService, app.Spec.Operations)}
}

// Mount registers /governance/simulate and /governance/conflicts.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/governance", func(r *apispec.Router) {
			r.With(rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermPermissionsView, rbac.PermRbacPoliciesView)).
				Post("/simulate", h.HandleSimulate, apispec.Request(SimulateInput{}), apispec.Response(SimulateResult{}))
			r.With(rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermRolesView, rbac.PermRbacPoliciesView)).
				Get("/conflicts", h.HandleConflicts, apispec.Response([]rbac.ConflictWarning{}))
		})
	})
}

// HandleSimulate answers allow or deny, with the reasons, for another user.
func (h *Handler) HandleSimulate(w http.ResponseWriter, r *http.Request) {
	var in SimulateInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	out, err := h.svc.Simulate(r.Context(), in)
	switch {
	case errors.Is(err, ErrInvalidInput):
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	case errors.Is(err, ErrRouteNotFound):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	case err != nil:
		h.app.Logger.Error("governance: simulate", "error", err, "user", in.UserID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	// A simulation reads another user's access, so it is recorded like a read of
	// their permissions, not silently.
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "policy.simulate",
		Severity:      "info",
		EntityType:    "user",
		EntityID:      in.UserID,
		EntityName:    in.Permission + in.Method + " " + in.Path,
		EnvironmentID: in.EnvironmentID,
	})
	response.OK(w, out)
}

// HandleConflicts lists wildcard grants and shadowed ABAC policies.
func (h *Handler) HandleConflicts(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Conflicts(r.Context())
	if err != nil {
		h.app.Logger.Error("governance: conflicts", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, out)
}
