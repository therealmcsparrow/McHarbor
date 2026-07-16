// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// Policy represents an auto-update policy.
type Policy struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ContainerMatch string `json:"containerMatch"` // glob or regex pattern for container names
	ImageMatch     string `json:"imageMatch"`     // glob or regex pattern for image names
	Schedule       string `json:"schedule"`       // cron expression
	Strategy       string `json:"strategy"`       // latest, semver, digest
	AutoRestart    bool   `json:"autoRestart"`
	Enabled        bool   `json:"enabled"`
	LastRunAt      string `json:"lastRunAt"`
	LastRunStatus  string `json:"lastRunStatus"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// UpdateHistory represents a single update execution record.
type UpdateHistory struct {
	ID         string `json:"id"`
	PolicyID   string `json:"policyId"`
	Container  string `json:"container"`
	OldImage   string `json:"oldImage"`
	NewImage   string `json:"newImage"`
	Status     string `json:"status"` // success, failed, skipped
	Message    string `json:"message"`
	ExecutedAt string `json:"executedAt"`
}

// CreatePolicyInput is the request body for creating a policy.
type CreatePolicyInput struct {
	Name           string `json:"name"`
	ContainerMatch string `json:"containerMatch"`
	ImageMatch     string `json:"imageMatch"`
	Schedule       string `json:"schedule"`
	Strategy       string `json:"strategy"`
	AutoRestart    bool   `json:"autoRestart"`
}

// UpdatePolicyInput is the request body for updating a policy.
type UpdatePolicyInput struct {
	Name           *string `json:"name"`
	ContainerMatch *string `json:"containerMatch"`
	ImageMatch     *string `json:"imageMatch"`
	Schedule       *string `json:"schedule"`
	Strategy       *string `json:"strategy"`
	AutoRestart    *bool   `json:"autoRestart"`
	Enabled        *bool   `json:"enabled"`
}

// Handler holds dependencies for update HTTP handlers.
type Handler struct {
	app        *router.AppDeps
	service   *Service
	selfUpdate *SelfUpdateChecker
}

// NewHandler creates a new updates handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc, selfUpdate: nil}
}

// SetSelfUpdateChecker injects the background self-update
// checker. Called from Mount() so the AppDeps wiring is complete
// before the checker is built.
func (h *Handler) SetSelfUpdateChecker(c *SelfUpdateChecker) {
	h.selfUpdate = c
}

// HandleList returns a paginated list of update policies.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("updates: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a new update policy.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreatePolicyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrUpdateNameRequired)
		return
	}

	policy, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("updates: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateCreateFailed)
		return
	}

	response.Created(w, policy)
}

// HandleGet returns a single update policy.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	policy, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("updates: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}
	if policy == nil {
		response.NotFoundCode(w, r, i18n.ErrUpdateNotFound)
		return
	}

	response.OK(w, policy)
}

// HandleUpdate updates an existing policy.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdatePolicyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	policy, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("updates: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrUpdateCreateFailed)
		return
	}
	if policy == nil {
		response.NotFoundCode(w, r, i18n.ErrUpdateNotFound)
		return
	}

	response.OK(w, policy)
}

// HandleDelete removes an update policy.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("updates: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrUpdateRemoveFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrUpdateNotFound)
		return
	}

	response.NoContent(w)
}

// githubRelease is a subset of the GitHub release API response.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

// VersionCheck is the response for the check-update endpoint.
type VersionCheck struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	PublishedAt     string `json:"publishedAt,omitempty"`
	ReleaseNotes    string `json:"releaseNotes,omitempty"`
}

// HandleCheckUpdate checks GitHub for a newer McHarbor release.
func (h *Handler) HandleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	currentVersion := appversion.Current()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/therealmcsparrow/mcharbor/releases/latest", nil)
	if err != nil {
		h.app.Logger.Error("updates: build github request", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateCheckFailed)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "McHarbor/"+currentVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.app.Logger.Warn("updates: github unreachable", "error", err)
		response.ErrCode(w, r, http.StatusBadGateway, i18n.ErrUpdateCheckFailed)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.app.Logger.Warn("updates: github returned non-200", "status", resp.StatusCode)
		response.ErrCode(w, r, http.StatusBadGateway, i18n.ErrUpdateCheckFailed)
		return
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		h.app.Logger.Error("updates: decode github response", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateCheckFailed)
		return
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	notes := rel.Body
	if len(notes) > 2000 {
		notes = notes[:2000]
	}

	response.OK(w, VersionCheck{
		CurrentVersion:  currentVersion,
		LatestVersion:   latest,
		UpdateAvailable: latest != currentVersion && compareVersions(latest, currentVersion) > 0,
		ReleaseURL:      rel.HTMLURL,
		PublishedAt:     rel.PublishedAt,
		ReleaseNotes:    notes,
	})
}

// compareVersions does a simple semver comparison (a > b → 1, a == b → 0, a < b → -1).
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var va, vb int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &va)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &vb)
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

// HandleState returns the cached self-update check result. The
// periodic checker writes to the settings table; this endpoint
// just reads it back so the frontend can poll for a new release
// without hammering the GitHub API.
func (h *Handler) HandleState(w http.ResponseWriter, r *http.Request) {
	settings, err := h.selfUpdate.Settings(r.Context())
	if err != nil {
		h.app.Logger.Error("self-update: load settings for state", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}
	state := h.selfUpdate.State()
	if state.LatestVersion == "" {
		// Force a one-shot check so the first poll returns something
		// actionable rather than a perpetual "checking..." spinner.
		if settings.Enabled {
			if s, err := h.selfUpdate.CheckNow(r.Context()); err == nil {
				state = s
			}
		}
	}
	state.IntervalHours = settings.IntervalHours
	response.OK(w, state)
}

// HandleGetSettings returns the operator-facing self-update config.
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.selfUpdate.Settings(r.Context())
	if err != nil {
		h.app.Logger.Error("self-update: load settings", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}
	response.OK(w, settings)
}

// HandleSaveSettings persists the operator-facing self-update
// configuration. A zero `intervalHours` resets to the default
// (24h); values are clamped to the [1, 168] hour range.
func (h *Handler) HandleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in SelfUpdateSettings
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	saved, err := h.selfUpdate.SaveSettings(r.Context(), in)
	if err != nil {
		h.app.Logger.Error("self-update: save settings", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}
	// Restart the ticker so the new interval takes effect
	// immediately rather than at the next scheduled tick.
	if in.Enabled {
		h.selfUpdate.Restart(r.Context())
	}
	response.OK(w, saved)
}

// HandleDismiss records that the operator has seen the current
// latest version so the in-app banner stops highlighting it.
func (h *Handler) HandleDismiss(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
	}
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.selfUpdate.Dismiss(r.Context(), req.Version); err != nil {
		h.app.Logger.Error("self-update: dismiss", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}
	response.OK(w, map[string]any{"ok": true})
}

// HandleHistory returns update execution history for a policy.
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.History(r.Context(), policyID, page, perPage)
	if err != nil {
		h.app.Logger.Error("updates: history list error", "error", err, "policyId", policyID)
		response.InternalErrorCode(w, r, i18n.ErrUpdateListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
