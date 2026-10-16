// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package itsm

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the ITSM routes.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// SecretInput is the body that stores a vendor-issued signing secret.
type SecretInput struct {
	Secret string `json:"secret"`
}

func firstForwarded(v string) string {
	if v == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(v, ",")[0])
}

// requestBaseURL is how the operator reached McHarbor, which is how the ITSM
// will reach it too; the proxy's forwarded headers win over the socket.
func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := firstForwarded(r.Header.Get("X-Forwarded-Proto")); p == "http" || p == "https" {
		scheme = p
	}
	host := r.Host
	if h := firstForwarded(r.Header.Get("X-Forwarded-Host")); h != "" {
		host = h
	}
	return scheme + "://" + host
}

func (h *Handler) audit(r *http.Request, action, severity, entityID string, details string) {
	if h.app.AuditLog == nil {
		return
	}
	h.app.AuditLog.Log(r, coreAudit.Entry{Action: action, Severity: severity, EntityType: "communication_channel",
		EntityID: entityID, Details: details})
}

// HandleLinks lists linked tickets.
func (h *Handler) HandleLinks(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	q := r.URL.Query()
	state := q.Get("state")
	switch state {
	case "", LinkPending, LinkOpen, LinkRemoteClosed, LinkResolved, LinkOrphaned:
	default:
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	items, total, err := h.svc.store.List(r.Context(), ListFilter{State: state, ChannelID: q.Get("channel"), EnvID: response.ParseEnvID(r)}, page, perPage)
	if err != nil {
		h.app.Logger.Error("itsm: listing links failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// HandleResync reads one ticket's state from the ITSM now.
func (h *Handler) HandleResync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.svc.Resync(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoLink):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	case errors.Is(err, ErrCannotPoll):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "cannot_poll"})
		return
	case err != nil:
		h.app.Logger.Warn("itsm: resync failed", "link", id, "error", err)
		response.ErrCodeWithData(w, r, http.StatusBadGateway, i18n.ErrInvalidBody, map[string]string{"reason": "resync_failed"})
		return
	}
	link, err := h.svc.store.LinkByID(r.Context(), id)
	if err != nil {
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.audit(r, "itsm.link.resynced", "info", link.ChannelID, fmt.Sprintf(`{"link":%q}`, id))
	response.OK(w, link)
}

// HandleRetry makes a link's parked deliveries due again.
func (h *Handler) HandleRetry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	link, err := h.svc.store.LinkByID(r.Context(), id)
	if errors.Is(err, ErrNoLink) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	n, err := h.svc.store.Unpark(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("itsm: retry failed", "link", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.audit(r, "itsm.link.retried", "info", link.ChannelID, fmt.Sprintf(`{"link":%q,"rows":%d}`, id, n))
	response.OK(w, map[string]int{"requeued": n})
}

// HandleChannels lists the channels that can open tickets.
func (h *Handler) HandleChannels(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Channels(r.Context())
	if err != nil {
		h.app.Logger.Error("itsm: listing channels failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, items)
}

// HandleRotateToken issues a new inbound URL and signing secret, shown once.
// Rotating breaks whatever the ITSM has configured, which the UI says first.
func (h *Handler) HandleRotateToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.svc.RotateToken(r.Context(), id, requestBaseURL(r))
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("itsm: rotating token failed", "channel", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.audit(r, "itsm.webhook.token_rotated", "warning", id, "{}")
	response.OK(w, res)
}

// HandleSetSecret stores a vendor-issued signing secret.
func (h *Handler) HandleSetSecret(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in SecretInput
	if err := response.DecodeBody(r, &in); err != nil || strings.TrimSpace(in.Secret) == "" || len(in.Secret) > 1024 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	err := h.svc.SetVendorSecret(r.Context(), id, strings.TrimSpace(in.Secret))
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("itsm: storing secret failed", "channel", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.audit(r, "itsm.webhook.secret_set", "warning", id, "{}")
	response.OK(w, map[string]bool{"secretSet": true})
}

// HandleInbound is the public route an ITSM calls. Every failure answers the
// same 401, so it reveals nothing about which tokens or channels exist.
func (h *Handler) HandleInbound(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	channelID, applied, err := h.svc.Receive(r.Context(), chi.URLParam(r, "token"), r.Header, body)
	if err != nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	if h.app.AuditLog != nil {
		h.app.AuditLog.Log(nil, coreAudit.Entry{Action: "incident.webhook.received", Severity: "info",
			EntityType: "communication_channel", EntityID: channelID, Details: fmt.Sprintf(`{"applied":%d}`, applied)})
	}
	response.OK(w, map[string]int{"applied": applied})
}
