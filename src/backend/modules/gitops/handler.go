// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds GitOps HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler constructs a GitOps handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
	}
}

// HandleListPipelines returns all gitops pipelines.
func (h *Handler) HandleListPipelines(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	pipelines, err := h.service.ListPipelines()
	if err != nil {
		h.app.Logger.Error("gitops: list pipelines error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGitOpsListFailed)
		return
	}
	response.OK(w, pipelines)
}

// HandleGetPipeline returns a single pipeline with stages.
func (h *Handler) HandleGetPipeline(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	pipeline, err := h.service.PipelineByID(id)
	if err != nil {
		h.app.Logger.Error("gitops: get pipeline error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGitOpsListFailed)
		return
	}
	if pipeline == nil {
		response.NotFoundCode(w, r, i18n.ErrGitOpsPipelineNotFound)
		return
	}
	response.OK(w, pipeline)
}

// HandleCreatePipeline creates a new pipeline.
func (h *Handler) HandleCreatePipeline(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in CreatePipelineInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	pipeline, err := h.service.CreatePipeline(in)
	if err != nil {
		h.app.Logger.Error("gitops: create pipeline error", "error", err)
		switch err.Error() {
		case "pipeline name is required":
			response.BadRequestCode(w, r, i18n.ErrGitOpsNameRequired)
			return
		case "repo does not exist":
			response.BadRequestCode(w, r, i18n.ErrGitOpsRepoMissing)
			return
		case "at least one stage is required", "stage 0: name is required",
			"stage 0: duplicate index 0", "stage 0: target environment is required":
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrGitOpsCreateFailed)
		return
	}
	response.Created(w, pipeline)
}

// HandleUpdatePipeline updates a pipeline.
func (h *Handler) HandleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var in UpdatePipelineInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	pipeline, err := h.service.UpdatePipeline(id, in)
	if err != nil {
		h.app.Logger.Error("gitops: update pipeline error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGitOpsUpdateFailed)
		return
	}
	if pipeline == nil {
		response.NotFoundCode(w, r, i18n.ErrGitOpsPipelineNotFound)
		return
	}
	response.OK(w, pipeline)
}

// HandleDeletePipeline removes a pipeline.
func (h *Handler) HandleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.service.DeletePipeline(id); err != nil {
		h.app.Logger.Error("gitops: delete pipeline error", "error", err, "id", id)
		if err.Error() == "sql: no rows in result set" {
			response.NotFoundCode(w, r, i18n.ErrGitOpsPipelineNotFound)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrGitOpsRemoveFailed)
		return
	}
	response.NoContent(w)
}

// HandlePromoteStage executes a promotion to a stage.
func (h *Handler) HandlePromoteStage(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var in PromoteInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if in.TriggeredBy == "" {
		in.TriggeredBy = user.ID
	}
	prom, err := h.service.Promote(r.Context(), id, in)
	if err != nil {
		h.app.Logger.Error("gitops: promote stage error", "error", err, "id", id)
		if err.Error() == "pipeline not found" {
			response.NotFoundCode(w, r, i18n.ErrGitOpsPipelineNotFound)
			return
		}
		if err.Error() == "stage not found in pipeline" {
			response.NotFoundCode(w, r, i18n.ErrGitOpsStageNotFound)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrGitOpsPromoteFailed)
		return
	}
	response.OK(w, prom)
}

// HandleListPromotions returns recent promotions for a pipeline.
func (h *Handler) HandleListPromotions(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)
	promotions, total, err := h.service.ListPromotions(id, page, perPage)
	if err != nil {
		h.app.Logger.Error("gitops: list promotions error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGitOpsListFailed)
		return
	}
	response.Paginated(w, promotions, total, page, perPage)
}

// HandleListApprovals returns pending approval requests.
func (h *Handler) HandleListApprovals(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	approvals, err := h.service.ListApprovals(status)
	if err != nil {
		h.app.Logger.Error("gitops: list approvals error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGitOpsListFailed)
		return
	}
	response.OK(w, approvals)
}

// HandleResolveApproval approves or rejects a pending approval.
func (h *Handler) HandleResolveApproval(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	action := chi.URLParam(r, "action")
	id := chi.URLParam(r, "id")
	var in ApprovalInput
	_ = response.DecodeBody(r, &in)
	if in.ResolvedBy == "" {
		in.ResolvedBy = user.ID
	}

	approval, err := h.service.ResolveApproval(id, action, in.ResolvedBy, in.Note)
	if err != nil {
		h.app.Logger.Error("gitops: approval error", "error", err, "id", id, "action", action)
		if err.Error() == "approval not found or already resolved" {
			response.NotFoundCode(w, r, i18n.ErrGitOpsApprovalNotFound)
			return
		}
		if action == "approved" {
			response.InternalErrorCode(w, r, i18n.ErrGitOpsApproveFailed)
		} else {
			response.InternalErrorCode(w, r, i18n.ErrGitOpsRejectFailed)
		}
		return
	}
	response.OK(w, approval)
}

// HandleResolveApprovalFor returns a handler bound to a specific action
// (approved | rejected). Used by routes.go to register two distinct
// endpoints — POST /api/gitops/approvals/{id}/approve and
// POST /api/gitops/approvals/{id}/reject.
func (h *Handler) HandleResolveApprovalFor(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.RequireAuth(r)
		if user == nil {
			response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
			return
		}
		id := chi.URLParam(r, "id")
		var in ApprovalInput
		_ = response.DecodeBody(r, &in)
		if in.ResolvedBy == "" {
			in.ResolvedBy = user.ID
		}
		approval, err := h.service.ResolveApproval(id, action, in.ResolvedBy, in.Note)
		if err != nil {
			h.app.Logger.Error("gitops: approval error", "error", err, "id", id, "action", action)
			if err.Error() == "approval not found or already resolved" {
				response.NotFoundCode(w, r, i18n.ErrGitOpsApprovalNotFound)
				return
			}
			if action == "approved" {
				response.InternalErrorCode(w, r, i18n.ErrGitOpsApproveFailed)
			} else {
				response.InternalErrorCode(w, r, i18n.ErrGitOpsRejectFailed)
			}
			return
		}
		response.OK(w, approval)
	}
}

// HandleListPRPreviews returns recent PR preview environments.
func (h *Handler) HandleListPRPreviews(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	pipelineID := r.URL.Query().Get("pipelineId")
	previews, err := h.service.ListPRPreviews(pipelineID)
	if err != nil {
		h.app.Logger.Error("gitops: list pr previews error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGitOpsListFailed)
		return
	}
	response.OK(w, previews)
}

// HandlePullRequestWebhook receives GitHub/GitLab/Gitea PR events.
// This endpoint is unauthenticated by design (token-verified via signature).
func (h *Handler) HandlePullRequestWebhook(w http.ResponseWriter, r *http.Request) {
	var in PullRequestInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	secret := r.Header.Get("X-Hub-Signature-256")
	if secret == "" {
		secret = r.Header.Get("X-Gitlab-Token")
	}
	preview, err := h.service.HandlePullRequestWebhook(in, secret, "")
	if err != nil {
		h.app.Logger.Error("gitops: pr webhook error", "error", err, "action", in.Action, "pr", in.PRNumber)
		response.BadRequestCode(w, r, i18n.ErrGitOpsWebhookFailed)
		return
	}
	response.OK(w, preview)
}
