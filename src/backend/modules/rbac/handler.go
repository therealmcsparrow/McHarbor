// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package rbac exposes role, permission and grant queries over HTTP.
package rbac

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler exposes /api/rbac/policies CRUD endpoints to admins.
type Handler struct {
	app     *router.AppDeps
	service *rbac.PolicyService
	logger  *slog.Logger
}

// NewHandler wires up the policy handler with the shared app deps.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: rbac.NewPolicyService(app.DB),
		logger:  app.Logger,
	}
}

// HandleList returns all ABAC policies.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("rbac: list policies", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if out == nil {
		out = []rbac.ABACPolicy{}
	}
	response.OK(w, out)
}

// HandleGet returns a single policy by id.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	policy, err := h.service.Get(r.Context(), id)
	if errors.Is(err, rbac.ErrPolicyNotFound) {
		response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("rbac: get policy", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, policy)
}

// HandleHistory returns the audit trail for a policy.
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.service.History(r.Context(), id, 100)
	if err != nil {
		h.app.Logger.Error("rbac: list policy history", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if out == nil {
		out = []rbac.PolicyHistoryEntry{}
	}
	response.OK(w, out)
}

// HandleCreate validates and inserts a new policy.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input PolicyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	policy, err := h.service.Create(r.Context(), rbac.CreateInput{
		Name:         strings.TrimSpace(input.Name),
		Description:  input.Description,
		Effect:       input.Effect,
		ResourceType: input.ResourceType,
		Expression:   input.Expression,
		Priority:     input.Priority,
		Enabled:      input.Enabled,
		Permissions:  dedup(input.Permissions),
		CreatedBy:    user.ID,
	})
	if err != nil {
		h.respondPolicyError(w, r, err)
		return
	}

	entry := audit.Entry{
		Action:     "create",
		EntityType: "rbac_policy",
		EntityID:   policy.ID,
		EntityName: policy.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(nil, policy); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("rbac: audit diff error", "error", diffErr, "id", policy.ID)
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, entry)
	response.Created(w, policy)
}

// HandleUpdate modifies an existing policy.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")

	var input PolicyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	existing, err := h.service.Get(r.Context(), id)
	if errors.Is(err, rbac.ErrPolicyNotFound) {
		response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("rbac: get policy before update", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	policy, err := h.service.Update(r.Context(), id, rbac.UpdateInput{
		Name:         strings.TrimSpace(input.Name),
		Description:  input.Description,
		Effect:       input.Effect,
		ResourceType: input.ResourceType,
		Expression:   input.Expression,
		Priority:     input.Priority,
		Enabled:      input.Enabled,
		Permissions:  dedup(input.Permissions),
		ActorID:      user.ID,
	})
	if err != nil {
		h.respondPolicyError(w, r, err)
		return
	}

	entry := audit.Entry{
		Action:     "update",
		EntityType: "rbac_policy",
		EntityID:   policy.ID,
		EntityName: policy.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, policy); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("rbac: audit diff error", "error", diffErr, "id", policy.ID)
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, entry)
	response.OK(w, policy)
}

// HandleDelete removes a policy.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.service.Get(r.Context(), id)
	if errors.Is(err, rbac.ErrPolicyNotFound) {
		response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("rbac: get policy before delete", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if err := h.service.Delete(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, rbac.ErrPolicyNotFound) {
			response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
			return
		}
		h.app.Logger.Error("rbac: delete policy", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	entry := audit.Entry{
		Action:     "delete",
		EntityType: "rbac_policy",
		EntityID:   id,
		EntityName: existing.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, nil); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("rbac: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, entry)
	response.OKMsg(w, r, i18n.MsgRBACPolicyRemoved)
}

// ToggleEnableRequest switches a policy on or off.
type ToggleEnableRequest struct {
	Enabled bool `json:"enabled"`
}

// HandleToggleEnable flips the enabled flag on a policy.
func (h *Handler) HandleToggleEnable(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")

	var input ToggleEnableRequest
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if err := h.service.SetEnabled(r.Context(), id, input.Enabled, user.ID); err != nil {
		if errors.Is(err, rbac.ErrPolicyNotFound) {
			response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
			return
		}
		h.app.Logger.Error("rbac: toggle policy", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	policy, err := h.service.Get(r.Context(), id)
	if err != nil {
		response.OK(w, map[string]any{"enabled": input.Enabled})
		return
	}
	response.OK(w, policy)
}

// HandleUserLabelsGet returns a user's free-form label bag. Used by
// the ABAC editor when an admin is authoring a policy that matches
// user.labels. Endpoint is gated by PermRbacPoliciesManage so only
// admins with policy access can read the labels.
func (h *Handler) HandleUserLabelsGet(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	labels, err := h.app.RBACService.UserLabels(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("rbac: load user labels", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]any{"labels": labels})
}

// HandleUserLabelsPut replaces the user's label bag. Empty keys are
// removed, so passing {"labels": {"team": ""}} clears the "team" label.
func (h *Handler) HandleUserLabelsPut(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	userID := chi.URLParam(r, "userID")

	var input UserLabelsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if len(input.Labels) > 256 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	before, err := h.app.RBACService.UserLabels(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("rbac: load user labels before save", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if err := h.app.RBACService.SetUserLabels(r.Context(), userID, input.Labels); err != nil {
		h.app.Logger.Error("rbac: save user labels", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	labels, err := h.app.RBACService.UserLabels(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("rbac: reload user labels after save", "error", err)
	}
	entry := audit.Entry{
		Action:     "update",
		EntityType: "user_labels",
		EntityID:   userID,
		EntityName: h.app.RBACService.Username(r.Context(), userID),
	}
	if diffEntry, diffErr := entry.WithDiff(before, labels); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("rbac: audit diff error", "error", diffErr, "id", userID)
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, entry)

	response.OK(w, map[string]any{"labels": labels})
}

// HandleTest dry-runs a policy expression against an admin-supplied
// attribute bundle. Used by the "Test" button in the policy editor.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	var input TestInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	id := chi.URLParam(r, "id")

	policy, err := h.service.Get(r.Context(), id)
	if errors.Is(err, rbac.ErrPolicyNotFound) {
		response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
		return
	}
	if err != nil {
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	attrs := convertAttributes(input.Attributes)
	exec := rbac.NewABACExecutor(h.logger)
	res := exec.Evaluate(r.Context(), policy.Expression, attrs)

	out := TestResult{Matched: res.Matched, Reason: res.Reason}
	if res.Err != nil {
		out.Error = res.Err.Error()
	}
	response.OK(w, out)
}

// respondPolicyError maps a service-layer error onto the right i18n code.
func (h *Handler) respondPolicyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, rbac.ErrPolicyNotFound):
		response.NotFoundCode(w, r, i18n.ErrRBACPolicyNotFound)
	case errors.Is(err, rbac.ErrABACInvalidExpression):
		response.BadRequestCode(w, r, i18n.ErrRBACPolicyInvalid)
	case errors.Is(err, rbac.ErrABACExecutionTimeout):
		response.BadRequestCode(w, r, i18n.ErrRBACPolicyTimeout)
	default:
		// Validation errors from the service layer bubble up as
		// "policy name is required" etc. Surface them as 400.
		msg := err.Error()
		if strings.HasPrefix(msg, "policy ") {
			response.BadRequestCode(w, r, i18n.ErrRBACPolicyInvalid)
			return
		}
		h.app.Logger.Error("rbac: policy service", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
}

// convertAttributes maps the DTO into the rbac.Attributes struct.
func convertAttributes(in *AttributesDTO) *rbac.Attributes {
	if in == nil {
		return &rbac.Attributes{}
	}
	out := &rbac.Attributes{}
	if in.User != nil {
		out.User = &rbac.UserAttributes{
			ID: in.User.ID, Username: in.User.Username, Email: in.User.Email,
			Roles: in.User.Roles, Groups: in.User.Groups,
			Labels: in.User.Labels,
		}
	}
	if in.Env != nil {
		out.Env = &rbac.EnvAttributes{ID: in.Env.ID, Name: in.Env.Name, Type: in.Env.Type}
	}
	if in.Resource != nil {
		out.Resource = &rbac.ResourceAttributes{
			Type: in.Resource.Type, ID: in.Resource.ID, Owner: in.Resource.Owner,
			Image: in.Resource.Image, State: in.Resource.State,
			Labels: in.Resource.Labels,
		}
	}
	if in.Now != nil {
		out.Now = &rbac.TimeAttributes{
			Year: in.Now.Year, Month: in.Now.Month, Day: in.Now.Day,
			Hour: in.Now.Hour, Weekday: in.Now.Weekday,
		}
	}
	return out
}

// dedup trims and de-duplicates a string slice, preserving order.
func dedup(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// (compile-time check to ensure the test handler does not lose refs)
var _ = fmt.Sprintf
