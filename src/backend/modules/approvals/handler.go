// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/pushmsg"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler is the HTTP layer for approval rules and requests.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler builds the handler; the stored bodies are encrypted with the
// application key, so a database dump alone never yields a held payload.
func NewHandler(app *router.AppDeps) *Handler {
	var enc Encrypter
	if app.Encryption != nil {
		enc = app.Encryption
	}
	return &Handler{app: app, svc: NewService(app.DB, enc)}
}

type reasonData struct {
	Reason string `json:"reason"`
}

// writeError maps service errors onto existing codes with a machine reason, so no
// new message code (and no plugin rebuild) is needed.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
	case errors.Is(err, ErrInvalidRule):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, reasonData{Reason: "invalid_rule"})
	case errors.Is(err, ErrExpired):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrGitOpsApprovalExpired, reasonData{Reason: "expired"})
	case errors.Is(err, ErrSelfApproval):
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, reasonData{Reason: "separation_of_duties"})
	case errors.Is(err, ErrApproverLacksPerm):
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrRBACPermissionDenied, reasonData{Reason: "approver_permission"})
	case errors.Is(err, ErrNotRequester):
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, reasonData{Reason: "not_requester"})
	case errors.Is(err, ErrAlreadyDecided):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrForbidden, reasonData{Reason: "already_decided"})
	case errors.Is(err, ErrNotPending):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrForbidden, reasonData{Reason: "not_pending"})
	case errors.Is(err, ErrNotApproved):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrForbidden, reasonData{Reason: "not_approved"})
	case errors.Is(err, ErrAlreadyExecuted):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrForbidden, reasonData{Reason: "already_executed"})
	default:
		h.app.Logger.Error("approvals: "+op, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
}

// HandleCategories lists the risk categories a rule can name.
func (h *Handler) HandleCategories(w http.ResponseWriter, r *http.Request) {
	response.OK(w, Categories())
}

// HandleListRules returns every rule.
func (h *Handler) HandleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.ListRules(r.Context())
	if err != nil {
		h.writeError(w, r, "list rules", err)
		return
	}
	response.OK(w, rules)
}

// HandleCreateRule stores a new rule.
func (h *Handler) HandleCreateRule(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in RuleInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), in, user.ID)
	if err != nil {
		h.writeError(w, r, "create rule", err)
		return
	}
	h.auditRule(r, "approval_rule.create", rule)
	response.Created(w, rule)
}

// HandleUpdateRule replaces a rule.
func (h *Handler) HandleUpdateRule(w http.ResponseWriter, r *http.Request) {
	var in RuleInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	rule, err := h.svc.UpdateRule(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		h.writeError(w, r, "update rule", err)
		return
	}
	h.auditRule(r, "approval_rule.update", rule)
	response.OK(w, rule)
}

// HandleDeleteRule removes a rule.
func (h *Handler) HandleDeleteRule(w http.ResponseWriter, r *http.Request) {
	rule, err := h.svc.DeleteRule(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, r, "delete rule", err)
		return
	}
	h.auditRule(r, "approval_rule.delete", rule)
	response.NoContent(w)
}

func (h *Handler) auditRule(r *http.Request, action string, rule *Rule) {
	env := ""
	if rule.EnvironmentID != nil {
		env = *rule.EnvironmentID
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		EntityType:    "approval_rule",
		EntityID:      rule.ID,
		EntityName:    rule.Category,
		Details:       fmt.Sprintf("enabled=%t required=%d approver=%s expiry_minutes=%d", rule.Enabled, rule.RequiredApprovers, rule.ApproverPermission, rule.ExpiryMinutes),
		EnvironmentID: env,
	})
}

// HandleList returns every request, for approvers and auditors.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	items, total, err := h.svc.List(r.Context(), r.URL.Query().Get("status"), "", page, perPage)
	if err != nil {
		h.writeError(w, r, "list", err)
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// HandleListMine returns the caller's own requests.
func (h *Handler) HandleListMine(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	page, perPage := response.ParsePagination(r)
	items, total, err := h.svc.List(r.Context(), r.URL.Query().Get("status"), user.ID, page, perPage)
	if err != nil {
		h.writeError(w, r, "list mine", err)
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns one request to its requester or to anyone who may read the queue.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	req, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, r, "get", err)
		return
	}
	if req.RequesterID != user.ID && user.ID != "system" {
		ok, err := h.holdsAny(r.Context(), user.ID, "", rbac.PermApprovalsView, rbac.PermApprovalsDecide)
		if err != nil {
			h.writeError(w, r, "get permission", err)
			return
		}
		if !ok {
			response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
			return
		}
	}
	response.OK(w, req)
}

func (h *Handler) holdsAny(ctx context.Context, userID, env string, perms ...rbac.Permission) (bool, error) {
	for _, p := range perms {
		ok, err := h.app.RBACService.HasPermission(ctx, userID, env, p)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// HandleDecide records an approve or deny from someone other than the requester.
func (h *Handler) HandleDecide(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in DecideInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	can := func(ctx context.Context, perm, env string) (bool, error) {
		if user.ID == "system" {
			return true, nil
		}
		return h.app.RBACService.HasPermission(ctx, user.ID, env, rbac.Permission(perm))
	}
	req, err := h.svc.Decide(r.Context(), chi.URLParam(r, "id"), user.ID, user.Username, in, can)
	if err != nil {
		h.writeError(w, r, "decide", err)
		return
	}
	action := "approval.deny"
	severity := "warning"
	if in.Approve {
		action = "approval.approve"
		severity = "success"
	}
	h.auditRequest(r, action, severity, req, "note="+in.Note)
	response.OK(w, req)
}

// HandleCancel lets the requester withdraw an open request.
func (h *Handler) HandleCancel(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	req, err := h.svc.Cancel(r.Context(), chi.URLParam(r, "id"), user.ID)
	if err != nil {
		h.writeError(w, r, "cancel", err)
		return
	}
	h.auditRequest(r, "approval.cancel", "info", req, "")
	response.OK(w, req)
}

func (h *Handler) auditRequest(r *http.Request, action, severity string, req *Request, extra string) {
	env := ""
	if req.EnvironmentID != nil {
		env = *req.EnvironmentID
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		Severity:      severity,
		EntityType:    "approval",
		EntityID:      req.ID,
		EntityName:    req.Category,
		Details:       fmt.Sprintf("%s %s requester=%s status=%s approvals=%d/%d %s", req.Method, req.RoutePattern, req.RequesterName, req.Status, req.ApprovalsCount, req.RequiredApprovers, extra),
		EnvironmentID: env,
	})
}

type pushNotifier interface {
	Notify(n pushmsg.Notification, aud pushmsg.Audience)
}

// notifyApprovers tells every holder of approvals.decide that a request waits.
func (h *Handler) notifyApprovers(req *Request) {
	svc, ok := h.app.LookupService("push")
	if !ok || req == nil {
		return
	}
	notifier, ok := svc.(pushNotifier)
	if !ok {
		return
	}
	notifier.Notify(pushmsg.Notification{
		Title:    "Approval requested",
		Body:     fmt.Sprintf("%s asks to run %s on %s", req.RequesterName, req.Category, req.ResourceName),
		Severity: "warning",
		URL:      "/security?tab=approvals",
		Tag:      "approval-" + req.ID,
	}, pushmsg.Audience{Permission: rbac.PermApprovalsDecide})
}
