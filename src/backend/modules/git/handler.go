// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"net/http"

	"github.com/go-chi/chi/v5"

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

	items, total, err := h.service.ListRepos(page, perPage)
	if err != nil {
		h.app.Logger.Error("git: list repos error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGitListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGetRepo returns a single git repo.
func (h *Handler) HandleGetRepo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo, err := h.service.RepoByID(id)
	if err != nil {
		h.app.Logger.Error("git: get repo error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGitFailed)
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
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
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

	repo, err := h.service.CreateRepo(input)
	if err != nil {
		h.app.Logger.Error("git: create repo error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGitCreateFailed)
		return
	}

	response.Created(w, repo)
}

// HandleUpdateRepo updates an existing git repo.
func (h *Handler) HandleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateRepoInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	repo, err := h.service.UpdateRepo(id, input)
	if err != nil {
		h.app.Logger.Error("git: update repo error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGitUpdateFailed)
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

	if err := h.service.DeleteRepo(id); err != nil {
		h.app.Logger.Error("git: delete repo error", "error", err, "id", id)
		response.NotFoundCode(w, r, i18n.ErrGitNotFound)
		return
	}

	response.NoContent(w)
}

// HandleSync triggers a git sync for a repository.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo, err := h.service.RepoByID(id)
	if err != nil {
		h.app.Logger.Error("git: sync lookup error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGitFailed)
		return
	}
	if repo == nil {
		response.NotFoundCode(w, r, i18n.ErrGitNotFound)
		return
	}

	// Stub: actual git clone/pull (via exec or go-git) is not yet implemented.
	// Mark as synced and create a deployment record as a placeholder.
	_ = h.service.MarkSynced(id, "")

	deployment, deployErr := h.service.CreateDeployment(id, "HEAD", repo.Branch, "pending", "Sync triggered manually")
	if deployErr != nil {
		h.app.Logger.Error("git: create deployment error", "error", deployErr, "repoId", id)
		response.InternalErrorCode(w, r, i18n.ErrGitFailed)
		return
	}

	response.OK(w, map[string]any{
		"repo":       repo,
		"deployment": deployment,
		"message":    "Sync triggered",
	})
}

// HandleListDeployments returns deployments for a repo.
func (h *Handler) HandleListDeployments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListDeployments(id, page, perPage)
	if err != nil {
		h.app.Logger.Error("git: list deployments error", "error", err, "repoId", id)
		response.InternalErrorCode(w, r, i18n.ErrGitListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
