// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for metrics HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new metrics handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DockerPool)
	return &Handler{app: app, service: svc}
}

// HandleHostInfo returns host system info and disk usage.
func (h *Handler) HandleHostInfo(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	info, err := h.service.HostInfo(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to get host info", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrMetricsHostInfoFailed)
		return
	}

	response.OK(w, info)
}

// HandleContainerStats returns calculated stats for all running containers.
func (h *Handler) HandleContainerStats(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	stats, err := h.service.AllContainerStats(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to get container stats", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrMetricsStatsFailed)
		return
	}

	response.OK(w, stats)
}

// HandleContainerStatsStream streams live container stats via SSE.
func (h *Handler) HandleContainerStatsStream(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	containerID := chi.URLParam(r, "id")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrMetricsContainerRequired)
		return
	}

	envID := response.ParseEnvID(r)

	// Validate container exists and is running before opening SSE stream.
	if err := h.service.ValidateContainerRunning(r.Context(), envID, containerID); err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrMetricsContainerNotFound)
			return
		}
		response.BadRequestCode(w, r, i18n.ErrMetricsContainerNotRunning)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrMetricsStreamNotSupported)
		return
	}

	// Disable the server write deadline for this long-lived SSE stream.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	ctx := r.Context()
	metricsCh, errCh := h.service.StreamContainerStats(ctx, envID, containerID)

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				h.app.Logger.Error("stats stream error", "error", err, "container", containerID)
				fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", "Stats stream ended")
				flusher.Flush()
			}
			return
		case m, ok := <-metricsCh:
			if !ok {
				return
			}
			payload, err := json.Marshal(m)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: stats\ndata: %s\n\n", string(payload))
			flusher.Flush()
		}
	}
}
