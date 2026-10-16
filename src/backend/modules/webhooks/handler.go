// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package webhooks manages outbound webhook definitions and their delivery history.
package webhooks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Handler holds dependencies for webhook HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new webhooks handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Encryption),
	}
}

// HandleList returns a paginated list of webhooks.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("webhooks: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrWebhookListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns a single webhook.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wh, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("webhooks: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrWebhookListFailed, err)
		return
	}
	if wh == nil {
		response.NotFoundCode(w, r, i18n.ErrWebhookNotFound)
		return
	}

	response.OK(w, wh)
}

// HandleCreate creates a new webhook.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateWebhookInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrWebhookNameRequired)
		return
	}
	if input.URL == "" {
		response.BadRequestCode(w, r, i18n.ErrWebhookUrlRequired)
		return
	}

	wh, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("webhooks: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrWebhookCreateFailed, err)
		return
	}

	response.Created(w, wh)
}

// HandleUpdate updates an existing webhook.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateWebhookInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	wh, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("webhooks: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrWebhookUpdateFailed, err)
		return
	}
	if wh == nil {
		response.NotFoundCode(w, r, i18n.ErrWebhookNotFound)
		return
	}

	response.OK(w, wh)
}

// HandleDelete removes a webhook.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.Delete(r.Context(), id); err != nil {
		h.app.Logger.Error("webhooks: delete error", "error", err, "id", id)
		response.NotFoundCause(w, r, i18n.ErrWebhookNotFound, err)
		return
	}

	response.NoContent(w)
}

// HandleTest sends a test delivery to the webhook URL.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wh, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("webhooks: test lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrWebhookListFailed, err)
		return
	}
	if wh == nil {
		response.NotFoundCode(w, r, i18n.ErrWebhookNotFound)
		return
	}

	// Build test payload
	testPayload := map[string]any{
		"event":     "test",
		"webhookId": wh.ID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   "This is a test delivery from McHarbor",
	}

	payloadBytes, err := json.Marshal(testPayload)
	if err != nil {
		h.app.Logger.Error("webhooks: test payload encode error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	start := time.Now()
	client := &http.Client{Timeout: 10 * time.Second}
	req, reqErr := http.NewRequestWithContext(r.Context(), http.MethodPost, wh.URL, bytes.NewReader(payloadBytes))
	var resp *http.Response
	postErr := reqErr
	if reqErr == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, postErr = client.Do(req)
	}
	duration := int(time.Since(start).Milliseconds())

	respStatus := 0
	respBody := ""
	success := false

	if postErr != nil {
		respBody = postErr.Error()
	} else {
		respStatus = resp.StatusCode
		success = resp.StatusCode >= 200 && resp.StatusCode < 300
		// Read limited response body
		buf, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			h.app.Logger.Warn("webhooks: test response read error", "error", readErr, "id", id)
		}
		closeutil.Report(resp.Body.Close(), "resp.Body")
		respBody = string(buf)
	}

	// Record the delivery
	if recErr := h.service.RecordDelivery(r.Context(), id, "test", string(payloadBytes), respStatus, respBody, success, duration); recErr != nil {
		h.app.Logger.Error("webhooks: record delivery error", "error", recErr, "id", id)
	}

	response.OK(w, map[string]any{
		"success":        success,
		"responseStatus": respStatus,
		"responseBody":   respBody,
		"duration":       duration,
	})
}

// HandleListDeliveries returns delivery history for a webhook.
func (h *Handler) HandleListDeliveries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListDeliveries(r.Context(), id, page, perPage)
	if err != nil {
		h.app.Logger.Error("webhooks: list deliveries error", "error", err, "webhookId", id)
		response.InternalErrorCause(w, r, i18n.ErrWebhookListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
