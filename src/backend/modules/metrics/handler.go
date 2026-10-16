// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/gpu"
	"github.com/therealmcsparrow/mcharbor/core/hardware"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// Handler holds dependencies for metrics HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
	history *HistoryService
	gpu     *gpu.Reader
	// hardware enumerates the host's devices for GET /metrics/hardware.
	// Host-scoped like `gpu`, and for the same reason: sysfs describes the
	// machine this process runs on, not the selected environment.
	hardware *hardware.Reader
}

// NewHandler creates a new metrics handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewServiceWithAgent(app.DockerPool, app.AgentPool)
	return &Handler{
		app:      app,
		service:  svc,
		history:  NewHistoryService(app.DB),
		gpu:      gpu.NewReader(),
		hardware: hardware.NewReader(),
	}
}

// HandleHostPrune runs a Docker prune operation. The request body must set
// Type and Confirm=true; for system prune, the Volumes flag is an explicit
// opt-in to also delete named volumes.
func (h *Handler) HandleHostPrune(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req PruneRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Type == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	result, err := h.service.Prune(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("host prune failed", "env", envID, "type", req.Type, "error", err)
		response.BadRequestCause(w, r, i18n.ErrMetricsFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "prune",
		EntityType:    "host",
		EntityID:      envID,
		Details:       "type=" + req.Type + " volumes=" + boolStr(req.Volumes),
		EnvironmentID: envID,
	})

	response.OK(w, result)
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// scrapeHandler is built once; promhttp keeps per-handler state that must not be
// recreated on every scrape.
var scrapeHandler = telemetry.Handler()

// HandleScrape serves McHarbor's own Prometheus exposition. Unlike the rest of
// this module it describes the control plane, not containers, and reads only memory.
func (h *Handler) HandleScrape(w http.ResponseWriter, r *http.Request) {
	scrapeHandler.ServeHTTP(w, r)
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
		response.InternalErrorCause(w, r, i18n.ErrMetricsHostInfoFailed, err)
		return
	}

	response.OK(w, info)
}

// HandleStatsPath reports how envID's container stats reach McHarbor — pushed by
// its agent, pulled, or not at all — and why, so a slower environment says so.
// GET /metrics/stats-path
func (h *Handler) HandleStatsPath(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	envID, err := h.app.DockerPool.ResolveEnvironmentIDContext(r.Context(), response.ParseEnvID(r))
	if err != nil {
		response.NotFoundCause(w, r, i18n.ErrEnvNotFound, err)
		return
	}
	response.OK(w, ResolveStatsPath(r.Context(), h.app.DB, h.app.DockerPool, h.app.StatsHub, envID))
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
		if cerrdefs.IsNotFound(err) {
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
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.app.Logger.Debug("disabling stats stream write deadline failed", "container", containerID, "error", err)
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send initial connection event
	if _, err := fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n"); err != nil {
		return
	}
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
				if _, werr := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", "Stats stream ended"); werr != nil {
					return
				}
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
			if _, werr := fmt.Fprintf(w, "event: stats\ndata: %s\n\n", string(payload)); werr != nil {
				return
			}
			flusher.Flush()
		}
	}
}
