// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for container HTTP handlers.
type Handler struct {
	svc      *Service
	stackSvc *stackStore
	app      *router.AppDeps
}

// NewHandler creates a new container handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc:      NewService(app.DockerPool, app.DB),
		stackSvc: newStackStore(app.DB),
		app:      app,
	}
}

// isSelfTarget returns true if the target container ID matches this McHarbor container.
func isSelfTarget(id string) bool {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return false
	}
	return strings.HasPrefix(hostname, id) || strings.HasPrefix(id, hostname)
}

func writeProtectedError(w http.ResponseWriter, r *http.Request, err error) bool {
	if !errors.Is(err, coredocker.ErrProtectedResource) {
		return false
	}
	response.ForbiddenCode(w, r, i18n.ErrProtectedTarget)
	return true
}

func shortContainerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func normalizeContainerName(name, fallbackID string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if trimmed != "" {
		return trimmed
	}
	return shortContainerID(fallbackID)
}

func (h *Handler) containerAuditName(ctx context.Context, envID, id string) string {
	info, err := h.svc.Inspect(ctx, envID, id)
	if err != nil {
		return shortContainerID(id)
	}

	return normalizeContainerName(info.Name, id)
}

func (h *Handler) logContainerAudit(r *http.Request, envID, action, id, details string) {
	h.logContainerAuditWithName(r, envID, action, id, h.containerAuditName(r.Context(), envID, id), details)
}

func (h *Handler) logContainerAuditWithName(r *http.Request, envID, action, id, name, details string) {
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		EntityType:    "container",
		EntityID:      id,
		EntityName:    normalizeContainerName(name, id),
		Details:       details,
		EnvironmentID: envID,
	})
}

// HandleList returns all containers.
// GET /containers?all=true&env=envId
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	all := r.URL.Query().Get("all") == "true"

	containers, err := h.svc.List(r.Context(), envID, all)
	if err != nil {
		h.app.Logger.Error("list containers failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerListFailed)
		return
	}

	response.OK(w, containers)
}

// HandleBulkStats returns a snapshot of resource stats for all running containers.
// GET /containers/stats/summary
func (h *Handler) HandleBulkStats(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	metrics, err := h.svc.BulkStats(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("bulk stats failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerStatsFailed)
		return
	}

	response.OK(w, metrics)
}

// HandlePrune removes stopped containers.
// POST /containers/prune
func (h *Handler) HandlePrune(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	report, err := h.svc.Prune(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("prune containers failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerPruneFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "prune",
		EntityType:    "container",
		EntityID:      "containers",
		EntityName:    "unused containers",
		Details:       "removed_stopped_containers",
		EnvironmentID: envID,
	})
	h.app.Logger.Info("containers pruned", "env", envID, "deleted", len(report.ContainersDeleted), "user", user.Username)

	response.OK(w, report)
}

// HandleCreate creates a new container.
// POST /containers
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Image == "" {
		response.BadRequestCode(w, r, i18n.ErrContainerImageRequired)
		return
	}

	resp, err := h.svc.Create(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("create container failed", "env", envID, "image", req.Image, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerCreateFailed)
		return
	}

	h.logContainerAudit(r, envID, "create", resp.ID, "image="+req.Image)
	response.Created(w, resp)
}

// HandleInspect returns detailed container information.
// GET /containers/{id}
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("inspect container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerInspectFailed)
		return
	}

	response.OK(w, info)
}

// HandleRemove removes a container.
// DELETE /containers/{id}?force=true&v=true
func (h *Handler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfRemove)
		return
	}

	force := r.URL.Query().Get("force") == "true"
	removeVolumes := r.URL.Query().Get("v") == "true"
	auditName := h.containerAuditName(r.Context(), envID, id)

	if err := h.svc.Remove(r.Context(), envID, id, force, removeVolumes); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("remove container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerRemoveFailed)
		return
	}

	h.logContainerAuditWithName(r, envID, "delete", id, auditName, "")
	response.NoContent(w)
}

// HandleRemoveExtended removes a container with optional image/stack cleanup.
// POST /containers/{id}/remove
func (h *Handler) HandleRemoveExtended(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfRemove)
		return
	}

	var req RemoveExtendedRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Inspect container to get image ID and stack label before removal
	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("inspect container for removal failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerInspectFailed)
		return
	}

	imageID := info.Image
	stackName := ""
	if info.Config != nil && info.Config.Labels != nil {
		stackName = info.Config.Labels["com.docker.compose.project"]
	}
	auditName := normalizeContainerName(info.Name, id)

	// Remove the container
	if err := h.svc.Remove(r.Context(), envID, id, req.Force, req.RemoveVolumes); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("remove container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerRemoveFailed)
		return
	}

	h.logContainerAuditWithName(r, envID, "delete", id, auditName, "")

	result := RemoveExtendedResult{ContainerRemoved: true}

	// Optionally remove the image
	if req.RemoveImage && imageID != "" {
		if err := h.svc.RemoveImage(r.Context(), envID, imageID); err != nil {
			h.app.Logger.Warn("remove image after container removal failed", "env", envID, "image", imageID, "error", err)
		} else {
			result.ImageRemoved = true
		}
	}

	// Optionally remove the stack from McHarbor
	if req.RemoveStack && stackName != "" {
		if err := h.stackSvc.DeleteByName(stackName); err != nil {
			h.app.Logger.Warn("remove stack after container removal failed", "stack", stackName, "error", err)
		} else {
			result.StackRemoved = true
		}
	}

	response.OK(w, result)
}

// HandleStart starts a container.
// POST /containers/{id}/start
func (h *Handler) HandleStart(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.Start(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("start container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerStartFailed)
		return
	}

	h.logContainerAudit(r, envID, "start", id, "")
	response.OKMsg(w, r, i18n.MsgContainerStarted)
}

// HandleStop stops a container.
// POST /containers/{id}/stop?t=10
func (h *Handler) HandleStop(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfStop)
		return
	}

	timeout := 0
	if t := r.URL.Query().Get("t"); t != "" {
		timeout = response.ParseInt(t)
	}

	if err := h.svc.Stop(r.Context(), envID, id, timeout); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("stop container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerStopFailed)
		return
	}

	h.logContainerAudit(r, envID, "stop", id, "")
	response.OKMsg(w, r, i18n.MsgContainerStopped)
}

// HandleRestart restarts a container.
// POST /containers/{id}/restart
func (h *Handler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfRestart)
		return
	}

	if err := h.svc.Restart(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("restart container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerRestartFailed)
		return
	}

	h.logContainerAudit(r, envID, "restart", id, "")
	response.OKMsg(w, r, i18n.MsgContainerRestarted)
}

// HandlePause pauses a container.
// POST /containers/{id}/pause
func (h *Handler) HandlePause(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfPause)
		return
	}

	if err := h.svc.Pause(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("pause container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerPauseFailed)
		return
	}

	h.logContainerAudit(r, envID, "pause", id, "")
	response.OKMsg(w, r, i18n.MsgContainerPaused)
}

// HandleUnpause unpauses a container.
// POST /containers/{id}/unpause
func (h *Handler) HandleUnpause(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.Unpause(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("unpause container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerUnpauseFailed)
		return
	}

	h.logContainerAudit(r, envID, "unpause", id, "")
	response.OKMsg(w, r, i18n.MsgContainerUnpaused)
}

// HandleKill sends a signal to a container.
// POST /containers/{id}/kill?signal=SIGTERM
func (h *Handler) HandleKill(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	signal := r.URL.Query().Get("signal")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfKill)
		return
	}

	if err := h.svc.Kill(r.Context(), envID, id, signal); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("kill container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerKillFailed)
		return
	}

	h.logContainerAudit(r, envID, "kill", id, "signal="+signal)
	response.OKMsg(w, r, i18n.MsgContainerKilled)
}

// HandleUpdate updates container runtime resources.
// POST /containers/{id}/update
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req UpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	resp, err := h.svc.Update(r.Context(), envID, id, req)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("update container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerUpdateFailed)
		return
	}

	h.logContainerAudit(r, envID, "update", id, "")
	response.OK(w, resp)
}

// HandleRecreate stops, renames, recreates, starts, and removes the old container.
// POST /containers/{id}/recreate
func (h *Handler) HandleRecreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req RecreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	resp, err := h.svc.Recreate(r.Context(), envID, id, req)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("recreate container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerRecreateFailed)
		return
	}

	h.logContainerAudit(r, envID, "recreate", id, "")
	response.OK(w, resp)
}

// HandleNetworkConnect connects a container to a network.
// POST /containers/{id}/network/connect
func (h *Handler) HandleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req NetworkConnectRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.Network == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.svc.NetworkConnect(r.Context(), envID, id, req.Network); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("network connect failed", "env", envID, "id", id, "network", req.Network, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
		return
	}

	h.logContainerAudit(r, envID, "network_connect", id, "")
	response.OKMsg(w, r, i18n.MsgContainerActionCompleted)
}

// HandleNetworkDisconnect disconnects a container from a network.
// POST /containers/{id}/network/disconnect
func (h *Handler) HandleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req NetworkDisconnectRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.Network == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.svc.NetworkDisconnect(r.Context(), envID, id, req.Network, req.Force); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("network disconnect failed", "env", envID, "id", id, "network", req.Network, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
		return
	}

	h.logContainerAudit(r, envID, "network_disconnect", id, "")
	response.OKMsg(w, r, i18n.MsgContainerActionCompleted)
}

// HandleLogs returns container logs.
// GET /containers/{id}/logs?stdout=true&stderr=true&tail=100&since=timestamp
func (h *Handler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	query := LogsQuery{
		Stdout: r.URL.Query().Get("stdout") != "false",
		Stderr: r.URL.Query().Get("stderr") != "false",
		Tail:   r.URL.Query().Get("tail"),
		Since:  r.URL.Query().Get("since"),
	}

	logs, err := h.svc.Logs(r.Context(), envID, id, query)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container logs failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerLogsFailed)
		return
	}

	response.OK(w, map[string]string{"logs": logs})
}

// HandleStats returns container resource usage statistics.
// GET /containers/{id}/stats?stream=false
func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	stream := r.URL.Query().Get("stream") == "true"

	body, err := h.svc.Stats(r.Context(), envID, id, stream)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container stats failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerStatsFailed)
		return
	}
	defer body.Close()

	if stream {
		// Disable the server write deadline for this long-lived stream.
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Time{})

		// Stream stats as newline-delimited JSON
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher) // safe: checked for nil before use

		buf := make([]byte, 4096)
		for {
			n, readErr := body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
		return
	}

	// Single snapshot: read the entire response body and return as JSON
	data, err := io.ReadAll(body)
	if err != nil {
		response.InternalErrorCode(w, r, i18n.ErrContainerStatsFailed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// HandleTop returns running processes in a container.
// GET /containers/{id}/top
func (h *Handler) HandleTop(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	top, err := h.svc.Top(r.Context(), envID, id)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container top failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerTopFailed)
		return
	}

	response.OK(w, top)
}

// HandleListFiles lists files and directories at a path inside a container.
// GET /containers/{id}/files?path=/
func (h *Handler) HandleListFiles(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	dirPath := r.URL.Query().Get("path")

	if dirPath == "" {
		dirPath = "/"
	}

	// Security: reject path traversal
	if strings.Contains(dirPath, "..") {
		response.BadRequestCode(w, r, i18n.ErrPathTraversal)
		return
	}

	// Security: require absolute path
	if !strings.HasPrefix(dirPath, "/") {
		response.BadRequestCode(w, r, i18n.ErrAbsolutePathReq)
		return
	}

	files, err := h.svc.ListFiles(r.Context(), envID, id, dirPath)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("list files failed", "env", envID, "id", id, "path", dirPath, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFilesFailed)
		return
	}

	response.OK(w, files)
}

// HandleDetectServices detects OS-level services running inside a container.
// GET /containers/{id}/services
func (h *Handler) HandleDetectServices(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	result, err := h.svc.DetectServices(r.Context(), envID, id)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("detect services failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerServicesFailed)
		return
	}

	response.OK(w, result)
}

// HandleDetectShells probes for available shells in a container.
// POST /containers/{id}/shells
func (h *Handler) HandleDetectShells(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	shells, err := h.svc.DetectShells(r.Context(), envID, id)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("detect shells failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerShellsFailed)
		return
	}

	response.OK(w, shells)
}

// validateFilePath checks a file path for traversal attacks and absolute requirement.
func validateFilePath(w http.ResponseWriter, r *http.Request, filePath string) bool {
	if filePath == "" {
		response.BadRequestCode(w, r, i18n.ErrContainerFileNameRequired)
		return false
	}
	if strings.Contains(filePath, "..") {
		response.BadRequestCode(w, r, i18n.ErrPathTraversal)
		return false
	}
	if !strings.HasPrefix(filePath, "/") {
		response.BadRequestCode(w, r, i18n.ErrAbsolutePathReq)
		return false
	}
	return true
}

// chmodModeRegex validates octal file permission strings like "755", "0644".
var chmodModeRegex = regexp.MustCompile(`^[0-7]{3,4}$`)

// HandleGetFile returns the content of a file inside a container.
// GET /containers/{id}/files/content?path=/etc/hostname&download=true
func (h *Handler) HandleGetFile(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	filePath := r.URL.Query().Get("path")

	if !validateFilePath(w, r, filePath) {
		return
	}

	data, err := h.svc.FileContent(r.Context(), envID, id, filePath)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		if strings.Contains(err.Error(), "exceeds maximum size") {
			response.BadRequestCode(w, r, i18n.ErrContainerFileTooLarge)
			return
		}
		h.app.Logger.Error("get file content failed", "env", envID, "id", id, "path", filePath, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileReadFailed)
		return
	}

	fileName := filepath.Base(filePath)
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	}

	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// HandleSaveFile saves content to a file inside a container.
// PUT /containers/{id}/files/content?path=/app/config.json
func (h *Handler) HandleSaveFile(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	filePath := r.URL.Query().Get("path")

	if !validateFilePath(w, r, filePath) {
		return
	}

	const maxSize = 10 * 1024 * 1024 // 10MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		response.BadRequestCode(w, r, i18n.ErrContainerFileTooLarge)
		return
	}

	if err := h.svc.SaveFileContent(r.Context(), envID, id, filePath, content); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("save file failed", "env", envID, "id", id, "path", filePath, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileWriteFailed)
		return
	}

	h.logContainerAudit(r, envID, "file_save", id, "path="+filePath)
	response.OK(w, map[string]string{"path": filePath})
}

// HandleUploadFile uploads a file to a container directory via multipart form.
// POST /containers/{id}/files/upload?path=/app
func (h *Handler) HandleUploadFile(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	destDir := r.URL.Query().Get("path")

	if !validateFilePath(w, r, destDir) {
		return
	}

	const maxUpload = 100 * 1024 * 1024 // 100MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		response.BadRequestCode(w, r, i18n.ErrContainerFileTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequestCode(w, r, i18n.ErrContainerFileNameRequired)
		return
	}
	defer file.Close()

	if err := h.svc.UploadFile(r.Context(), envID, id, destDir, header.Filename, file, header.Size); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("upload file failed", "env", envID, "id", id, "dest", destDir, "file", header.Filename, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileUploadFailed)
		return
	}

	h.logContainerAudit(r, envID, "file_upload", id, "file="+header.Filename+" dest="+destDir)
	response.OK(w, map[string]string{"file": header.Filename, "path": destDir})
}

// HandleCreateDir creates a directory inside a container.
// POST /containers/{id}/files/directory?path=/app/newdir
func (h *Handler) HandleCreateDir(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	dirPath := r.URL.Query().Get("path")

	if !validateFilePath(w, r, dirPath) {
		return
	}

	if err := h.svc.CreateDirectory(r.Context(), envID, id, dirPath); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("create directory failed", "env", envID, "id", id, "path", dirPath, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileMkdirFailed)
		return
	}

	h.logContainerAudit(r, envID, "file_mkdir", id, "path="+dirPath)
	response.OK(w, map[string]string{"path": dirPath})
}

// HandleRenameFile renames a file or directory inside a container.
// POST /containers/{id}/files/rename — body: { "path": "/old", "newName": "new" }
func (h *Handler) HandleRenameFile(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req struct {
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if !validateFilePath(w, r, req.Path) {
		return
	}
	if req.NewName == "" || strings.Contains(req.NewName, "/") || strings.Contains(req.NewName, "..") {
		response.BadRequestCode(w, r, i18n.ErrContainerFileNameRequired)
		return
	}

	dir := filepath.Dir(req.Path)
	newPath := dir + "/" + req.NewName

	if err := h.svc.RenameFile(r.Context(), envID, id, req.Path, newPath); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("rename file failed", "env", envID, "id", id, "old", req.Path, "new", newPath, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileRenameFailed)
		return
	}

	h.logContainerAudit(r, envID, "file_rename", id, "old="+req.Path+" new="+newPath)
	response.OK(w, map[string]string{"oldPath": req.Path, "newPath": newPath})
}

// HandleChmod changes file permissions inside a container.
// POST /containers/{id}/files/chmod — body: { "path": "/app/script.sh", "mode": "755" }
func (h *Handler) HandleChmod(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if !validateFilePath(w, r, req.Path) {
		return
	}
	if !chmodModeRegex.MatchString(req.Mode) {
		response.BadRequestCode(w, r, i18n.ErrContainerFileModeInvalid)
		return
	}

	if err := h.svc.ChangePermissions(r.Context(), envID, id, req.Path, req.Mode); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("chmod failed", "env", envID, "id", id, "path", req.Path, "mode", req.Mode, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileChmodFailed)
		return
	}

	h.logContainerAudit(r, envID, "file_chmod", id, "path="+req.Path+" mode="+req.Mode)
	response.OK(w, map[string]string{"path": req.Path, "mode": req.Mode})
}

// HandleDeleteFile deletes a file or directory inside a container.
// DELETE /containers/{id}/files/content?path=/app/old.txt&recursive=false
func (h *Handler) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	filePath := r.URL.Query().Get("path")
	recursive := r.URL.Query().Get("recursive") == "true"

	if !validateFilePath(w, r, filePath) {
		return
	}

	if err := h.svc.DeleteFile(r.Context(), envID, id, filePath, recursive); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("delete file failed", "env", envID, "id", id, "path", filePath, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerFileDeleteFailed)
		return
	}

	h.logContainerAudit(r, envID, "file_delete", id, "path="+filePath)
	response.NoContent(w)
}

// HandleCheckImageUpdates checks for available image updates for containers.
// POST /containers/check-updates
func (h *Handler) HandleCheckImageUpdates(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req struct {
		ContainerIDs []string `json:"containerIds"`
	}
	if err := response.DecodeBody(r, &req); err != nil {
		// Allow empty body — check all containers
		req.ContainerIDs = nil
	}

	results, err := h.svc.CheckImageUpdates(r.Context(), envID, req.ContainerIDs)
	if err != nil {
		h.app.Logger.Error("check image updates failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerUpdateCheckFailed)
		return
	}

	response.OK(w, results)
}
