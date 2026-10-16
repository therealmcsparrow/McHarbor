// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the push endpoints.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler returns a handler over svc.
func NewHandler(app *router.AppDeps, svc *Service) *Handler {
	return &Handler{app: app, svc: svc}
}

// browserUser returns the caller, refusing API keys: a push subscription is a
// browser's, and automation has no device to notify.
func (h *Handler) browserUser(w http.ResponseWriter, r *http.Request) *auth.User {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return nil
	}
	if rbac.APIKeyFromContext(r.Context()) != nil {
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return nil
	}
	return user
}

// HandlePublicKey returns the VAPID application server key.
func (h *Handler) HandlePublicKey(w http.ResponseWriter, r *http.Request) {
	if h.browserUser(w, r) == nil {
		return
	}
	pub, err := h.svc.PublicKey(r.Context())
	if err != nil {
		h.app.Logger.Error("push: reading the VAPID key failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, PublicKey{PublicKey: pub})
}

// HandleList returns the caller's own devices.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := h.browserUser(w, r)
	if user == nil {
		return
	}
	subs, err := h.svc.List(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("push: listing subscriptions failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, subs)
}

// HandleSubscribe registers this browser for the caller.
func (h *Handler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	user := h.browserUser(w, r)
	if user == nil {
		return
	}
	var req SubscribeRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	sub, err := h.svc.Subscribe(r.Context(), user.ID, req)
	if errors.Is(err, ErrInvalidSubscription) {
		h.app.Logger.Warn("push: subscription rejected", "user", user.ID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if errors.Is(err, ErrTooManySubscriptions) {
		response.ErrCodeWithData(w, r, http.StatusTooManyRequests, i18n.ErrRateLimitExceed, map[string]string{"reason": "too_many_devices"})
		return
	}
	if err != nil {
		h.app.Logger.Error("push: subscribing failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "create", EntityType: "push_subscription", EntityID: sub.ID, EntityName: sub.Label})
	response.Created(w, sub)
}

// HandleDelete revokes one of the caller's devices.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := h.browserUser(w, r)
	if user == nil {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("push: deleting subscription failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "delete", EntityType: "push_subscription", EntityID: id})
	response.NoContent(w)
}

// HandleTest pushes a test notification to the caller's own devices.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	user := h.browserUser(w, r)
	if user == nil {
		return
	}
	res, err := h.svc.SendTest(r.Context(), user.ID, Notification{
		Title:    "McHarbor test notification",
		Body:     "Push notifications reach this device.",
		Severity: "info",
		URL:      "/oncall/device",
		Tag:      "mcharbor-test",
		Key:      MsgTest,
	})
	if err != nil {
		h.app.Logger.Error("push: test notification failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, res)
}

// HandleRegenerate replaces the instance VAPID keypair, which removes every
// device subscription on the instance.
func (h *Handler) HandleRegenerate(w http.ResponseWriter, r *http.Request) {
	pub, removed, err := h.svc.Regenerate(r.Context())
	if err != nil {
		h.app.Logger.Error("push: regenerating the VAPID key failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "regenerate", EntityType: "push_vapid_key", Severity: "warning",
		Details: fmt.Sprintf("removed_subscriptions=%d", removed),
	})
	response.OK(w, map[string]any{"publicKey": pub, "removedSubscriptions": removed})
}
