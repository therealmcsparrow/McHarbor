// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for stack HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new stacks handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.DockerPool, app.Config.DataDir)
	return &Handler{app: app, service: svc}
}

// HandleList returns all stacks with live status.
// GET /stacks?env=envId
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	stacks, err := h.service.List(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to list stacks", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if user.ID != "system" {
		allowedAll, err := h.app.RBACService.HasPermission(user.ID, envID, rbac.PermStacksView)
		if err != nil {
			h.app.Logger.Error("failed to evaluate stack permissions", "env", envID, "error", err)
			response.InternalErrorCode(w, r, i18n.ErrInternalServer)
			return
		}
		if !allowedAll {
			allowedStackNames, err := h.app.RBACService.AllowedStackNames(user.ID, envID, rbac.PermStacksView)
			if err != nil {
				h.app.Logger.Error("failed to load allowed stacks", "env", envID, "error", err)
				response.InternalErrorCode(w, r, i18n.ErrInternalServer)
				return
			}
			stacks = filterStacksByName(stacks, allowedStackNames)
		}
	}

	response.OK(w, stacks)
}

// HandleCreate deploys a new Compose stack.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrStackNameRequired)
		return
	}
	if req.Compose == "" {
		response.BadRequestCode(w, r, i18n.ErrStackComposeRequired)
		return
	}

	st, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.app.Logger.Error("failed to create stack", "name", req.Name, "error", err)
		response.BadRequestCode(w, r, i18n.ErrStackCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "stack",
		EntityName: req.Name,
	})

	h.app.Logger.Info("stack created", "name", st.Name, "user", user.Username)
	response.Created(w, st)
}

// HandleUpdate updates the compose content or description of a stack.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")

	var req UpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	st, err := h.service.Update(r.Context(), name, req)
	if err != nil {
		h.app.Logger.Error("failed to update stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if st == nil {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "stack",
		EntityName: name,
	})

	if st.ID != "" {
		h.service.FireWebhooks(st.ID, "update")
	}

	h.app.Logger.Info("stack updated", "name", name, "user", user.Username)
	response.OK(w, st)
}

// HandleDelete removes a stack entirely (down containers + DB + files).
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	if err := h.service.RemoveStack(r.Context(), envID, name); err != nil {
		h.app.Logger.Error("failed to remove stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackRemoveFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "stack",
		EntityName: name,
	})

	h.app.Logger.Info("stack removed", "name", name, "user", user.Username)
	response.OKMsg(w, r, i18n.MsgStackRemoved)
}

// HandleUp starts the stack.
func (h *Handler) HandleUp(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	result := h.service.Up(r.Context(), name)

	h.app.Logger.Info("stack up", "name", name, "success", result.Success, "user", user.Username)
	if !result.Success {
		response.InternalErrorCode(w, r, i18n.ErrStackDeployFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "up",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(name); stackID != "" {
		h.service.FireWebhooks(stackID, "up")
	}

	response.OK(w, result)
}

// HandleStop stops all containers in a stack (containers remain as "exited").
// POST /stacks/{name}/stop?env=envId
func (h *Handler) HandleStop(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	if err := h.service.StopStack(r.Context(), envID, name); err != nil {
		h.app.Logger.Error("failed to stop stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackStopFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "stop",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(name); stackID != "" {
		h.service.FireWebhooks(stackID, "stop")
	}

	h.app.Logger.Info("stack stopped", "name", name, "user", user.Username)
	response.OK(w, map[string]bool{"success": true})
}

// HandleDown stops and removes all containers in a stack.
// POST /stacks/{name}/down?env=envId&volumes=true
func (h *Handler) HandleDown(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	if err := h.service.DownStack(r.Context(), envID, name); err != nil {
		h.app.Logger.Error("failed to down stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackDownFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "down",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(name); stackID != "" {
		h.service.FireWebhooks(stackID, "down")
	}

	h.app.Logger.Info("stack down", "name", name, "user", user.Username)
	response.OK(w, map[string]bool{"success": true})
}

// HandleRestart restarts all containers in a stack.
// POST /stacks/{name}/restart?env=envId
func (h *Handler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	if err := h.service.RestartStack(r.Context(), envID, name); err != nil {
		h.app.Logger.Error("failed to restart stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackRestartFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "restart",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(name); stackID != "" {
		h.service.FireWebhooks(stackID, "restart")
	}

	h.app.Logger.Info("stack restarted", "name", name, "user", user.Username)
	response.OK(w, map[string]bool{"success": true})
}

// HandleManagedUpdate pulls latest images and redeploys a managed stack.
// POST /stacks/{name}/update
func (h *Handler) HandleManagedUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")

	result, err := h.service.UpdateManagedStack(r.Context(), name)
	if err != nil {
		h.app.Logger.Error("failed to update managed stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackUpdateFailed)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}
	if !result.Success {
		h.app.Logger.Error("managed stack update failed", "name", name, "composeError", result.Error)
		response.InternalErrorCode(w, r, i18n.ErrStackUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(name); stackID != "" {
		h.service.FireWebhooks(stackID, "update")
	}

	h.app.Logger.Info("managed stack updated", "name", name, "user", user.Username)
	response.OK(w, result)
}

// HandleReinstall force recreates a managed stack without pulling new images.
// POST /stacks/{name}/reinstall
func (h *Handler) HandleReinstall(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")

	result, err := h.service.ReinstallManagedStack(r.Context(), name)
	if err != nil {
		h.app.Logger.Error("failed to reinstall managed stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackDeployFailed)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}
	if !result.Success {
		h.app.Logger.Error("managed stack reinstall failed", "name", name, "composeError", result.Error)
		response.InternalErrorCode(w, r, i18n.ErrStackDeployFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "reinstall",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(name); stackID != "" {
		h.service.FireWebhooks(stackID, "deploy")
	}

	h.app.Logger.Info("managed stack reinstalled", "name", name, "user", user.Username)
	response.OK(w, result)
}

// HandleGetDetail returns a single stack by name, supporting both managed and discovered.
// GET /stacks/{name}?env=envId
func (h *Handler) HandleGetDetail(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	st, err := h.service.Detail(r.Context(), envID, name)
	if err != nil {
		h.app.Logger.Error("failed to get stack detail", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if st == nil {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}

	response.OK(w, st)
}

// HandleGetContainers returns container details for all containers in a stack.
// GET /stacks/{name}/containers?env=envId
func (h *Handler) HandleGetContainers(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	containers, err := h.service.StackContainers(r.Context(), envID, name)
	if err != nil {
		h.app.Logger.Error("failed to get stack containers", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	if containers == nil {
		containers = []types.Container{}
	}

	response.OK(w, containers)
}

// HandleGetEnvVars returns stored environment variables for a managed stack.
// GET /stacks/{name}/env-vars?env=envId
func (h *Handler) HandleGetEnvVars(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	vars, err := h.service.EnvVars(name)
	if err != nil {
		h.app.Logger.Error("failed to get stack env vars", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if vars == nil {
		vars = map[string]string{}
	}

	response.OK(w, vars)
}

// HandleGetCompose returns the raw compose file content.
func (h *Handler) HandleGetCompose(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	content, err := h.service.ComposeContent(r.Context(), name)
	if err != nil {
		h.app.Logger.Error("failed to get compose content", "name", name, "error", err)
		response.NotFoundCode(w, r, i18n.ErrStackComposeFailed)
		return
	}

	response.OK(w, map[string]string{"content": content})
}

// HandleAdoptPreview generates a docker-compose.yml preview from a discovered stack or standalone container.
// POST /stacks/adopt/preview
func (h *Handler) HandleAdoptPreview(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req AdoptPreviewRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	envID := response.ParseEnvID(r)

	preview, err := h.service.PreviewAdopt(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("failed to preview adopt", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackPreviewFailed)
		return
	}

	response.OK(w, preview)
}

// HandleAdopt takes over management of a discovered stack or standalone container.
// POST /stacks/adopt
func (h *Handler) HandleAdopt(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req AdoptRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrStackNameRequired)
		return
	}
	if req.Compose == "" {
		response.BadRequestCode(w, r, i18n.ErrStackComposeRequired)
		return
	}

	envID := response.ParseEnvID(r)

	st, err := h.service.Adopt(r.Context(), envID, req)
	if err != nil {
		if errors.Is(err, ErrStackAlreadyManaged) {
			response.ConflictCode(w, r, i18n.ErrStackAlreadyManaged)
			return
		}
		h.app.Logger.Error("failed to adopt stack", "name", req.Name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackAdoptFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "adopt",
		EntityType: "stack",
		EntityName: req.Name,
	})

	h.app.Logger.Info("stack adopted", "name", st.Name, "user", user.Username)
	response.Created(w, st)
}

// HandleGetContainerLink returns the manual stack link for a container.
// GET /stacks/links?containerId=...
func (h *Handler) HandleGetContainerLink(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	containerID := r.URL.Query().Get("containerId")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	envID := response.ParseEnvID(r)
	link, err := h.service.ContainerStackLink(envID, containerID)
	if err != nil {
		h.app.Logger.Error("failed to get container stack link", "container", containerID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackLinkFailed)
		return
	}

	response.OK(w, link)
}

// HandleLinkContainer creates or replaces a manual container-to-stack link.
// POST /stacks/links
func (h *Handler) HandleLinkContainer(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req LinkContainerRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.ContainerID == "" || req.StackName == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	envID := response.ParseEnvID(r)
	link, err := h.service.LinkContainer(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("failed to link container to stack", "container", req.ContainerID, "stack", req.StackName, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackLinkFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "link",
		EntityType: "container",
		EntityID:   link.ContainerID,
		EntityName: link.StackName,
	})
	h.app.Logger.Info("container linked to stack", "container", link.ContainerID, "stack", link.StackName, "user", user.Username)
	response.OK(w, link)
}

// HandleUnlinkContainer removes a manual container-to-stack link.
// DELETE /stacks/links/{containerId}
func (h *Handler) HandleUnlinkContainer(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	containerID := chi.URLParam(r, "containerId")
	envID := response.ParseEnvID(r)
	if err := h.service.UnlinkContainer(envID, containerID); err != nil {
		h.app.Logger.Error("failed to unlink container from stack", "container", containerID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackLinkFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "unlink",
		EntityType: "container",
		EntityID:   containerID,
	})
	h.app.Logger.Info("container unlinked from stack", "container", containerID, "user", user.Username)
	response.NoContent(w)
}

// HandleListWebhooks returns all webhooks for a stack.
// GET /stacks/{name}/webhooks
func (h *Handler) HandleListWebhooks(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	stackID := h.service.StackIDByName(name)
	if stackID == "" {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}

	webhooks, err := h.service.ListWebhooks(stackID)
	if err != nil {
		h.app.Logger.Error("failed to list webhooks", "stack", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, webhooks)
}

// HandleCreateWebhook creates a new webhook for a stack.
// POST /stacks/{name}/webhooks
func (h *Handler) HandleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	stackID := h.service.StackIDByName(name)
	if stackID == "" {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}

	var input CreateStackWebhookInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.URL == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	u, parseErr := url.Parse(input.URL)
	if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	wh, err := h.service.CreateWebhook(stackID, input)
	if err != nil {
		h.app.Logger.Error("failed to create webhook", "stack", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create_webhook",
		EntityType: "stack",
		EntityName: name,
	})

	h.app.Logger.Info("webhook created", "stack", name, "user", user.Username)
	response.Created(w, wh)
}

// HandleUpdateWebhook updates an existing webhook.
// PUT /stacks/{name}/webhooks/{id}
func (h *Handler) HandleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	webhookID := chi.URLParam(r, "id")

	var input UpdateStackWebhookInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	wh, err := h.service.UpdateWebhook(webhookID, input)
	if err != nil {
		h.app.Logger.Error("failed to update webhook", "webhookId", webhookID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if wh == nil {
		response.NotFoundCode(w, r, i18n.ErrStackWebhookNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update_webhook",
		EntityType: "stack",
		EntityName: name,
	})

	h.app.Logger.Info("webhook updated", "stack", name, "webhookId", webhookID, "user", user.Username)
	response.OK(w, wh)
}

// HandleDeleteWebhook removes a webhook.
// DELETE /stacks/{name}/webhooks/{id}
func (h *Handler) HandleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	webhookID := chi.URLParam(r, "id")

	if err := h.service.DeleteWebhook(webhookID); err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			response.NotFoundCode(w, r, i18n.ErrStackWebhookNotFound)
			return
		}
		h.app.Logger.Error("failed to delete webhook", "webhookId", webhookID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete_webhook",
		EntityType: "stack",
		EntityName: name,
	})

	h.app.Logger.Info("webhook deleted", "stack", name, "webhookId", webhookID, "user", user.Username)
	response.OK(w, map[string]bool{"success": true})
}

// HandleTestWebhook sends a test payload to a webhook.
// POST /stacks/{name}/webhooks/{id}/test
func (h *Handler) HandleTestWebhook(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	webhookID := chi.URLParam(r, "id")

	result, err := h.service.TestWebhook(r.Context(), webhookID)
	if err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			response.NotFoundCode(w, r, i18n.ErrStackWebhookNotFound)
			return
		}
		h.app.Logger.Error("failed to test webhook", "webhookId", webhookID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackWebhookFailed)
		return
	}

	h.app.Logger.Info("webhook tested", "stack", name, "webhookId", webhookID, "user", user.Username)
	response.OK(w, result)
}

// HandlePrune removes containers for services no longer in the compose file.
// POST /stacks/{name}/prune?env=envId
func (h *Handler) HandlePrune(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)

	result, err := h.service.PruneOrphans(r.Context(), envID, name)
	if err != nil {
		h.app.Logger.Error("failed to prune stack", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackPruneFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "prune",
		EntityType: "stack",
		EntityName: name,
	})

	h.app.Logger.Info("stack pruned", "name", name, "removed", result.Count, "user", user.Username)
	response.OK(w, result)
}

// HandleUpdateEnvVars updates environment variables for a managed stack.
// PUT /stacks/{name}/env-vars
func (h *Handler) HandleUpdateEnvVars(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")

	var envVars map[string]string
	if err := response.DecodeBody(r, &envVars); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.service.UpdateEnvVars(name, envVars); err != nil {
		h.app.Logger.Error("failed to update env vars", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackEnvVarsFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update_env_vars",
		EntityType: "stack",
		EntityName: name,
	})

	h.app.Logger.Info("stack env vars updated", "name", name, "user", user.Username)
	response.OKMsg(w, r, i18n.MsgStackEnvVarsUpdated)
}

// HandleCheckImageUpdates checks for available image updates across stack services.
// POST /stacks/check-updates?env=envId
func (h *Handler) HandleCheckImageUpdates(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req struct {
		StackNames []string `json:"stackNames"`
	}
	if err := response.DecodeBody(r, &req); err != nil {
		// Allow an empty body and check all stacks.
		req.StackNames = nil
	}
	if user.ID != "system" {
		allowedAll, err := h.app.RBACService.HasPermission(user.ID, envID, rbac.PermStacksView)
		if err != nil {
			h.app.Logger.Error("failed to evaluate stack permissions", "env", envID, "error", err)
			response.InternalErrorCode(w, r, i18n.ErrInternalServer)
			return
		}
		if !allowedAll {
			allowedStackNames, err := h.app.RBACService.AllowedStackNames(user.ID, envID, rbac.PermStacksView)
			if err != nil {
				h.app.Logger.Error("failed to load allowed stacks", "env", envID, "error", err)
				response.InternalErrorCode(w, r, i18n.ErrInternalServer)
				return
			}
			req.StackNames = restrictStackNames(req.StackNames, allowedStackNames)
		}
	}

	results, err := h.service.CheckImageUpdates(r.Context(), envID, req.StackNames)
	if err != nil {
		h.app.Logger.Error("check stack image updates failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackUpdateCheckFailed)
		return
	}

	response.OK(w, results)
}

// HandleGetLogs returns combined logs from all containers in a stack.
// GET /stacks/{name}/logs?env=envId&tail=500
func (h *Handler) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	envID := response.ParseEnvID(r)
	tail := 500
	if t, err := strconv.Atoi(r.URL.Query().Get("tail")); err == nil && t > 0 && t <= 5000 {
		tail = t
	}

	logs, err := h.service.StackLogs(r.Context(), envID, name, tail)
	if err != nil {
		h.app.Logger.Error("failed to get stack logs", "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrStackLogsFailed)
		return
	}

	response.OK(w, logs)
}

func filterStacksByName(stacks []Stack, names []string) []Stack {
	if len(names) == 0 {
		return []Stack{}
	}

	filtered := make([]Stack, 0, len(stacks))
	for _, stack := range stacks {
		if slices.Contains(names, stack.Name) {
			filtered = append(filtered, stack)
		}
	}

	return filtered
}

func restrictStackNames(requested []string, allowed []string) []string {
	if len(allowed) == 0 {
		return []string{}
	}
	if len(requested) == 0 {
		return allowed
	}

	filtered := make([]string, 0, len(requested))
	for _, stackName := range requested {
		if slices.Contains(allowed, stackName) {
			filtered = append(filtered, stackName)
		}
	}

	return filtered
}
