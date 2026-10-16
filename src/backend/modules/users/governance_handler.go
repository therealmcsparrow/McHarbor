// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/pushmsg"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

type reasonData struct {
	Reason string `json:"reason"`
}

// HandleBulkAccount previews or applies disable, enable or unlock to a selection.
func (h *Handler) HandleBulkAccount(w http.ResponseWriter, r *http.Request) {
	h.handleBulk(w, r, map[string]bool{BulkDisable: true, BulkEnable: true, BulkUnlock: true})
}

// HandleBulkDelete previews or applies deletion of a selection. It is its own
// route so it can carry users.manage and fall under the user.delete approval rule.
func (h *Handler) HandleBulkDelete(w http.ResponseWriter, r *http.Request) {
	h.handleBulk(w, r, map[string]bool{BulkDelete: true})
}

func (h *Handler) handleBulk(w http.ResponseWriter, r *http.Request, allowed map[string]bool) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in BulkInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if in.Action == "" && allowed[BulkDelete] {
		in.Action = BulkDelete
	}
	if !allowed[in.Action] {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, reasonData{Reason: "invalid_action"})
		return
	}
	res, err := h.service.Bulk(r.Context(), in, user.ID)
	if errors.Is(err, ErrBulkInvalid) {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, reasonData{Reason: "invalid_selection"})
		return
	}
	if err != nil {
		h.app.Logger.Error("users: bulk action", "error", err, "action", in.Action)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if !res.DryRun {
		for _, item := range res.Items {
			if item.Outcome != OutcomeApplied {
				continue
			}
			h.app.AuditLog.Log(r, audit.Entry{Action: "user." + res.Action, EntityType: "user", EntityID: item.UserID,
				EntityName: item.Username, Details: "bulk reason=" + in.Reason})
		}
		severity := "success"
		if res.Summary.Failed > 0 {
			severity = "warning"
		}
		h.app.AuditLog.Log(r, audit.Entry{Action: "user.bulk_" + res.Action, Severity: severity, EntityType: "user",
			EntityName: fmt.Sprintf("%d users", res.Summary.Total),
			Details:    fmt.Sprintf("applied=%d skipped=%d failed=%d reason=%s", res.Summary.Applied, res.Summary.Skipped, res.Summary.Failed, in.Reason)})
	}
	response.OK(w, res)
}

// HandleLifecycle returns every account with its lifecycle flags and counts.
func (h *Handler) HandleLifecycle(w http.ResponseWriter, r *http.Request) {
	inactive := queryInt(r, "inactiveDays")
	elevated := queryInt(r, "elevatedDays")
	report, err := h.service.Lifecycle(r.Context(), inactive, elevated)
	if err != nil {
		h.app.Logger.Error("users: lifecycle", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, report)
}

func (h *Handler) writeBreakGlassError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
	case errors.Is(err, ErrBreakGlassNotDesignated):
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, reasonData{Reason: "not_designated"})
	case errors.Is(err, ErrBreakGlassInvalid):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, reasonData{Reason: "invalid_break_glass"})
	case errors.Is(err, ErrBreakGlassActive):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrForbidden, reasonData{Reason: "already_active"})
	case errors.Is(err, ErrBreakGlassNoSession):
		response.ErrCodeWithData(w, r, http.StatusNotFound, i18n.ErrNotFound, reasonData{Reason: "no_session"})
	default:
		h.app.Logger.Error("users: break-glass "+op, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
}

// HandleBreakGlassStatus returns the caller's own designation and session.
func (h *Handler) HandleBreakGlassStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	st, err := h.service.BreakGlassStatusFor(r.Context(), user.ID)
	if err != nil {
		h.writeBreakGlassError(w, r, "status", err)
		return
	}
	response.OK(w, st)
}

// HandleBreakGlassActivate opens an emergency session for the caller. It raises
// an error-severity audit row and pages everyone who manages accounts, because
// the point of break-glass is that its use is loud.
func (h *Handler) HandleBreakGlassActivate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if rbac.APIKeyFromContext(r.Context()) != nil {
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, reasonData{Reason: "interactive_only"})
		return
	}
	// Break-glass grants everything, so a live session of a designated account is
	// not enough on its own: the person must re-prove who they are first.
	if ok, err := h.app.AuthService.HasSudoWindow(r.Context(), rawSessionCookie(r), auth.SudoScopeBreakGlass); err != nil {
		h.app.Logger.Error("break-glass: checking the step-up window", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	} else if !ok {
		response.ForbiddenCode(w, r, i18n.ErrSudoRequired)
		return
	}
	var in ActivateInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	sess, err := h.service.ActivateBreakGlass(r.Context(), user.ID, in)
	if err != nil {
		h.writeBreakGlassError(w, r, "activate", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "break_glass.activate", Severity: "error", EntityType: "user",
		EntityID: user.ID, EntityName: user.Username,
		Details: fmt.Sprintf("critical: emergency access until %s reason=%s", sess.ExpiresAt, sess.Reason)})
	h.notifyBreakGlass(user.Username, sess)
	response.OK(w, sess)
}

// HandleBreakGlassEnd lets the caller close their own session early.
func (h *Handler) HandleBreakGlassEnd(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	h.endBreakGlass(w, r, user.ID, user.ID)
}

// HandleBreakGlassRevokeSession lets an account manager close someone's session.
func (h *Handler) HandleBreakGlassRevokeSession(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	h.endBreakGlass(w, r, chi.URLParam(r, "userId"), user.ID)
}

func (h *Handler) endBreakGlass(w http.ResponseWriter, r *http.Request, target, by string) {
	sess, err := h.service.EndBreakGlass(r.Context(), target, by)
	if err != nil {
		h.writeBreakGlassError(w, r, "end", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "break_glass.end", Severity: "warning", EntityType: "user",
		EntityID: target, EntityName: sess.Username, Details: "ended_by=" + by})
	response.OK(w, sess)
}

// HandleBreakGlassAccounts lists designated accounts.
func (h *Handler) HandleBreakGlassAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.BreakGlassAccounts(r.Context())
	if err != nil {
		h.writeBreakGlassError(w, r, "accounts", err)
		return
	}
	response.OK(w, items)
}

// HandleBreakGlassSessions lists recent emergency sessions.
func (h *Handler) HandleBreakGlassSessions(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit")
	items, err := h.service.BreakGlassSessions(r.Context(), limit)
	if err != nil {
		h.writeBreakGlassError(w, r, "sessions", err)
		return
	}
	response.OK(w, items)
}

// HandleBreakGlassDesignate designates or updates a break-glass account.
func (h *Handler) HandleBreakGlassDesignate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in DesignateInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	target := chi.URLParam(r, "userId")
	if err := h.service.Designate(r.Context(), target, user.ID, in); err != nil {
		h.writeBreakGlassError(w, r, "designate", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "break_glass.designate", Severity: "warning", EntityType: "user",
		EntityID: target, EntityName: h.auditUsername(r.Context(), target), Details: fmt.Sprintf("max_minutes=%d", in.MaxMinutes)})
	st, err := h.service.BreakGlassStatusFor(r.Context(), target)
	if err != nil {
		h.writeBreakGlassError(w, r, "designate status", err)
		return
	}
	response.OK(w, st)
}

// HandleBreakGlassUndesignate removes a designation.
func (h *Handler) HandleBreakGlassUndesignate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	target := chi.URLParam(r, "userId")
	if err := h.service.Undesignate(r.Context(), target, user.ID); err != nil {
		h.writeBreakGlassError(w, r, "undesignate", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "break_glass.undesignate", Severity: "warning", EntityType: "user",
		EntityID: target, EntityName: h.auditUsername(r.Context(), target)})
	response.NoContent(w)
}

type pushNotifier interface {
	Notify(n pushmsg.Notification, aud pushmsg.Audience)
}

func (h *Handler) notifyBreakGlass(username string, sess *BreakGlassSession) {
	svc, ok := h.app.LookupService("push")
	if !ok {
		return
	}
	notifier, ok := svc.(pushNotifier)
	if !ok {
		return
	}
	notifier.Notify(pushmsg.Notification{
		Title:    "Break-glass access activated",
		Body:     fmt.Sprintf("%s opened emergency access until %s: %s", username, sess.ExpiresAt, sess.Reason),
		Severity: "critical",
		URL:      "/security?tab=break-glass",
		Tag:      "break-glass-" + sess.ID,
	}, pushmsg.Audience{Permission: rbac.PermAccountManage})
}

// queryInt reads an optional integer query parameter, treating an absent or malformed value as 0 so the service applies its default.
func queryInt(r *http.Request, key string) int {
	n, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return 0
	}
	return n
}
