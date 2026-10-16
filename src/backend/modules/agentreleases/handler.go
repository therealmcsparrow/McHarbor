// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the release management routes under /updates/agents/releases.
// The routes are registered by the updates module, beside the agent sweep they feed.
type Handler struct {
	app   *router.AppDeps
	store *Store
}

// NewHandler returns a Handler over the shared store. A nil store answers every
// route with a service-unavailable code instead of panicking.
func NewHandler(app *router.AppDeps, store *Store) *Handler {
	return &Handler{app: app, store: store}
}

// ActionResponse is the body of a release action: a translated message and the
// release it touched.
type ActionResponse struct {
	Message string  `json:"message"`
	Code    string  `json:"code"`
	Release Release `json:"release"`
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return nil, false
	}
	if h.store == nil {
		response.ServiceUnavailableCode(w, r, i18n.ErrAgentReleaseUnavailable)
		return nil, false
	}
	return user, true
}

// HandleList serves GET /updates/agents/releases: stored releases, the target,
// the last check and the settings, in one read for the settings page.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.ready(w, r); !ok {
		return
	}
	overview, err := h.store.Overview(r.Context())
	if err != nil {
		h.app.Logger.Error("agent-releases: overview", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentReleaseListFailed, err)
		return
	}
	response.OK(w, overview)
}

// HandleCheck serves POST /updates/agents/releases/check: ask the sources which
// version is newest. It downloads nothing.
func (h *Handler) HandleCheck(w http.ResponseWriter, r *http.Request) {
	user, ok := h.ready(w, r)
	if !ok {
		return
	}
	st, err := h.store.Check(r.Context())
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "agent_release_check", EntityType: "agent_release", EntityID: st.LatestVersion, EntityName: "mcharbor-agent",
		Details: fmt.Sprintf("Agent release check by %s: latest %q from %q", user.Username, st.LatestVersion, st.LatestSource),
	})
	if err != nil {
		h.app.Logger.Warn("agent-releases: check failed", "error", err)
		response.ErrCodeWithData(w, r, http.StatusBadGateway, i18n.ErrAgentReleaseCheckFailed, st)
		return
	}
	response.OK(w, st)
}

// HandlePull serves POST /updates/agents/releases/{version}/pull, with an optional
// ?build= to fetch that build rather than the newest. The pull runs in the
// background and lands beside the stored builds; once verified it becomes the target.
func (h *Handler) HandlePull(w http.ResponseWriter, r *http.Request) {
	user, ok := h.ready(w, r)
	if !ok {
		return
	}
	version := strings.TrimPrefix(chi.URLParam(r, "version"), "v")
	build := strings.TrimSpace(r.URL.Query().Get("build"))
	if err := h.store.StartPull(r.Context(), version, build, user.Username, true); err != nil {
		h.writeError(w, r, err, i18n.ErrAgentReleasePullFailed)
		return
	}
	label := version
	if build != "" {
		label += " build " + build
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "agent_release_pull", EntityType: "agent_release", EntityID: version, EntityName: "mcharbor-agent " + version,
		Details: fmt.Sprintf("Agent release %s download started by %s", label, user.Username),
	})
	rel, err := h.store.Get(r.Context(), version, build)
	if err != nil {
		rel = Release{Version: version, Status: StatusPulling}
	}
	h.respond(w, r, i18n.MsgAgentReleasePullStarted, rel)
}

// HandleDelete serves DELETE /updates/agents/releases/{version}, with ?build=
// naming which stored build of the version goes.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.ready(w, r)
	if !ok {
		return
	}
	version := chi.URLParam(r, "version")
	build := strings.TrimSpace(r.URL.Query().Get("build"))
	before, err := h.store.Get(r.Context(), version, build)
	if err != nil {
		h.app.Logger.Warn("agent-releases: reading release before delete", "version", version, "build", build, "error", err)
	}
	if err := h.store.Delete(r.Context(), version, build); err != nil {
		h.writeError(w, r, err, i18n.ErrAgentReleaseDeleteFailed)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "agent_release_delete", Severity: "warning", EntityType: "agent_release", EntityID: version,
		EntityName: "mcharbor-agent " + version,
		Details:    fmt.Sprintf("Agent release %s build %q (%s) deleted by %s", version, build, formatSize(before.SizeBytes), user.Username),
	})
	h.respond(w, r, i18n.MsgAgentReleaseDeleted, before)
}

// HandleSetTarget serves PUT /updates/agents/releases/target. The target is what
// installs, deploys and update sweeps serve; an empty version clears it.
func (h *Handler) HandleSetTarget(w http.ResponseWriter, r *http.Request) {
	user, ok := h.ready(w, r)
	if !ok {
		return
	}
	var body SetTargetRequest
	if err := response.DecodeBody(r, &body); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	version := strings.TrimPrefix(strings.TrimSpace(body.Version), "v")
	build := strings.TrimSpace(body.Build)
	if version != "" && !ValidVersion(version) {
		response.BadRequestCode(w, r, i18n.ErrAgentReleaseInvalidVersion)
		return
	}
	if build != "" && !buildPattern.MatchString(build) {
		response.BadRequestCode(w, r, i18n.ErrAgentReleaseInvalidVersion)
		return
	}
	previous := ""
	if rel, ok := h.store.TargetRelease(r.Context()); ok {
		previous = targetLabel(rel.Version, rel.Build)
	}
	if err := h.store.SetTarget(r.Context(), version, build); err != nil {
		h.writeError(w, r, err, i18n.ErrAgentReleaseSaveFailed)
		return
	}
	rel, ok := h.store.TargetRelease(r.Context())
	after := ""
	if version != "" {
		after = targetLabel(rel.Version, rel.Build)
		if !ok {
			h.app.Logger.Warn("agent-releases: reading release after target change", "version", version, "build", build)
		}
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "agent_release_target", EntityType: "agent_release", EntityID: version, EntityName: "mcharbor-agent",
		StateBefore: previous, StateAfter: after,
		Details: fmt.Sprintf("Agent target changed from %q to %q by %s", previous, after, user.Username),
	})
	h.respond(w, r, i18n.MsgAgentReleaseTargetSet, rel)
}

// HandleGetSettings serves GET /updates/agents/releases/settings.
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.ready(w, r); !ok {
		return
	}
	settings, err := h.store.Settings(r.Context())
	if err != nil {
		h.app.Logger.Error("agent-releases: settings", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentReleaseListFailed, err)
		return
	}
	response.OK(w, settings)
}

// HandleSaveSettings serves PUT /updates/agents/releases/settings.
func (h *Handler) HandleSaveSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.ready(w, r)
	if !ok {
		return
	}
	var in Settings
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	saved, err := h.store.SaveSettings(r.Context(), in)
	if err != nil {
		h.writeError(w, r, err, i18n.ErrAgentReleaseSaveFailed)
		return
	}
	sources := make([]string, 0, len(saved.Sources))
	for _, s := range saved.Sources {
		sources = append(sources, s.URL)
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "agent_release_settings", EntityType: "agent_release", EntityName: "mcharbor-agent",
		Details: fmt.Sprintf("Agent release settings saved by %s: mode %s, check %q, keep %d, sources %s",
			user.Username, saved.Mode, saved.CheckCron, saved.Keep, strings.Join(sources, ", ")),
	})
	response.OK(w, saved)
}

// targetLabel is a target as the audit trail shows it: the version, and its
// build when it has one.
func targetLabel(version, build string) string {
	if build == "" {
		return version
	}
	return version + " build " + build
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, rel Release) {
	response.OK(w, ActionResponse{Message: i18n.T(i18n.FromContext(r.Context()), code), Code: string(code), Release: rel})
}

// writeError maps a store error to its i18n code. Anything unrecognised is
// logged and reported as fallback, never as the raw error.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error, fallback i18n.MsgCode) {
	switch {
	case errors.Is(err, ErrInvalidVersion), errors.Is(err, ErrInvalidBuild):
		response.BadRequestCode(w, r, i18n.ErrAgentReleaseInvalidVersion)
	case errors.Is(err, ErrInvalidSettings):
		reason := "invalid"
		var se *SettingsError
		if errors.As(err, &se) {
			reason = se.Reason
		}
		response.BadRequestCodeWithData(w, r, i18n.ErrAgentReleaseInvalidSettings, map[string]string{"reason": reason})
	case errors.Is(err, ErrReleaseNotFound):
		response.NotFoundCode(w, r, i18n.ErrAgentReleaseNotFound)
	case errors.Is(err, ErrReleaseNotReady):
		response.ConflictCode(w, r, i18n.ErrAgentReleaseNotReady)
	case errors.Is(err, ErrReleaseIsTarget):
		response.ConflictCode(w, r, i18n.ErrAgentReleaseIsTarget)
	case errors.Is(err, ErrReleaseInUse):
		response.ConflictCode(w, r, i18n.ErrAgentReleaseInUse)
	case errors.Is(err, ErrReleaseBusy):
		response.ConflictCode(w, r, i18n.ErrAgentReleaseBusy)
	default:
		h.app.Logger.Error("agent-releases: request failed", "path", r.URL.Path, "error", err)
		response.InternalErrorCause(w, r, fallback, err)
	}
}
