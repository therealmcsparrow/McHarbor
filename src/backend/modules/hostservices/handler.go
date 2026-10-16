// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfhost"
)

// Handler serves /host-services and /host-services/self.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler builds the handler over the app's agent pool and database, and
// self, the self-host runner the system module uses for host updates.
func NewHandler(app *router.AppDeps, self selfhost.Runner) *Handler {
	return &Handler{app: app, svc: NewService(app.DB, app.AgentPool, self)}
}

// targetLabel names the host in a log line.
func targetLabel(target Target) string {
	if target.Self {
		return "self"
	}
	return target.EnvID
}

// fail writes the response for a service error: a Refusal becomes its own
// code, with systemd's reason as data when there is one; anything else is
// logged and answered with fallback.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, target Target, op string, err error, fallback i18n.MsgCode) {
	var refusal *Refusal
	switch {
	case errors.As(err, &refusal):
		h.app.Logger.Warn("host services: refused", "env", targetLabel(target), "op", op, "code", refusal.Code, "detail", refusal.Detail)
		if refusal.Detail != "" {
			writeRefusalWithDetail(w, r, refusal)
			return
		}
		response.ErrCode(w, r, refusal.Status, refusal.Code)
	case errors.Is(err, ErrEnvironmentNotFound):
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
	default:
		h.app.Logger.Error("host services: failed", "env", targetLabel(target), "op", op, "error", err)
		response.InternalErrorCause(w, r, fallback, err)
	}
}

// writeRefusalWithDetail answers with the translated code followed by what
// systemd said. The detail is the host's own refusal, deliberately forwarded:
// "systemd refused the action" alone leaves the operator reading server logs,
// and every client — the tab's dialog, the CLI — shows the error text as is.
func writeRefusalWithDetail(w http.ResponseWriter, r *http.Request, refusal *Refusal) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(refusal.Status)
	body := response.APIResponse{
		Success: false,
		Error:   i18n.T(i18n.FromContext(r.Context()), refusal.Code) + ": " + refusal.Detail,
		Code:    string(refusal.Code),
		Data:    ErrorDetail{Detail: refusal.Detail},
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Default().Debug("host services: writing the refusal failed", "error", err)
	}
}

// outcome names how a mutation ended, for the audit row.
func outcome(err error) string {
	if err == nil {
		return "ok"
	}
	var refusal *Refusal
	if errors.As(err, &refusal) {
		return "refused (" + string(refusal.Code) + ")"
	}
	return "failed"
}

// audit records a mutation. A self-host row carries no environment and says
// so in its details, since the environment column is empty for it.
func (h *Handler) audit(r *http.Request, target Target, action, unit, details string) {
	if target.Self {
		details = "McHarbor's own host: " + details
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		EntityType:    "host_service",
		EntityID:      unit,
		EntityName:    unit,
		EnvironmentID: target.EnvID,
		Details:       details,
	})
}

// requireEnv returns the ?env= id, or answers 404 when it is missing: an
// environment route has no default host; McHarbor's own is /self.
func requireEnv(w http.ResponseWriter, r *http.Request) (Target, bool) {
	envID := response.ParseEnvID(r)
	if envID == "" {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return Target{}, false
	}
	return EnvTarget(envID), true
}

// authed answers 401 when the request carries no user.
func authed(w http.ResponseWriter, r *http.Request) bool {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return false
	}
	return true
}

// envRoute adapts a target-taking handler to the ?env= routes.
func (h *Handler) envRoute(serve func(http.ResponseWriter, *http.Request, Target)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		target, ok := requireEnv(w, r)
		if !ok {
			return
		}
		serve(w, r, target)
	}
}

// selfRoute adapts a target-taking handler to the /self routes.
func (h *Handler) selfRoute(serve func(http.ResponseWriter, *http.Request, Target)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		serve(w, r, SelfTarget())
	}
}

// HandleList returns every service unit on the environment's host, or the
// reason the host cannot be managed.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	h.envRoute(h.serveList)(w, r)
}

// HandleSelfList returns every service unit on McHarbor's own host, or the
// reason McHarbor cannot reach or manage it.
func (h *Handler) HandleSelfList(w http.ResponseWriter, r *http.Request) {
	h.selfRoute(h.serveList)(w, r)
}

func (h *Handler) serveList(w http.ResponseWriter, r *http.Request, target Target) {
	list, err := h.svc.List(r.Context(), target)
	if err != nil {
		h.fail(w, r, target, "list", err, i18n.ErrHostServicesListFailed)
		return
	}
	response.OK(w, list)
}

// HandleAction starts, stops, restarts, enables or disables one unit.
func (h *Handler) HandleAction(w http.ResponseWriter, r *http.Request) {
	h.envRoute(h.serveAction)(w, r)
}

// HandleSelfAction starts, stops, restarts, enables or disables one unit on
// McHarbor's own host.
func (h *Handler) HandleSelfAction(w http.ResponseWriter, r *http.Request) {
	h.selfRoute(h.serveAction)(w, r)
}

func (h *Handler) serveAction(w http.ResponseWriter, r *http.Request, target Target) {
	unit, action := chi.URLParam(r, "unit"), chi.URLParam(r, "action")
	if ValidateUnit(unit) != nil {
		response.BadRequestCode(w, r, i18n.ErrHostServicesInvalidUnit)
		return
	}
	if !ValidAction(action) {
		response.BadRequestCode(w, r, i18n.ErrHostServicesInvalidAction)
		return
	}
	result, err := h.svc.Action(r.Context(), target, unit, action)
	h.audit(r, target, "host_service_"+action, unit, fmt.Sprintf("systemctl %s %s: %s", action, unit, outcome(err)))
	if err != nil {
		h.fail(w, r, target, action, err, i18n.ErrHostServicesActionFailed)
		return
	}
	response.OK(w, result)
}

// HandleGetOverride returns the effective unit and its drop-in override.
func (h *Handler) HandleGetOverride(w http.ResponseWriter, r *http.Request) {
	h.envRoute(h.serveGetOverride)(w, r)
}

// HandleSelfGetOverride returns a unit's effective definition and drop-in on
// McHarbor's own host.
func (h *Handler) HandleSelfGetOverride(w http.ResponseWriter, r *http.Request) {
	h.selfRoute(h.serveGetOverride)(w, r)
}

func (h *Handler) serveGetOverride(w http.ResponseWriter, r *http.Request, target Target) {
	unit := chi.URLParam(r, "unit")
	if ValidateUnit(unit) != nil {
		response.BadRequestCode(w, r, i18n.ErrHostServicesInvalidUnit)
		return
	}
	override, err := h.svc.Override(r.Context(), target, unit)
	if err != nil {
		h.fail(w, r, target, "override_get", err, i18n.ErrHostServicesOverrideFailed)
		return
	}
	response.OK(w, override)
}

// HandlePutOverride writes the drop-in override atomically and reloads as
// asked. The audit row carries the content's sha256, not the content.
func (h *Handler) HandlePutOverride(w http.ResponseWriter, r *http.Request) {
	h.envRoute(h.servePutOverride)(w, r)
}

// HandleSelfPutOverride writes a drop-in override on McHarbor's own host.
func (h *Handler) HandleSelfPutOverride(w http.ResponseWriter, r *http.Request) {
	h.selfRoute(h.servePutOverride)(w, r)
}

func (h *Handler) servePutOverride(w http.ResponseWriter, r *http.Request, target Target) {
	unit := chi.URLParam(r, "unit")
	if ValidateUnit(unit) != nil {
		response.BadRequestCode(w, r, i18n.ErrHostServicesInvalidUnit)
		return
	}
	var req OverrideRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, overrideMaxBytes+4096)).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	result, err := h.svc.SaveOverride(r.Context(), target, unit, req)
	h.audit(r, target, "host_service_override_saved", unit, fmt.Sprintf(
		"override for %s: sha256=%s bytes=%d reload=%t verify=%t: %s",
		unit, ContentHash(req.Content), len(req.Content), req.Reload, req.Verify, outcome(err)))
	if err != nil {
		h.fail(w, r, target, "override_put", err, i18n.ErrHostServicesOverrideFailed)
		return
	}
	response.OK(w, result)
}

// HandleDeleteOverride reverts the unit to its vendor definition by removing
// the drop-in, then reloads systemd.
func (h *Handler) HandleDeleteOverride(w http.ResponseWriter, r *http.Request) {
	h.envRoute(h.serveDeleteOverride)(w, r)
}

// HandleSelfDeleteOverride reverts a unit on McHarbor's own host to its vendor
// definition.
func (h *Handler) HandleSelfDeleteOverride(w http.ResponseWriter, r *http.Request) {
	h.selfRoute(h.serveDeleteOverride)(w, r)
}

func (h *Handler) serveDeleteOverride(w http.ResponseWriter, r *http.Request, target Target) {
	unit := chi.URLParam(r, "unit")
	if ValidateUnit(unit) != nil {
		response.BadRequestCode(w, r, i18n.ErrHostServicesInvalidUnit)
		return
	}
	err := h.svc.RevertOverride(r.Context(), target, unit)
	h.audit(r, target, "host_service_override_reverted", unit, fmt.Sprintf("override for %s removed and daemon reloaded: %s", unit, outcome(err)))
	if err != nil {
		h.fail(w, r, target, "override_delete", err, i18n.ErrHostServicesOverrideFailed)
		return
	}
	response.OKMsg(w, r, i18n.MsgHostServicesOverrideRemoved)
}

// HandleDaemonReload runs systemctl daemon-reload on the host.
func (h *Handler) HandleDaemonReload(w http.ResponseWriter, r *http.Request) {
	h.envRoute(h.serveDaemonReload)(w, r)
}

// HandleSelfDaemonReload runs systemctl daemon-reload on McHarbor's own host.
func (h *Handler) HandleSelfDaemonReload(w http.ResponseWriter, r *http.Request) {
	h.selfRoute(h.serveDaemonReload)(w, r)
}

func (h *Handler) serveDaemonReload(w http.ResponseWriter, r *http.Request, target Target) {
	err := h.svc.DaemonReload(r.Context(), target)
	h.audit(r, target, "host_service_daemon_reload", "", "systemctl daemon-reload: "+outcome(err))
	if err != nil {
		h.fail(w, r, target, "daemon_reload", err, i18n.ErrHostServicesReloadFailed)
		return
	}
	response.OKMsg(w, r, i18n.MsgHostServicesReloaded)
}
