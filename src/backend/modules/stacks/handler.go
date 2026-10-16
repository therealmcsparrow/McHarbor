// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
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
	svc := NewService(app.DB, app.DockerPool, app.Config.DataDir, app.Encryption)
	svc.SetPinnedSecrets(stackPinned(app))
	return &Handler{app: app, service: svc}
}

// stackEnvironment resolves the environment of the stack a /stacks/{name} request names,
// for rbac.PinEnvToResource.
func (h *Handler) stackEnvironment(r *http.Request) (string, bool, error) {
	envID, found, err := h.service.StackEnvironment(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		h.app.Logger.Error("stacks: resolving the stack's environment", "name", chi.URLParam(r, "name"), "error", err)
	}
	return envID, found, err
}

// writeComposeMaskError answers a compose save that the marker restore refused, and reports
// whether it did.
func writeComposeMaskError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, envredact.ErrComposeMaskedMoved):
		response.BadRequestCode(w, r, i18n.ErrMaskedValueMoved)
		return true
	case errors.Is(err, envredact.ErrComposeMaskedValue):
		response.BadRequestCode(w, r, i18n.ErrStackComposeMaskedValue)
		return true
	}
	return false
}

func writeProtectedError(w http.ResponseWriter, r *http.Request, err error) bool {
	if !errors.Is(err, coredocker.ErrProtectedResource) {
		return false
	}
	response.ForbiddenCode(w, r, i18n.ErrProtectedTarget)
	return true
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if user.ID != "system" {
		allowedAll, err := h.app.RBACService.HasPermission(r.Context(), user.ID, envID, rbac.PermStacksView)
		if err != nil {
			h.app.Logger.Error("failed to evaluate stack permissions", "env", envID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if !allowedAll {
			allowedStackNames, err := h.app.RBACService.AllowedStackNames(r.Context(), user.ID, envID, rbac.PermStacksView)
			if err != nil {
				h.app.Logger.Error("failed to load allowed stacks", "env", envID, "error", err)
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
	// The gate decided on ?env=, so the stack is created there: a body naming another
	// environment would deploy where the caller holds no grant.
	if envID := response.ParseEnvID(r); envID != "" {
		if req.EnvironmentID != nil && *req.EnvironmentID != "" && *req.EnvironmentID != envID {
			rbac.RecordDenied(r, user, rbac.PermStacksManage, *req.EnvironmentID)
			response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
			return
		}
		req.EnvironmentID = &envID
	}

	st, err := h.service.Create(r.Context(), req)
	if err != nil {
		if writeRefusalError(w, r, err) {
			return
		}
		if writeComposeMaskError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to create stack", "name", req.Name, "error", err)
		response.BadRequestCause(w, r, i18n.ErrStackCreateFailed, err)
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

// SetOwnerRequest names the user who owns a stack from now on.
type SetOwnerRequest struct {
	OwnerID string `json:"ownerId"`
}

// HandleSetOwner updates the owner of a stack. Phase 4 of the RBAC
// roadmap. The body is `{"ownerId": "..."}` or `{"ownerId": ""}` to
// clear ownership. Gated by users.manage — only admins who already
// have full user-management rights can reassign owners.
func (h *Handler) HandleSetOwner(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req SetOwnerRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.OwnerID != "" {
		// Validate the target user is real and active. Reject any
		// garbage IDs early so we don't end up with an FK violation
		// surfacing as a 500.
		active, err := h.service.UserIsActive(r.Context(), req.OwnerID)
		if err != nil {
			h.app.Logger.Error("stacks: validate owner", "error", err, "ownerId", req.OwnerID)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if !active {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
	}

	envID := response.ParseEnvID(r)
	if envID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.service.SetOwner(r.Context(), envID, name, req.OwnerID); err != nil {
		if errors.Is(err, ErrStackNotFound) {
			response.NotFoundCode(w, r, i18n.ErrStackNotFound)
			return
		}
		h.app.Logger.Error("stacks: set owner", "error", err, "stack", name)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "set_owner",
		EntityType: "stack",
		EntityID:   name,
		EntityName: name,
		Details:    fmt.Sprintf("owner set to %q", req.OwnerID),
	})

	response.OK(w, map[string]any{"name": name, "ownerId": req.OwnerID})
}

// HandleUpdate updates the compose content or description of a stack.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	if !h.guardSelfStack(w, r, name, "update") {
		return
	}

	var req UpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	st, err := h.service.Update(r.Context(), name, req)
	if err != nil {
		if writeRefusalError(w, r, err) {
			return
		}
		if writeProtectedError(w, r, err) {
			return
		}
		if errors.Is(err, envredact.ErrComposeMaskedValue) || errors.Is(err, envredact.ErrComposeMaskedMoved) {
			h.app.Logger.Warn("stack compose save refused: masked value", "name", name, "error", err)
			writeComposeMaskError(w, r, err)
			return
		}
		h.app.Logger.Error("failed to update stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		h.service.FireWebhooks(r.Context(), st.ID, "update")
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
	if !h.guardSelfStack(w, r, name, "delete") {
		return
	}
	envID := response.ParseEnvID(r)

	if err := h.service.RemoveStack(r.Context(), envID, name); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to remove stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackRemoveFailed, err)
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
	if !h.guardSelfStack(w, r, name, "up") {
		return
	}
	result := h.service.Up(r.Context(), name)

	h.app.Logger.Info("stack up", "name", name, "success", result.Success, "user", user.Username)
	if !result.Success {
		if writeComposeRefusal(w, r, result) {
			return
		}
		if result.Error == coredocker.ErrProtectedResource.Error() {
			response.ForbiddenCode(w, r, i18n.ErrProtectedTarget)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrStackDeployFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "up",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(r.Context(), name); stackID != "" {
		h.service.FireWebhooks(r.Context(), stackID, "up")
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
	if !h.guardSelfStack(w, r, name, "stop") {
		return
	}
	envID := response.ParseEnvID(r)

	if err := h.service.StopStack(r.Context(), envID, name); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to stop stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackStopFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "stop",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(r.Context(), name); stackID != "" {
		h.service.FireWebhooks(r.Context(), stackID, "stop")
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
	if !h.guardSelfStack(w, r, name, "down") {
		return
	}
	envID := response.ParseEnvID(r)

	if err := h.service.DownStack(r.Context(), envID, name); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to down stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackDownFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "down",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(r.Context(), name); stackID != "" {
		h.service.FireWebhooks(r.Context(), stackID, "down")
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
	if !h.guardSelfStack(w, r, name, "restart") {
		return
	}
	envID := response.ParseEnvID(r)

	if err := h.service.RestartStack(r.Context(), envID, name); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to restart stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackRestartFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "restart",
		EntityType: "stack",
		EntityName: name,
	})

	if stackID := h.service.StackIDByName(r.Context(), name); stackID != "" {
		h.service.FireWebhooks(r.Context(), stackID, "restart")
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
	if !h.guardSelfStack(w, r, name, "managed_update") {
		return
	}

	result, err := h.service.UpdateManagedStack(r.Context(), name)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to update managed stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackUpdateFailed, err)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}
	if writeComposeRefusal(w, r, result) {
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

	if stackID := h.service.StackIDByName(r.Context(), name); stackID != "" {
		h.service.FireWebhooks(r.Context(), stackID, "update")
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
	if !h.guardSelfStack(w, r, name, "reinstall") {
		return
	}

	result, err := h.service.ReinstallManagedStack(r.Context(), name)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to reinstall managed stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackDeployFailed, err)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}
	if writeComposeRefusal(w, r, result) {
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

	if stackID := h.service.StackIDByName(r.Context(), name); stackID != "" {
		h.service.FireWebhooks(r.Context(), stackID, "deploy")
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if containers == nil {
		containers = []container.Summary{}
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
	rows, err := h.service.EnvVarRows(r.Context(), name)
	if err != nil {
		h.app.Logger.Error("failed to get stack env vars", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	// Credential-looking values are masked by default. ?reveal=true returns
	// them in the clear only to a caller holding stacks.manage on this stack:
	// stacks.view does not deploy, so it never reached these values before
	// they were shown here. McHarbor's own keys stay masked either way.
	reveal := r.URL.Query().Get("reveal") == "true"
	if reveal && !h.mayRevealStackEnv(w, r, user, name) {
		return
	}

	pinned := stackPinned(h.app)
	var vars map[string]string
	if reveal {
		vars = map[string]string{}
		for _, row := range rows {
			vars[row.Key] = row.Value
		}
		h.auditStackReveal(r, name, "reveal_env_vars", "")
		h.app.Logger.Info("stack env vars revealed", "name", name, "user", user.Username)
	} else {
		// The stored is_secret flag predates the shared classifier; the policy on top masks
		// what it knows that the flag does not (a token by shape, a connection string).
		vars, _ = envredact.Policy{Pinned: pinned}.EnvMap(maskEnvVars(rows))
	}
	vars = withholdRootOfTrust(vars, pinned)

	response.NoStore(w)
	response.OK(w, vars)
}

// mayRevealStackEnv answers a reveal request itself when the caller lacks stacks.manage on
// the stack, and reports whether the handler may continue. It fails closed.
func (h *Handler) mayRevealStackEnv(w http.ResponseWriter, r *http.Request, user *auth.User, name string) bool {
	if user.ID == "system" {
		return true
	}
	envID := response.ParseEnvID(r)
	allowed := false
	if h.app.RBACService != nil {
		var err error
		allowed, err = h.app.RBACService.HasStackPermission(r.Context(), user.ID, envID, name, rbac.PermStacksManage)
		if err != nil {
			h.app.Logger.Error("checking the stack env reveal permission failed", "name", name, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return false
		}
	}
	if !allowed {
		rbac.RecordDenied(r, user, rbac.PermStacksManage, envID)
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	return true
}

// auditStackReveal records one stack reveal at warning severity with the stack's id and its
// own environment, so the trail says which stack in which environment was read.
func (h *Handler) auditStackReveal(r *http.Request, name, action, details string) {
	if h.app.AuditLog == nil {
		return
	}
	id, envID, err := h.service.StackIdentity(r.Context(), name)
	if err != nil {
		h.app.Logger.Warn("stacks: reading the stack identity for an audit row failed", "name", name, "error", err)
	}
	if envID == "" {
		envID = response.ParseEnvID(r)
	}
	if details == "" {
		details = "env values revealed"
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		Severity:      "warning",
		EntityType:    "stack",
		EntityID:      id,
		EntityName:    name,
		Details:       details,
		EnvironmentID: envID,
	})
}

// mayRevealContainerEnv answers the request itself when the caller lacks containers.manage on
// envID, the key that reveals a container's environment, and reports whether the handler may
// continue. It fails closed.
func (h *Handler) mayRevealContainerEnv(w http.ResponseWriter, r *http.Request, user *auth.User, envID string) bool {
	if user.ID == "system" {
		return true
	}
	if h.app.RBACService == nil {
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	allowed, err := h.app.RBACService.AllowedOn(r, envID, rbac.PermContainersManage)
	if err != nil {
		h.app.Logger.Error("checking containers.manage for an adopt reveal failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	if !allowed {
		rbac.RecordDenied(r, user, rbac.PermContainersManage, envID)
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	return true
}

// HandleGetCompose returns the compose file with credential-looking environment values
// masked; ?reveal=true returns them to a stacks.manage holder on the stack and is audited.
// McHarbor's own keys stay masked either way.
func (h *Handler) HandleGetCompose(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	reveal := r.URL.Query().Get("reveal") == "true"
	if reveal && !h.mayRevealStackEnv(w, r, user, name) {
		return
	}
	content, err := h.service.ComposeContent(r.Context(), name)
	if err != nil {
		h.app.Logger.Error("failed to get compose content", "name", name, "error", err)
		response.NotFoundCause(w, r, i18n.ErrStackComposeFailed, err)
		return
	}

	policy := envredact.Policy{Reveal: reveal, Pinned: stackPinned(h.app)}
	detector := composeDetector(name, h.service.liveMcHarborServices(r.Context(), response.ParseEnvID(r), name))
	content, withheld := policy.Compose(content, detector)
	if reveal {
		details := "compose values revealed"
		if len(withheld) > 0 {
			details += "; still withheld: " + strings.Join(withheld, ",")
		}
		h.auditStackReveal(r, name, "reveal_compose", details)
	}

	response.NoStore(w)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	envID := response.ParseEnvID(r)

	// The preview reconstructs running containers, credentials included, so values come
	// masked; ?reveal=true needs containers.manage — the key that reveals a container's
	// env — and is audited like every other reveal.
	reveal := r.URL.Query().Get("reveal") == "true"
	var preview *AdoptPreviewResponse
	var err error
	if reveal {
		if !h.mayRevealContainerEnv(w, r, user, envID) {
			return
		}
		preview, err = h.service.PreviewAdoptRevealed(r.Context(), envID, req)
	} else {
		preview, err = h.service.PreviewAdopt(r.Context(), envID, req)
	}
	if err == nil && reveal {
		target := req.StackName
		if target == "" {
			target = req.ContainerID
		}
		h.app.AuditLog.Log(r, audit.Entry{
			Action:        "reveal_adopt_preview",
			Severity:      "warning",
			EntityType:    "stack",
			EntityName:    target,
			Details:       "adopt preview values revealed",
			EnvironmentID: envID,
		})
	}
	if err != nil {
		h.app.Logger.Error("failed to preview adopt", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackPreviewFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
	if !h.guardSelfAdopt(w, r, envID, req) {
		return
	}

	// A masked preview is restored from the running containers, which reads their
	// credentials into a stack file; that needs the same containers.manage a reveal does.
	restoring := strings.Contains(req.Compose, envredact.Marker)
	if restoring && !h.mayRevealContainerEnv(w, r, user, envID) {
		return
	}

	st, err := h.service.Adopt(r.Context(), envID, req)
	if err != nil {
		if errors.Is(err, ErrStackAlreadyManaged) {
			response.ConflictCode(w, r, i18n.ErrStackAlreadyManaged)
			return
		}
		if writeComposeMaskError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to adopt stack", "name", req.Name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackAdoptFailed, err)
		return
	}

	adoptEntry := audit.Entry{
		Action:        "adopt",
		EntityType:    "stack",
		EntityID:      st.ID,
		EntityName:    req.Name,
		EnvironmentID: envID,
	}
	if restoring {
		adoptEntry.Severity = "warning"
		adoptEntry.Details = "withheld values restored from the adopted containers"
	}
	h.app.AuditLog.Log(r, adoptEntry)

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
	link, err := h.service.ContainerStackLink(r.Context(), envID, containerID)
	if err != nil {
		h.app.Logger.Error("failed to get container stack link", "container", containerID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackLinkFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
		response.InternalErrorCause(w, r, i18n.ErrStackLinkFailed, err)
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
	if err := h.service.UnlinkContainer(r.Context(), envID, containerID); err != nil {
		h.app.Logger.Error("failed to unlink container from stack", "container", containerID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackLinkFailed, err)
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
	stackID := h.service.StackIDByName(r.Context(), name)
	if stackID == "" {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}

	webhooks, err := h.service.ListWebhooks(r.Context(), stackID)
	if err != nil {
		h.app.Logger.Error("failed to list webhooks", "stack", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
	stackID := h.service.StackIDByName(r.Context(), name)
	if stackID == "" {
		response.NotFoundCode(w, r, i18n.ErrStackNotFound)
		return
	}

	var input CreateStackWebhookInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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

	wh, err := h.service.CreateWebhook(r.Context(), stackID, input)
	if err != nil {
		h.app.Logger.Error("failed to create webhook", "stack", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	wh, err := h.service.UpdateWebhook(r.Context(), webhookID, input)
	if err != nil {
		h.app.Logger.Error("failed to update webhook", "webhookId", webhookID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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

	if err := h.service.DeleteWebhook(r.Context(), webhookID); err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			response.NotFoundCode(w, r, i18n.ErrStackWebhookNotFound)
			return
		}
		h.app.Logger.Error("failed to delete webhook", "webhookId", webhookID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.InternalErrorCause(w, r, i18n.ErrStackWebhookFailed, err)
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
	if !h.guardSelfStack(w, r, name, "prune") {
		return
	}
	envID := response.ParseEnvID(r)

	result, err := h.service.PruneOrphans(r.Context(), envID, name)
	if err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to prune stack", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackPruneFailed, err)
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
	if !h.guardSelfStack(w, r, name, "update_env_vars") {
		return
	}

	var envVars map[string]string
	if err := response.DecodeBody(r, &envVars); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if err := h.service.UpdateEnvVars(r.Context(), name, envVars); err != nil {
		if writeProtectedError(w, r, err) {
			return
		}
		h.app.Logger.Error("failed to update env vars", "name", name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackEnvVarsFailed, err)
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

// CheckImageUpdatesRequest names the stacks whose images to check; empty means all.
type CheckImageUpdatesRequest struct {
	StackNames []string `json:"stackNames"`
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

	var req CheckImageUpdatesRequest
	if err := response.DecodeBody(r, &req); err != nil {
		// Allow an empty body and check all stacks.
		req.StackNames = nil
	}
	if user.ID != "system" {
		allowedAll, err := h.app.RBACService.HasPermission(r.Context(), user.ID, envID, rbac.PermStacksView)
		if err != nil {
			h.app.Logger.Error("failed to evaluate stack permissions", "env", envID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if !allowedAll {
			allowedStackNames, err := h.app.RBACService.AllowedStackNames(r.Context(), user.ID, envID, rbac.PermStacksView)
			if err != nil {
				h.app.Logger.Error("failed to load allowed stacks", "env", envID, "error", err)
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			req.StackNames = restrictStackNames(req.StackNames, allowedStackNames)
		}
	}

	results, err := h.service.CheckImageUpdates(r.Context(), envID, req.StackNames)
	if err != nil {
		h.app.Logger.Error("check stack image updates failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackUpdateCheckFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrStackLogsFailed, err)
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
