// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package webhooks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
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

	items, total, err := h.service.List(page, perPage)
	if err != nil {
		h.app.Logger.Error("webhooks: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrWebhookListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns a single webhook.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wh, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("webhooks: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrWebhookListFailed)
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
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
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

	wh, err := h.service.Create(input)
	if err != nil {
		h.app.Logger.Error("webhooks: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrWebhookCreateFailed)
		return
	}

	response.Created(w, wh)
}

// HandleUpdate updates an existing webhook.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateWebhookInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	wh, err := h.service.Update(id, input)
	if err != nil {
		h.app.Logger.Error("webhooks: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrWebhookUpdateFailed)
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

	if err := h.service.Delete(id); err != nil {
		h.app.Logger.Error("webhooks: delete error", "error", err, "id", id)
		response.NotFoundCode(w, r, i18n.ErrWebhookNotFound)
		return
	}

	response.NoContent(w)
}

// HandleTest sends a test delivery to the webhook URL.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wh, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("webhooks: test lookup error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrWebhookListFailed)
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

	payloadBytes, _ := json.Marshal(testPayload) // safe: simple map literal

	start := time.Now()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, postErr := client.Post(wh.URL, "application/json", bytes.NewReader(payloadBytes))
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
		buf := make([]byte, 1024)
		n, _ := resp.Body.Read(buf)
		resp.Body.Close()
		respBody = string(buf[:n])
	}

	// Record the delivery
	_ = h.service.RecordDelivery(id, "test", string(payloadBytes), respStatus, respBody, success, duration)

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

	items, total, err := h.service.ListDeliveries(id, page, perPage)
	if err != nil {
		h.app.Logger.Error("webhooks: list deliveries error", "error", err, "webhookId", id)
		response.InternalErrorCode(w, r, i18n.ErrWebhookListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
