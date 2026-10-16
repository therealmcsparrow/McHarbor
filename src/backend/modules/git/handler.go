// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for git HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new git handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Encryption),
	}
}

// HandleListRepos returns a paginated list of git repositories.
func (h *Handler) HandleListRepos(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListReposContext(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("git: list repos error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGitListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGetRepo returns a single git repo.
func (h *Handler) HandleGetRepo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo, err := h.service.RepoByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("git: get repo error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitFailed, err)
		return
	}
	if repo == nil {
		response.NotFoundCode(w, r, i18n.ErrGitNotFound)
		return
	}

	response.OK(w, repo)
}

// HandleCreateRepo adds a new git repository.
func (h *Handler) HandleCreateRepo(w http.ResponseWriter, r *http.Request) {
	var input CreateRepoInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrGitNameRequired)
		return
	}
	if input.URL == "" {
		response.BadRequestCode(w, r, i18n.ErrGitUrlRequired)
		return
	}

	repo, err := h.service.CreateRepo(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("git: create repo error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGitCreateFailed, err)
		return
	}

	response.Created(w, repo)
}

// HandleUpdateRepo updates an existing git repo.
func (h *Handler) HandleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateRepoInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	repo, err := h.service.UpdateRepo(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("git: update repo error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitUpdateFailed, err)
		return
	}
	if repo == nil {
		response.NotFoundCode(w, r, i18n.ErrGitNotFound)
		return
	}

	response.OK(w, repo)
}

// HandleDeleteRepo removes a git repo.
func (h *Handler) HandleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.DeleteRepo(r.Context(), id); err != nil {
		h.app.Logger.Error("git: delete repo error", "error", err, "id", id)
		response.NotFoundCause(w, r, i18n.ErrGitNotFound, err)
		return
	}

	response.NoContent(w)
}

// HandleSync triggers a git sync for a repository.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo, err := h.service.RepoByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("git: sync lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGitFailed, err)
		return
	}
	if repo == nil {
		response.NotFoundCode(w, r, i18n.ErrGitNotFound)
		return
	}

	result, syncErr := h.service.Sync(r.Context(), repo, h.app.Config.DataDir)
	if syncErr != nil {
		// The transport error names hosts, paths and sometimes the
		// credential that failed, so it is logged and not returned.
		h.app.Logger.Error("git: sync failed", "error", syncErr, "repoId", id, "branch", repo.Branch)
		if markErr := h.service.MarkSynced(r.Context(), id, syncErr.Error()); markErr != nil {
			h.app.Logger.Error("git: recording sync failure", "error", markErr, "repoId", id)
		}
		if _, depErr := h.service.CreateDeployment(r.Context(), id, "", repo.Branch, DeploymentFailure, "Sync failed"); depErr != nil {
			h.app.Logger.Error("git: create deployment error", "error", depErr, "repoId", id)
		}
		if errors.Is(syncErr, ErrAuthUnsupported) {
			response.BadRequestCode(w, r, i18n.ErrGitAuthUnsupported)
			return
		}
		response.InternalErrorCause(w, r, i18n.ErrGitSyncFailed, syncErr)
		return
	}

	if err := h.service.MarkSynced(r.Context(), id, ""); err != nil {
		h.app.Logger.Error("git: marking synced", "error", err, "repoId", id)
	}
	deployment, deployErr := h.service.CreateDeployment(r.Context(),
		id, result.CommitSHA, result.Branch, DeploymentSuccess, "Sync completed")
	if deployErr != nil {
		h.app.Logger.Error("git: create deployment error", "error", deployErr, "repoId", id)
		response.InternalErrorCause(w, r, i18n.ErrGitFailed, deployErr)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "git_sync",
		EntityType: "git_repo",
		EntityID:   id,
		EntityName: repo.Name,
		Details:    "branch=" + result.Branch + " commit=" + result.CommitSHA,
	})

	response.OK(w, map[string]any{
		"repo":       repo,
		"deployment": deployment,
		"commitSha":  result.CommitSHA,
	})
}

// HandleListDeployments returns deployments for a repo.
func (h *Handler) HandleListDeployments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListDeployments(r.Context(), id, page, perPage)
	if err != nil {
		h.app.Logger.Error("git: list deployments error", "error", err, "repoId", id)
		response.InternalErrorCause(w, r, i18n.ErrGitListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
