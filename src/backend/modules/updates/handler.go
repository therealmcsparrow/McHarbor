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

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
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
	service    *Service
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a new update policy.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreatePolicyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrUpdateNameRequired)
		return
	}

	policy, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("updates: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUpdateCreateFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	policy, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("updates: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrUpdateCreateFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateRemoveFailed, err)
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
	TagName     string        `json:"tag_name"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

// VersionCheck is the response for the check-update endpoint.
type VersionCheck struct {
	CurrentVersion  string `json:"currentVersion"`
	CurrentBuild    string `json:"currentBuild,omitempty"`
	LatestVersion   string `json:"latestVersion"`
	LatestBuild     string `json:"latestBuild,omitempty"`
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateCheckFailed, err)
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
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()

	if resp.StatusCode != http.StatusOK {
		h.app.Logger.Warn("updates: github returned non-200", "status", resp.StatusCode)
		response.ErrCode(w, r, http.StatusBadGateway, i18n.ErrUpdateCheckFailed)
		return
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		h.app.Logger.Error("updates: decode github response", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUpdateCheckFailed, err)
		return
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	latestBuild := h.selfUpdate.releaseBuild(ctx, rel.Assets)
	currentBuild := appversion.Build()
	notes := rel.Body
	if len(notes) > 2000 {
		notes = notes[:2000]
	}

	response.OK(w, VersionCheck{
		CurrentVersion:  currentVersion,
		CurrentBuild:    currentBuild,
		LatestVersion:   latest,
		LatestBuild:     latestBuild,
		UpdateAvailable: releaseNewer(currentVersion, currentBuild, latest, latestBuild),
		ReleaseURL:      rel.HTMLURL,
		PublishedAt:     rel.PublishedAt,
		ReleaseNotes:    notes,
	})
}

// leadingInt parses the digits at the start of a version part and yields 0 when there are none, like "%d" scanning did.
func leadingInt(part string) int {
	var n int
	if _, err := fmt.Sscanf(part, "%d", &n); err != nil {
		return 0
	}
	return n
}

// compareVersions does a simple semver comparison (a > b → 1, a == b → 0, a < b → -1).
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var va, vb int
		if i < len(pa) {
			va = leadingInt(pa[i])
		}
		if i < len(pb) {
			vb = leadingInt(pb[i])
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
		return
	}
	response.OK(w, settings)
}

// HandleSaveSettings persists the operator-facing self-update
// configuration. A zero `intervalHours` resets to the default
// (24h); values are clamped to the [1, 168] hour range.
func (h *Handler) HandleSaveSettings(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in SelfUpdateSettings
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	// The route is gated by updates.manage; enabling an automatic
	// install (restarts McHarbor) or agent updates (restarts agents)
	// additionally needs the permission the manual action requires.
	current, err := h.selfUpdate.Settings(r.Context())
	if err != nil {
		h.app.Logger.Error("self-update: load settings before save", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
		return
	}
	if in.AutoInstall && !current.AutoInstall && !h.userHasGlobal(r, user.ID, rbac.PermSystemManage) {
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return
	}
	if in.AutoUpdateAgents && !current.AutoUpdateAgents && !h.userHasGlobal(r, user.ID, rbac.PermEnvironmentsManage) {
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return
	}
	if !ValidSchedule(in) {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "bad_cron"})
		return
	}
	saved, err := h.selfUpdate.SaveSettings(r.Context(), in)
	if err != nil {
		h.app.Logger.Error("self-update: save settings", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
		return
	}
	// Restart the ticker so the new interval takes effect
	// immediately rather than at the next scheduled tick.
	if in.Enabled {
		// The app's lifetime, not the request's: the loop must outlive this call.
		h.selfUpdate.Restart(h.app.ContextOrBackground()) //nolint:contextcheck // the loop must live as long as the app, so it cannot inherit the request context
	}
	response.OK(w, saved)
}

// userHasGlobal reports whether the user holds perm at global scope.
// The synthetic "system" user (AUTH_DISABLE) always passes, matching
// rbac.RequireGlobalPermission.
func (h *Handler) userHasGlobal(r *http.Request, userID string, perm rbac.Permission) bool {
	if userID == "system" {
		return true
	}
	if h.app.RBACService == nil {
		return false
	}
	ok, err := h.app.RBACService.HasPermission(r.Context(), userID, "", perm)
	if err != nil {
		h.app.Logger.Error("self-update: permission check failed", "permission", string(perm), "error", err)
		return false
	}
	return ok
}

// HandleAgentInventory serves GET /updates/agents.
//
// Read-only: the table behind the update buttons, so an operator can see what
// is connected, what it is running and what would be touched before touching
// anything.
func (h *Handler) HandleAgentInventory(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if h.selfUpdate == nil || h.selfUpdate.agentUpdate == nil {
		response.InternalErrorCode(w, r, i18n.ErrAgentUpdateFailed)
		return
	}

	rows, target, err := h.selfUpdate.agentUpdate.Inventory(r.Context(), h.app.DB)
	if err != nil {
		h.app.Logger.Error("self-update: agent inventory", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentUpdateFailed, err)
		return
	}

	settings, err := h.selfUpdate.Settings(r.Context())
	if err != nil {
		h.app.Logger.Error("self-update: settings for agent inventory", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentUpdateFailed, err)
		return
	}

	_, updating := h.selfUpdate.agentUpdate.RunStates()
	_, targetBuild := h.selfUpdate.agentUpdate.GlobalTarget(r.Context())
	response.OK(w, map[string]any{
		"agents":         rows,
		"targetVersion":  target,
		"targetBuild":    targetBuild,
		"updating":       updating,
		"excludedEnvIds": settings.AgentExcludedEnvIDs,
	})
}

// UpdateAgentsRequest optionally limits an agent update sweep to these
// environments; empty means every updatable agent not excluded in settings.
type UpdateAgentsRequest struct {
	EnvIDs []string `json:"envIds"`
}

// HandleUpdateAgents runs one agent update sweep now: every connected
// agent older than the target version is told to replace itself. The
// sweep bypasses the periodic retry backoff because the operator asked.
func (h *Handler) HandleUpdateAgents(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if h.selfUpdate == nil || h.selfUpdate.agentUpdate == nil {
		response.InternalErrorCode(w, r, i18n.ErrAgentUpdateFailed)
		return
	}
	settings, err := h.selfUpdate.Settings(r.Context())
	if err != nil {
		h.app.Logger.Error("self-update: load settings before agent sweep", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentUpdateFailed, err)
		return
	}
	// An optional selection. Rather than a second code path, the chosen agents
	// are expressed as an exclusion of everything else, so one sweep implementation
	// serves both buttons and the configured exclusions still apply.
	var body UpdateAgentsRequest
	if err := response.DecodeBody(r, &body); err != nil {
		h.app.Logger.Debug("self-update: agent sweep without a usable selection body", "error", err)
	}

	excluded := settings.AgentExcludedEnvIDs
	if len(body.EnvIDs) > 0 {
		chosen := make(map[string]bool, len(body.EnvIDs))
		for _, id := range body.EnvIDs {
			chosen[strings.TrimSpace(id)] = true
		}
		if rows, _, err := h.selfUpdate.agentUpdate.Inventory(r.Context(), h.app.DB); err == nil {
			for _, row := range rows {
				if !chosen[row.EnvID] {
					excluded = append(excluded, row.EnvID)
				}
			}
		}
	}

	// Started rather than awaited. Each agent's acknowledgement can take
	// minutes, and holding the response open for all of them is what made the
	// table one spinner with nothing to say. Per-agent state is published as
	// the sweep moves, and the table polls the inventory for it.
	target := h.selfUpdate.agentUpdate.TargetVersion(r.Context())
	queued := h.selfUpdate.agentUpdate.StartUpdate(r.Context(), true, excluded)

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "agents_update",
		EntityType: "agent",
		EntityID:   target,
		EntityName: "mcharbor-agent",
		Details: fmt.Sprintf("Agent update sweep started by %s: %d agent(s) queued for %s",
			user.Username, queued, target),
	})

	response.OK(w, struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Queued  int    `json:"queued"`
		Target  string `json:"targetVersion"`
	}{
		Message: i18n.T(i18n.FromContext(r.Context()), i18n.MsgAgentUpdateTriggered),
		Code:    string(i18n.MsgAgentUpdateTriggered),
		Queued:  queued,
		Target:  target,
	})
}

// DismissRequest names the release version the operator no longer wants to be told about.
type DismissRequest struct {
	Version string `json:"version"`
}

// HandleDismiss records that the operator has seen the current
// latest version so the in-app banner stops highlighting it.
func (h *Handler) HandleDismiss(w http.ResponseWriter, r *http.Request) {
	var req DismissRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if err := h.selfUpdate.Dismiss(r.Context(), req.Version); err != nil {
		h.app.Logger.Error("self-update: dismiss", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrUpdateListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// policyEnvironment is the rbac.PinEnvToResource resolver for /updates/{id}.
func (h *Handler) policyEnvironment(r *http.Request) (string, bool, error) {
	envID, found, err := h.service.PolicyEnvironment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.app.Logger.Error("updates: resolving the policy", "error", err)
	}
	return envID, found, err
}
