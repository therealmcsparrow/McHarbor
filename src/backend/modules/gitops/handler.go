// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
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
		service: NewService(app.DB, app.Encryption),
	}
}

// HandleListPipelines returns all gitops pipelines.
func (h *Handler) HandleListPipelines(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	pipelines, err := h.service.ListPipelinesCtx(r.Context())
	if err != nil {
		h.app.Logger.Error("gitops: list pipelines error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
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
	pipeline, err := h.service.PipelineByIDCtx(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("gitops: get pipeline error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	pipeline, err := h.service.CreatePipeline(r.Context(), auth.RequireAuth(r).ID, in)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	pipeline, err := h.service.UpdatePipeline(r.Context(), auth.RequireAuth(r).ID, id, in)
	if err != nil {
		h.app.Logger.Error("gitops: update pipeline error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsUpdateFailed, err)
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
	if err := h.service.DeletePipeline(r.Context(), id); err != nil {
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

// HandlePromoteStage records a promotion and starts its deploy in the background,
// answering with the 'pending' row: a compose pull can outlast the server's write
// timeout, so the outcome is read from the promotion history rather than awaited.
func (h *Handler) HandlePromoteStage(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var in PromoteInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if in.TriggeredBy == "" {
		in.TriggeredBy = user.ID
	}
	prom, err := h.service.StartPromotion(r.Context(), id, in)
	if err != nil {
		h.app.Logger.Error("gitops: promote stage error", "error", err, "id", id)
		if errors.Is(err, ErrPipelineNotFound) {
			response.NotFoundCode(w, r, i18n.ErrGitOpsPipelineNotFound)
			return
		}
		if errors.Is(err, ErrStageNotFound) {
			response.NotFoundCode(w, r, i18n.ErrGitOpsStageNotFound)
			return
		}
		response.InternalErrorCause(w, r, i18n.ErrGitOpsPromoteFailed, err)
		return
	}
	if h.app.AuditLog != nil {
		if err := h.app.AuditLog.RecordContext(r.Context(), user.ID, user.Username, audit.Entry{
			Action:        "gitops.promotion.started",
			Severity:      "info",
			EntityType:    "gitops_pipeline",
			EntityID:      id,
			EnvironmentID: prom.EnvironmentID,
			Details:       "promotion " + prom.ID + " of commit " + prom.CommitSHA + " to stack " + prom.StackName,
		}); err != nil {
			h.app.Logger.Warn("gitops: auditing the promotion failed", "error", err)
		}
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
	promotions, total, err := h.service.ListPromotions(r.Context(), id, page, perPage)
	if err != nil {
		h.app.Logger.Error("gitops: list promotions error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
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
	approvals, err := h.service.ListApprovals(r.Context(), status)
	if err != nil {
		h.app.Logger.Error("gitops: list approvals error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
		return
	}
	response.OK(w, approvals)
}

// HandleResolveApproval approves or rejects a pending approval named by the
// {action} route parameter; the fixed routes use HandleApprove and HandleReject.
func (h *Handler) HandleResolveApproval(w http.ResponseWriter, r *http.Request) {
	h.resolveApproval(w, r, chi.URLParam(r, "action"))
}

// HandleApprove approves a pending promotion approval.
func (h *Handler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	h.resolveApproval(w, r, "approved")
}

// HandleReject rejects a pending promotion approval.
func (h *Handler) HandleReject(w http.ResponseWriter, r *http.Request) {
	h.resolveApproval(w, r, "rejected")
}

// resolveApproval records who resolved an approval as the authenticated caller,
// never a name from the request body, which any approver could otherwise forge.
func (h *Handler) resolveApproval(w http.ResponseWriter, r *http.Request, action string) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var in ApprovalInput
	if err := response.DecodeBody(r, &in); err != nil && r.ContentLength > 0 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	approval, err := h.service.ResolveApproval(r.Context(), id, action, user.ID, in.Note)
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

// HandleListPRPreviews returns recent PR preview environments.
func (h *Handler) HandleListPRPreviews(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	pipelineID := r.URL.Query().Get("pipelineId")
	previews, err := h.service.ListPRPreviews(r.Context(), pipelineID)
	if err != nil {
		h.app.Logger.Error("gitops: list pr previews error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
		return
	}
	response.OK(w, previews)
}

// maxWebhookBody bounds what an unauthenticated caller can make the server
// read. A pull-request payload from any forge is far under this.
const maxWebhookBody = 1 << 20

// HandlePullRequestWebhook receives GitHub, GitLab and Gitea pull-request events.
// The route is public because a forge cannot present a session; what
// authenticates it is an HMAC over the raw body, verified against the
// repository's own stored secret before the payload is acted on.
func (h *Handler) HandlePullRequestWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	// The repository is named by the query string the forge is configured with,
	// or by the native payload the CLI sends. It is read before verification only
	// to find which secret to verify against; nothing is acted on until the HMAC
	// over these exact bytes matches.
	repoID := r.URL.Query().Get("repo")
	if repoID == "" {
		var peek struct {
			RepoID string `json:"repoId"`
		}
		if err := json.Unmarshal(body, &peek); err == nil {
			repoID = peek.RepoID
		}
	}
	if repoID == "" {
		response.BadRequestCode(w, r, i18n.ErrGitOpsRepoMissing)
		return
	}

	delivery := WebhookDelivery{
		RepoID:      repoID,
		Body:        body,
		Event:       firstHeader(r, "X-GitHub-Event", "X-Gitea-Event", "X-Gitlab-Event"),
		HMACHeader:  firstHeader(r, "X-Hub-Signature-256", "X-Gitea-Signature"),
		TokenHeader: r.Header.Get("X-Gitlab-Token"),
	}

	preview, err := h.service.HandlePullRequestWebhook(r.Context(), delivery)
	if err != nil {
		if errors.Is(err, ErrWebhookSecretMissing) || errors.Is(err, ErrWebhookSignatureInvalid) {
			h.app.Logger.Warn("gitops: pr webhook refused", "repo", repoID, "error", err)
			response.UnauthorizedCode(w, r, i18n.ErrGitOpsWebhookSignatureBad)
			return
		}
		h.app.Logger.Error("gitops: pr webhook error", "error", err, "repo", repoID)
		response.BadRequestCause(w, r, i18n.ErrGitOpsWebhookFailed, err)
		return
	}
	// An action the lifecycle does not care about is a success, not a failure: a
	// 400 here shows in the forge as a broken delivery for every label change.
	if preview == nil {
		response.OKMsg(w, r, i18n.MsgGitOpsWebhookIgnored)
		return
	}
	response.OK(w, preview)
}

// firstHeader returns the first of names that the request actually carries.
func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if v := r.Header.Get(name); v != "" {
			return v
		}
	}
	return ""
}

// HandleDestroyPreview tears a preview down on an operator's instruction. It is
// idempotent, so repeating a request whose response was lost is a success.
func (h *Handler) HandleDestroyPreview(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	preview, err := h.service.PreviewByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("gitops: reading preview failed", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
		return
	}
	if preview == nil {
		response.NotFoundCode(w, r, i18n.ErrGitOpsPreviewNotFound)
		return
	}
	if err := h.service.TeardownPreview(r.Context(), id, "manual", user.ID, ""); err != nil {
		h.app.Logger.Error("gitops: destroying preview failed", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsPreviewDestroyFailed, err)
		return
	}
	updated, err := h.service.PreviewByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("gitops: re-reading preview failed", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
		return
	}
	response.OK(w, updated)
}

// HandleWebhookSecretStatus reports whether a repository can accept a delivery.
// The secret itself is never returned: it is shown once at generation and
// redacted for the rest of its life, the way an agent token is.
func (h *Handler) HandleWebhookSecretStatus(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	repoID := chi.URLParam(r, "repoId")
	configured, err := h.service.WebhookSecretConfigured(r.Context(), repoID)
	if err != nil {
		h.app.Logger.Error("gitops: reading webhook secret status failed", "error", err, "repo", repoID)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsListFailed, err)
		return
	}
	response.OK(w, WebhookSecretStatus{RepoID: repoID, Configured: configured})
}

// HandleGenerateWebhookSecret mints a repository's webhook secret and returns the
// plaintext once. Calling it again rotates the secret, which invalidates the
// value the forge currently holds.
func (h *Handler) HandleGenerateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	repoID := chi.URLParam(r, "repoId")
	secret, err := h.service.GenerateWebhookSecret(r.Context(), repoID)
	if err != nil {
		h.app.Logger.Error("gitops: generating webhook secret failed", "error", err, "repo", repoID)
		response.BadRequestCause(w, r, i18n.ErrGitOpsRepoMissing, err)
		return
	}
	if h.app.AuditLog != nil {
		if err := h.app.AuditLog.RecordContext(r.Context(), user.ID, user.Username, audit.Entry{
			Action:     "gitops.webhook_secret.generated",
			Severity:   "warning",
			EntityType: "git_repo",
			EntityID:   repoID,
			Details:    "a new pull-request webhook secret was generated; the previous one no longer verifies",
		}); err != nil {
			h.app.Logger.Warn("gitops: auditing webhook secret generation failed", "error", err)
		}
	}
	response.Created(w, WebhookSecretGenerated{RepoID: repoID, Secret: secret})
}

// HandleClearWebhookSecret removes a repository's secret, which refuses every
// subsequent delivery for it rather than falling back to accepting unsigned ones.
func (h *Handler) HandleClearWebhookSecret(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	repoID := chi.URLParam(r, "repoId")
	if err := h.service.ClearWebhookSecret(r.Context(), repoID); err != nil {
		h.app.Logger.Error("gitops: clearing webhook secret failed", "error", err, "repo", repoID)
		response.InternalErrorCause(w, r, i18n.ErrGitOpsUpdateFailed, err)
		return
	}
	if h.app.AuditLog != nil {
		if err := h.app.AuditLog.RecordContext(r.Context(), user.ID, user.Username, audit.Entry{
			Action:     "gitops.webhook_secret.cleared",
			Severity:   "warning",
			EntityType: "git_repo",
			EntityID:   repoID,
			Details:    "the pull-request webhook secret was removed; deliveries for this repository are now refused",
		}); err != nil {
			h.app.Logger.Warn("gitops: auditing webhook secret removal failed", "error", err)
		}
	}
	response.NoContent(w)
}

// WebhookSecretStatus is what the API may say about a repository's secret: that
// one exists, never what it is.
type WebhookSecretStatus struct {
	RepoID     string `json:"repoId"`
	Configured bool   `json:"configured"`
}

// WebhookSecretGenerated carries a freshly minted secret. This is the only
// response in the product that contains it, and only at the moment of creation.
type WebhookSecretGenerated struct {
	RepoID string `json:"repoId"`
	Secret string `json:"secret"`
}
