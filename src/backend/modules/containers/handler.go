// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cerrdefs "github.com/containerd/errdefs"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// Handler holds dependencies for container HTTP handlers.
type Handler struct {
	svc      *Service
	stackSvc *stackStore
	app      *router.AppDeps
	// dataCache remembers McHarbor's own data-volume identity for the file-browser guard;
	// dataSourceFn replaces the lookup in tests.
	dataCache    dataSourceCache
	dataSourceFn func(ctx context.Context) []string
}

// NewHandler creates a new container handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DockerPool, app.DB, app.Config.DataDir, app.Encryption)
	svc.SetStatsHub(app.StatsHub)
	svc.SetPinnedSecrets(pinnedSecrets(app))
	return &Handler{
		svc:      svc,
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
	return h.svc.AuditName(ctx, envID, id)
}

// HandleMoveTransfer streams one prepared move archive to a connected agent.
func (h *Handler) HandleMoveTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")
	entry, status, ok := moveTransfers.consume(transferID, r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, http.StatusText(status), status)
		return
	}

	if err := h.svc.writeMoveTransfer(r.Context(), w, entry); err != nil {
		h.app.Logger.Error("container move transfer stream failed", "transfer", transferID, "kind", entry.Kind, "sourceEnv", entry.SourceEnvID, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
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

// logContainerAuditSeverity is logContainerAudit for the entries that are not
// routine — a secret read, a denied one — where an "info" row would let the
// event sink into the noise of ordinary container management.
func (h *Handler) logContainerAuditSeverity(r *http.Request, envID, action, id, severity, details string) {
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		Severity:      severity,
		EntityType:    "container",
		EntityID:      id,
		EntityName:    normalizeContainerName(h.containerAuditName(r.Context(), envID, id), id),
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
		response.InternalErrorCause(w, r, i18n.ErrContainerListFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrContainerStatsFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrContainerPruneFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Image == "" {
		response.BadRequestCode(w, r, i18n.ErrContainerImageRequired)
		return
	}

	resp, err := h.svc.Create(r.Context(), envID, req)
	if err != nil {
		var ref *selfguard.Refusal
		if errors.As(err, &ref) {
			h.refuseSelf(w, r, envID, "", req.Name, "create", ref)
			return
		}
		h.app.Logger.Error("create container failed", "env", envID, "image", req.Image, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerCreateFailed, err)
		return
	}

	h.logContainerAudit(r, envID, "create", resp.ID, "image="+req.Image)
	response.Created(w, resp)
}

// HandleInspect returns detailed container information with credential-looking env values
// replaced by envredact.Marker; ?reveal=true returns them to a caller holding
// containers.manage and is audit-logged. A McHarbor container's own keys never leave.
// GET /containers/{id}
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	reveal := r.URL.Query().Get("reveal") == "true"
	if reveal && !h.mayRevealEnv(w, r, envID) {
		return
	}

	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("inspect container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerInspectFailed, err)
		return
	}

	withheld := h.envPolicy(info, reveal).Container(&info)
	if reveal {
		h.auditEnvReveal(r, envID, info, withheld)
	} else {
		stripSecretMounts(&info)
	}
	response.NoStore(w)
	response.OK(w, h.inspectView(r.Context(), envID, info))
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
	if !h.guardSelf(w, r, envID, &id, "delete", selfOpLifecycle) {
		return
	}

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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("remove container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerRemoveFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "delete", selfOpLifecycle) {
		return
	}

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfRemove)
		return
	}

	var req RemoveExtendedRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	// Inspect container to get image ID and stack label before removal
	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("inspect container for removal failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerInspectFailed, err)
		return
	}

	imageID := info.Image
	stackName := ""
	if info.Config != nil && info.Config.Labels != nil {
		stackName = info.Config.Labels["com.docker.compose.project"]
	}
	auditName := normalizeContainerName(info.Name, id)

	// Discover networks that would be orphaned (only this container attached) so
	// we can offer to remove them together with the container. Best-effort:
	// network inspection failures must not block container removal.
	orphanCandidates, orphanErr := h.svc.FindOrphanedNetworks(r.Context(), envID, id, info)
	if orphanErr != nil {
		h.app.Logger.Warn("find orphaned networks failed", "env", envID, "id", id, "error", orphanErr)
	}

	// Remove the container
	if err := h.svc.Remove(r.Context(), envID, id, req.Force, req.RemoveVolumes); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("remove container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerRemoveFailed, err)
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
		if err := h.stackSvc.DeleteByName(r.Context(), stackName); err != nil {
			h.app.Logger.Warn("remove stack after container removal failed", "stack", stackName, "error", err)
		} else {
			result.StackRemoved = true
		}
	}

	// Optionally remove networks that became orphaned by this container removal
	if req.RemoveNetwork && len(orphanCandidates) > 0 {
		for _, n := range orphanCandidates {
			if err := h.svc.RemoveNetwork(r.Context(), envID, n.ID); err != nil {
				h.app.Logger.Warn("remove network after container removal failed", "env", envID, "network", n.Name, "error", err)
				result.NetworksFailed = append(result.NetworksFailed, FailedNetworkRemoval{
					ID:     n.ID,
					Name:   n.Name,
					Reason: "network removal failed",
				})
				continue
			}
			result.NetworksRemoved = append(result.NetworksRemoved, RemovedNetwork(n))
			h.app.AuditLog.Log(r, audit.Entry{
				Action:        "delete",
				EntityType:    "network",
				EntityID:      n.ID,
				EntityName:    n.Name,
				EnvironmentID: envID,
			})
		}
	}

	response.OK(w, result)
}

// HandleMovePlan previews the resources and settings needed to move a container.
// POST /containers/{id}/move/plan
func (h *Handler) HandleMovePlan(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req MoveContainerPlanRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if !h.allowedOnTarget(w, r, req.TargetEnvID, rbac.PermContainersView) {
		return
	}

	plan, err := h.svc.MovePlan(r.Context(), envID, id, req)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("plan container move failed", "env", envID, "target_env", req.TargetEnvID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		return
	}

	response.OK(w, plan)
}

// allowedOnTarget refuses a move whose target environment the caller holds no
// grant on. The route gate checked only the source (`?env=`); without this a
// container built where the caller is trusted could be moved where they are not.
func (h *Handler) allowedOnTarget(w http.ResponseWriter, r *http.Request, targetEnvID string, perm rbac.Permission) bool {
	return h.app.RBACService.GateOn(w, r, targetEnvID, perm)
}

// HandleMove moves a container to another Docker environment.
// POST /containers/{id}/move
func (h *Handler) HandleMove(w http.ResponseWriter, r *http.Request) {
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

	var req MoveContainerRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if !h.allowedOnTarget(w, r, req.TargetEnvID, rbac.PermContainersManage) {
		return
	}
	if !h.guardSelf(w, r, envID, &id, "move", selfOpLifecycle) {
		return
	}
	if !h.guardMoveTarget(w, r, envID, id, req.TargetEnvID) {
		return
	}

	auditName := h.containerAuditName(r.Context(), envID, id)
	result, err := h.svc.Move(r.Context(), envID, id, req)
	if err != nil {
		if h.writeSelfRefusal(w, r, envID, id, "move", err) {
			return
		}
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("move container failed", "env", envID, "target_env", req.TargetEnvID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		return
	}

	h.logContainerAuditWithName(r, envID, "move", id, auditName, "target_env="+req.TargetEnvID)
	response.OK(w, result)
}

// HandleMoveStream moves a container and streams progress via Server-Sent Events.
// POST /containers/{id}/move/stream
func (h *Handler) HandleMoveStream(w http.ResponseWriter, r *http.Request) {
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

	var req MoveContainerRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if !h.allowedOnTarget(w, r, req.TargetEnvID, rbac.PermContainersManage) {
		return
	}
	if !h.guardSelf(w, r, envID, &id, "move", selfOpLifecycle) {
		return
	}
	if !h.guardMoveTarget(w, r, envID, id, req.TargetEnvID) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.app.Logger.Warn("disabling write deadline failed", "error", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	auditName := h.containerAuditName(r.Context(), envID, id)
	ctx := r.Context()
	events := make(chan MoveContainerEvent, 16)
	go func() {
		result, err := h.svc.MoveWithProgress(ctx, envID, id, req, events)
		if err != nil {
			h.app.Logger.Error("move container failed", "env", envID, "target_env", req.TargetEnvID, "id", id, "error", err)
			return
		}
		h.app.Logger.Info("container moved", "env", envID, "target_env", req.TargetEnvID, "id", id, "target_id", result.TargetContainerID)
		h.logContainerAuditWithName(r, envID, "move", id, auditName, "target_env="+req.TargetEnvID)
	}()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				h.app.Logger.Error("marshal move progress event failed", "error", err)
				continue
			}
			if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", data); writeErr != nil {
				return
			}
			flusher.Flush()

			if event.Status == "done" || event.Status == "error" {
				return
			}
		}
	}
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
	if !h.guardSelf(w, r, envID, &id, "start", selfOpLifecycle) {
		return
	}

	if err := h.svc.Start(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("start container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerStartFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "stop", selfOpLifecycle) {
		return
	}

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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("stop container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerStopFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "restart", selfOpLifecycle) {
		return
	}

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfRestart)
		return
	}

	if err := h.svc.Restart(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("restart container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerRestartFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "pause", selfOpLifecycle) {
		return
	}

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfPause)
		return
	}

	if err := h.svc.Pause(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("pause container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerPauseFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "unpause", selfOpLifecycle) {
		return
	}

	if err := h.svc.Unpause(r.Context(), envID, id); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("unpause container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerUnpauseFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "kill", selfOpLifecycle) {
		return
	}
	signal := r.URL.Query().Get("signal")

	if isSelfTarget(id) {
		response.BadRequestCode(w, r, i18n.ErrContainerSelfKill)
		return
	}

	if err := h.svc.Kill(r.Context(), envID, id, signal); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("kill container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerKillFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "update", selfOpLifecycle) {
		return
	}

	var req UpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	updatedFields := containerUpdateFields(req)
	if len(updatedFields) == 0 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	resp, err := h.svc.Update(r.Context(), envID, id, req)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("update container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerUpdateFailed, err)
		return
	}

	h.logContainerAudit(r, envID, "update", id, "fields="+strings.Join(updatedFields, ","))
	response.OK(w, resp)
}

func containerUpdateFields(req UpdateRequest) []string {
	fields := make([]string, 0, 11)
	values := []struct {
		name    string
		present bool
	}{
		{"memory", req.Memory != nil},
		{"memorySwap", req.MemorySwap != nil},
		{"memoryReservation", req.MemoryReservation != nil},
		{"nanoCPUs", req.NanoCPUs != nil},
		{"cpuShares", req.CPUShares != nil},
		{"cpuQuota", req.CPUQuota != nil},
		{"cpuPeriod", req.CPUPeriod != nil},
		{"cpusetCpus", req.CpusetCpus != nil},
		{"cpusetMems", req.CpusetMems != nil},
		{"blkioWeight", req.BlkioWeight != nil},
		{"restartPolicy", req.RestartPolicy != nil},
	}
	for _, value := range values {
		if value.present {
			fields = append(fields, value.name)
		}
	}
	return fields
}

// HandleRename changes the Docker container name.
// POST /containers/{id}/rename
func (h *Handler) HandleRename(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	if !h.guardSelf(w, r, envID, &id, "rename", selfOpLifecycle) {
		return
	}

	var req RenameRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	name := normalizeContainerRenameName(req.Name)
	if name == "" {
		response.BadRequestCode(w, r, i18n.ErrContainerNameRequired)
		return
	}
	if !isValidContainerRenameName(name) {
		response.BadRequestCode(w, r, i18n.ErrContainerNameInvalid)
		return
	}

	if err := h.svc.Rename(r.Context(), envID, id, name, req.UnlockProtected); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("rename container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerRenameFailed, err)
		return
	}

	h.logContainerAudit(r, envID, "rename", id, "name="+name)
	response.OKMsg(w, r, i18n.MsgContainerRenamed)
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
	if !h.guardSelf(w, r, envID, &id, "recreate", selfOpLifecycle) {
		return
	}

	var req RecreateEditRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	resp, err := h.svc.RecreateWithMounts(r.Context(), envID, id, req)
	if err != nil {
		if errors.Is(err, ErrInvalidMount) {
			h.app.Logger.Warn("recreate refused: invalid mount", "env", envID, "id", id, "error", err)
			response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "invalid_mount"})
			return
		}
		if h.writeSelfRefusal(w, r, envID, id, "recreate", err) {
			return
		}
		if errors.Is(err, ErrMaskedValueMoved) {
			h.app.Logger.Warn("recreate refused: a withheld value would move", "env", envID, "id", id, "error", err)
			response.BadRequestCause(w, r, i18n.ErrMaskedValueMoved, err)
			return
		}
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("recreate container failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerRecreateFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "network_connect", selfOpLifecycle) {
		return
	}

	var req NetworkConnectRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Network == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.svc.NetworkConnect(r.Context(), envID, id, req); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("network connect failed", "env", envID, "id", id, "network", req.Network, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
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
	if !h.guardSelf(w, r, envID, &id, "network_disconnect", selfOpLifecycle) {
		return
	}

	var req NetworkDisconnectRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
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
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}
	tailCount, err := strconv.Atoi(tail)
	if err != nil || tailCount < 1 || tailCount > 2000 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	query := LogsQuery{
		Stdout: r.URL.Query().Get("stdout") != "false",
		Stderr: r.URL.Query().Get("stderr") != "false",
		Tail:   strconv.Itoa(tailCount),
		Since:  r.URL.Query().Get("since"),
	}

	logs, err := h.svc.Logs(r.Context(), envID, id, query)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container logs failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerLogsFailed, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store, private")
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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container stats failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerStatsFailed, err)
		return
	}
	defer func() {
		if err := body.Close(); err != nil {
			slog.Warn("closing body failed", "error", err)
		}
	}()

	if stream {
		// Disable the server write deadline for this long-lived stream.
		rc := http.NewResponseController(w)
		if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			h.app.Logger.Warn("disabling write deadline failed", "error", err)
		}

		// Stream stats as newline-delimited JSON
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher) // safe: checked for nil before use

		buf := make([]byte, 4096)
		for {
			n, readErr := body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return
				}
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
		response.InternalErrorCause(w, r, i18n.ErrContainerStatsFailed, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		h.app.Logger.Debug("writing response body failed", "error", err)
	}
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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container top failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerTopFailed, err)
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

	if !h.guardProtectedPath(w, r, envID, id, "secret_file_list", dirPath) {
		return
	}
	canReveal := h.canRevealSecrets(r, envID)
	if isSecretPath(dirPath) {
		if !canReveal {
			h.logSecretFileAudit(r, envID, id, "secret_file_list_denied", dirPath)
			response.ForbiddenCode(w, r, i18n.ErrSecretPathForbidden)
			return
		}
		h.logSecretFileAudit(r, envID, id, "secret_file_list", dirPath)
	}

	files, err := h.svc.ListFiles(r.Context(), envID, id, dirPath)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("list files failed", "env", envID, "id", id, "path", dirPath, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFilesFailed, err)
		return
	}

	if !canReveal {
		files = hideSecretEntries(files)
	}

	h.auditMountRead(r, envID, id, "file_list", "path="+dirPath, dirPath)
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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("detect services failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerServicesFailed, err)
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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("detect shells failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerShellsFailed, err)
		return
	}

	response.OK(w, shells)
}

// HandleDiff returns the filesystem changes recorded against the container's
// running image versus its declared image layers.
// GET /containers/{id}/diff
func (h *Handler) HandleDiff(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	result, err := h.svc.Diff(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container diff failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerDiffFailed, err)
		return
	}

	response.OK(w, result)
}

// HandleExec runs a one-shot command inside a running container, capturing
// stdout/stderr/exit code and persisting the invocation to container_exec_history.
// Mutating gated (PermContainersManage). POST /containers/{id}/exec
func (h *Handler) HandleExec(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	if !h.guardSelf(w, r, envID, &id, "exec", selfOpExec) {
		return
	}

	var req ExecRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrContainerExecBadRequest, err)
		return
	}

	result, err := h.svc.RunExec(r.Context(), envID, id, user.Username, user.ID, req)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container exec failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerExecFailed, err)
		return
	}

	response.OK(w, result)
}

// HandleExecHistory returns the most recent exec audit entries for the given
// container. GET /containers/{id}/exec/history?limit=N. The route is
// containers.view, but other people's rows carry command output only an exec
// could produce, so they need containers.manage; a viewer sees only their own.
func (h *Handler) HandleExecHistory(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	onlyUser, ok := h.execHistoryScope(w, r, user, envID)
	if !ok {
		return
	}
	entries, err := h.svc.listExecHistory(r.Context(), envID, id, user.ID, onlyUser, limit)
	if err != nil {
		h.app.Logger.Error("container exec history failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerExecHistoryFailed, err)
		return
	}

	response.OK(w, entries)
}

// execHistoryScope returns "" when the caller may read every user's exec rows on envID
// (containers.manage, the key that runs exec), or the caller's own id otherwise. It fails
// closed: an RBAC error is a 500, never a wider read.
func (h *Handler) execHistoryScope(w http.ResponseWriter, r *http.Request, user *auth.User, envID string) (string, bool) {
	if user.ID == "system" {
		return "", true
	}
	if h.app.RBACService == nil {
		return user.ID, true
	}
	allowed, err := h.app.RBACService.AllowedOn(r, envID, rbac.PermContainersManage)
	if err != nil {
		h.app.Logger.Error("checking containers.manage for exec history failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return "", false
	}
	if allowed {
		return "", true
	}
	return user.ID, true
}

// HandleHealthHistory returns the persisted healthcheck probe series and the
// run-state samples behind the Overview tab's Health and Uptime cards.
// Read-only (PermContainersView). GET /containers/{id}/health/history?limit=N
func (h *Handler) HandleHealthHistory(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidPath)
		return
	}

	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	history, err := h.svc.healthHistory(r.Context(), envID, id, limit)
	if err != nil {
		h.app.Logger.Error("container health history failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerHealthFailed, err)
		return
	}

	response.OK(w, history)
}

// HandleHealthSummary returns fleet-wide availability over a window, stitched
// from the raw health/uptime series and the daily rollup behind it.
//
// This is the reporting surface for the same storage the per-container
// endpoint above serves: one aggregated query instead of one request per
// workload. Read-only (PermContainersView).
// GET /containers/health/summary?days=30&limit=500 (or from=&to=)
func (h *Handler) HandleHealthSummary(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	now := time.Now().UTC()

	// `days` is the common case; `from`/`to` exist because a report covers a
	// stated period, not "the last N days from whenever it happened to run".
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		days = n
	}
	if days > uptimeRollupRetentionDays {
		days = uptimeRollupRetentionDays
	}
	from, to := now.AddDate(0, 0, -days), now

	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
		from = parsed.UTC()
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
		to = parsed.UTC()
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	summary, err := h.svc.healthSummary(r.Context(), envID, from, to, limit)
	if err != nil {
		h.app.Logger.Error("container health summary failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerHealthFailed, err)
		return
	}

	response.OK(w, summary)
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

	if !h.guardSecretPath(w, r, envID, id, "secret_file_read", filePath) {
		return
	}

	data, err := h.svc.FileContent(r.Context(), envID, id, filePath)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		if strings.Contains(err.Error(), "exceeds maximum size") {
			response.BadRequestCode(w, r, i18n.ErrContainerFileTooLarge)
			return
		}
		h.app.Logger.Error("get file content failed", "env", envID, "id", id, "path", filePath, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileReadFailed, err)
		return
	}

	fileName := filepath.Base(filePath)
	download := r.URL.Query().Get("download") == "true"
	if download {
		w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	}

	action := "file_read"
	if download {
		action = "file_download"
	}
	h.auditMountRead(r, envID, id, action, "path="+filePath, filePath)

	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		h.app.Logger.Debug("writing response body failed", "error", err)
	}
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
	if !h.guardSelf(w, r, envID, &id, "file_write", selfOpExec) {
		return
	}
	filePath := r.URL.Query().Get("path")

	if !validateFilePath(w, r, filePath) {
		return
	}

	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", filePath) {
		return
	}

	const maxSize = 10 * 1024 * 1024 // 10MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrContainerFileTooLarge, err)
		return
	}

	if err := h.svc.SaveFileContent(r.Context(), envID, id, filePath, content); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("save file failed", "env", envID, "id", id, "path", filePath, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileWriteFailed, err)
		return
	}

	h.auditMountMutation(r, envID, id, "file_save", "path="+filePath, filePath)
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
	if !h.guardSelf(w, r, envID, &id, "file_upload", selfOpExec) {
		return
	}
	destDir := r.URL.Query().Get("path")

	if !validateFilePath(w, r, destDir) {
		return
	}

	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", destDir) {
		return
	}

	const maxUpload = 4 << 30 // 4GB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	files, dirs, closeFiles, err := uploadFilesFromMultipartRequest(r)
	if err != nil || (len(files) == 0 && len(dirs) == 0) {
		if err != nil {
			h.app.Logger.Warn("parse upload multipart form failed", "env", envID, "id", id, "dest", destDir, "content_length", r.ContentLength, "error", err)
		}
		response.BadRequestCode(w, r, i18n.ErrContainerFileNameRequired)
		return
	}
	defer closeFiles()

	if err := h.svc.UploadFiles(r.Context(), envID, id, destDir, files, dirs); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("upload file failed", "env", envID, "id", id, "dest", destDir, "count", len(files), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileUploadFailed, err)
		return
	}

	h.auditMountMutation(r, envID, id, "file_upload", fmt.Sprintf("count=%d dest=%s", len(files), destDir), destDir)
	response.OK(w, map[string]any{"count": len(files), "path": destDir})
}

func uploadFilesFromMultipartRequest(r *http.Request) ([]UploadFileItem, []string, func(), error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("creating multipart reader: %w", err)
	}

	var files []UploadFileItem
	var dirs []string
	var opened []*os.File
	var tempPaths []string
	pendingPath := ""
	lastFileNeedsPath := -1
	cleanup := func() {
		for _, file := range opened {
			if err := file.Close(); err != nil {
				continue
			}
		}
		for _, tempPath := range tempPaths {
			if err := os.Remove(tempPath); err != nil {
				continue
			}
		}
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("reading upload part: %w", err)
		}
		switch part.FormName() {
		case "dirs":
			dir, err := readUploadTextPart(part)
			if err != nil {
				cleanup()
				return nil, nil, func() {}, err
			}
			dirs = append(dirs, dir)
		case "paths":
			relativePath, err := readUploadTextPart(part)
			if err != nil {
				cleanup()
				return nil, nil, func() {}, err
			}
			if lastFileNeedsPath >= 0 {
				files[lastFileNeedsPath].RelativePath = relativePath
				lastFileNeedsPath = -1
			} else {
				pendingPath = relativePath
			}
		case "files", "file":
			file, tempPath, size, err := uploadTempFileFromPart(part)
			if err != nil {
				cleanup()
				return nil, nil, func() {}, err
			}
			opened = append(opened, file)
			tempPaths = append(tempPaths, tempPath)
			relativePath := part.FileName()
			if pendingPath != "" {
				relativePath = pendingPath
				pendingPath = ""
			} else {
				lastFileNeedsPath = len(files)
			}
			files = append(files, UploadFileItem{
				RelativePath: relativePath,
				Reader:       file,
				Size:         size,
			})
		default:
			if _, err := io.Copy(io.Discard, part); err != nil {
				cleanup()
				return nil, nil, func() {}, fmt.Errorf("discarding upload part: %w", err)
			}
		}
		if err := part.Close(); err != nil {
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("closing upload part: %w", err)
		}
	}

	return files, dirs, cleanup, nil
}

func readUploadTextPart(part *multipart.Part) (string, error) {
	const maxTextPart = 1 << 20
	data, err := io.ReadAll(io.LimitReader(part, maxTextPart+1))
	if err != nil {
		return "", fmt.Errorf("reading upload form value: %w", err)
	}
	if len(data) > maxTextPart {
		return "", fmt.Errorf("upload form value is too large")
	}
	return string(data), nil
}

func uploadTempFileFromPart(part *multipart.Part) (*os.File, string, int64, error) {
	file, err := os.CreateTemp("", "mcharbor-upload-*")
	if err != nil {
		return nil, "", 0, fmt.Errorf("creating upload temp file: %w", err)
	}
	size, copyErr := io.Copy(file, part)
	if copyErr != nil {
		name := file.Name()
		if closeErr := file.Close(); closeErr != nil {
			return nil, "", 0, fmt.Errorf("copying upload temp file: %w; closing temp file: %w", copyErr, closeErr)
		}
		if removeErr := os.Remove(name); removeErr != nil {
			return nil, "", 0, fmt.Errorf("copying upload temp file: %w; removing temp file: %w", copyErr, removeErr)
		}
		return nil, "", 0, fmt.Errorf("copying upload temp file: %w", copyErr)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		name := file.Name()
		if closeErr := file.Close(); closeErr != nil {
			return nil, "", 0, fmt.Errorf("seeking upload temp file: %w; closing temp file: %w", err, closeErr)
		}
		if removeErr := os.Remove(name); removeErr != nil {
			return nil, "", 0, fmt.Errorf("seeking upload temp file: %w; removing temp file: %w", err, removeErr)
		}
		return nil, "", 0, fmt.Errorf("seeking upload temp file: %w", err)
	}
	return file, file.Name(), size, nil
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
	if !h.guardSelf(w, r, envID, &id, "file_mkdir", selfOpExec) {
		return
	}
	dirPath := r.URL.Query().Get("path")

	if !validateFilePath(w, r, dirPath) {
		return
	}

	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", dirPath) {
		return
	}

	if err := h.svc.CreateDirectory(r.Context(), envID, id, dirPath); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("create directory failed", "env", envID, "id", id, "path", dirPath, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileMkdirFailed, err)
		return
	}

	h.auditMountMutation(r, envID, id, "file_mkdir", "path="+dirPath, dirPath)
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
	if !h.guardSelf(w, r, envID, &id, "file_rename", selfOpExec) {
		return
	}

	var req struct {
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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

	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", req.Path) {
		return
	}
	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", newPath) {
		return
	}

	if err := h.svc.RenameFile(r.Context(), envID, id, req.Path, newPath); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("rename file failed", "env", envID, "id", id, "old", req.Path, "new", newPath, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileRenameFailed, err)
		return
	}

	h.auditMountMutation(r, envID, id, "file_rename", "old="+req.Path+" new="+newPath, req.Path, newPath)
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
	if !h.guardSelf(w, r, envID, &id, "file_chmod", selfOpExec) {
		return
	}

	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if !validateFilePath(w, r, req.Path) {
		return
	}
	if !chmodModeRegex.MatchString(req.Mode) {
		response.BadRequestCode(w, r, i18n.ErrContainerFileModeInvalid)
		return
	}

	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", req.Path) {
		return
	}

	if err := h.svc.ChangePermissions(r.Context(), envID, id, req.Path, req.Mode); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("chmod failed", "env", envID, "id", id, "path", req.Path, "mode", req.Mode, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileChmodFailed, err)
		return
	}

	h.auditMountMutation(r, envID, id, "file_chmod", "path="+req.Path+" mode="+req.Mode, req.Path)
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
	if !h.guardSelf(w, r, envID, &id, "file_delete", selfOpExec) {
		return
	}
	filePath := r.URL.Query().Get("path")
	recursive := r.URL.Query().Get("recursive") == "true"

	if !validateFilePath(w, r, filePath) {
		return
	}

	if !h.guardSecretPath(w, r, envID, id, "secret_file_write", filePath) {
		return
	}

	if err := h.svc.DeleteFile(r.Context(), envID, id, filePath, recursive); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("delete file failed", "env", envID, "id", id, "path", filePath, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerFileDeleteFailed, err)
		return
	}

	h.auditMountMutation(r, envID, id, "file_delete", "path="+filePath, filePath)
	response.NoContent(w)
}

// CheckImageUpdatesRequest names the containers whose images to check; empty means all.
type CheckImageUpdatesRequest struct {
	ContainerIDs []string `json:"containerIds"`
}

// HandleCheckImageUpdates checks for available image updates for containers.
// POST /containers/check-updates
func (h *Handler) HandleCheckImageUpdates(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req CheckImageUpdatesRequest
	if err := response.DecodeBody(r, &req); err != nil {
		// Allow empty body — check all containers
		req.ContainerIDs = nil
	}

	results, err := h.svc.CheckImageUpdates(r.Context(), envID, req.ContainerIDs)
	if err != nil {
		h.app.Logger.Error("check image updates failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerUpdateCheckFailed, err)
		return
	}

	response.OK(w, results)
}
