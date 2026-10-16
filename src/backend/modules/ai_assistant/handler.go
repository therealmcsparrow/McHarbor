// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

// Handler holds dependencies for AI assistant endpoints.
type Handler struct {
	svc *Service
	app *router.AppDeps
}

// NewHandler creates a new AI assistant handler with no provider bridge:
// chat reports ErrProviderPluginNotLoaded until one is injected. main.go
// uses NewHandlerWithProviders.
func NewHandler(app *router.AppDeps) *Handler {
	return NewHandlerWithProviders(app, nil)
}

// NewHandlerWithProviders creates the handler with the AI-provider
// connection resolver injected (see providers_bridge.go).
func NewHandlerWithProviders(app *router.AppDeps, providers ProviderResolver) *Handler {
	var env EnvProvider
	if app.AIEnv != nil {
		if ep, ok := app.AIEnv.(EnvProvider); ok {
			env = ep
		}
	}
	svc := NewService(app.DB, env, providers, app.PluginAdapters)
	svc.SetRBAC(app.RBACService)
	svc.SetLogger(app.Logger)
	svc.SetAudit(app.AuditLog)
	return &Handler{app: app, svc: svc}
}

// HandleChat handles one user turn.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req ChatRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(req.Message) == "" || len(req.Message) > 16<<10 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if _, done := h.ensureUsageBudget(w, r, user.ID, "turns"); done {
		return
	}

	ctx := withUserID(r.Context(), user.ID)
	req.UserID = user.ID
	resp, err := h.svc.Chat(ctx, req)
	if err != nil {
		h.app.Logger.Error("ai chat failed", "user", user.Username, "error", err)
		if errors.Is(err, ErrEnvironmentAccessDenied) {
			response.ForbiddenCode(w, r, i18n.ErrForbidden)
			return
		}
		if errors.Is(err, ErrProviderRateLimited) {
			response.ErrCode(w, r, http.StatusTooManyRequests, i18n.ErrAIRateLimited)
			return
		}
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if err := h.svc.RecordTurn(ctx, user.ID); err != nil {
		h.app.Logger.Warn("ai usage record turn failed", "user", user.Username, "error", err)
	}

	response.OK(w, resp)
}

// HandleGetCursor returns the conversation's cursor (focus + trail +
// env scope). The frontend panel renders it directly. Phase 2 ships
// this so the panel can populate before the first chat turn.
func (h *Handler) HandleGetCursor(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	convID := r.URL.Query().Get("conversationId")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	ctx := withUserID(r.Context(), user.ID)
	cur, err := h.svc.ConversationCursor(ctx, convID)
	if err != nil {
		h.app.Logger.Error("ai cursor get failed", "user", user.Username, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, cur)
}

// HandleUpdateCursor applies a focus / env / clear update to the
// conversation's cursor. Body is a CursorUpdate; the service does the
// focus->trail move and persists the result.
func (h *Handler) HandleUpdateCursor(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req CursorUpdate
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	convID := r.URL.Query().Get("conversationId")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	ctx := withUserID(r.Context(), user.ID)
	cur, err := h.svc.UpdateCursor(ctx, convID, req)
	if err != nil {
		h.app.Logger.Error("ai cursor update failed", "user", user.Username, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, cur)
}

// HandleConsentAction applies the user's approve/deny decision.
func (h *Handler) HandleConsentAction(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chiURLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	var req ActionConsentRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	action, err := h.svc.ConsentAction(r.Context(), id, user.ID, req)
	if err != nil {
		h.app.Logger.Error("ai action consent failed", "user", user.Username, "id", id, "error", err)
		h.auditAIAction(r, "ai.action.consent_failed", id, "", "", "approved="+strconv.FormatBool(req.Approve))
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	actionName := "ai.action.denied"
	if req.Approve {
		actionName = "ai.action.approved"
	}
	h.auditAIAction(r, actionName, action.ID, action.Tool, planEnvID(action.PlanJSON), "status="+string(action.Status))
	response.OK(w, action)
}

// HandleExecuteAction runs an approved action. Returns 400 if the
// action is not in the approved state.
func (h *Handler) HandleExecuteAction(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chiURLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if _, done := h.ensureUsageBudget(w, r, user.ID, "actions"); done {
		return
	}
	action, err := h.svc.ExecuteAction(r.Context(), id, user.ID)
	if err != nil {
		h.app.Logger.Error("ai action execute failed", "user", user.Username, "id", id, "error", err)
		h.auditAIAction(r, "ai.action.execute_failed", id, "", "", "error=execute_failed")
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	auditName := "ai.action.executed"
	if action.Status == ActionFailed {
		auditName = "ai.action.failed"
	}
	h.auditAIAction(r, auditName, action.ID, action.Tool, planEnvID(action.PlanJSON), "status="+string(action.Status))
	if err := h.svc.RecordActionCall(r.Context(), user.ID); err != nil {
		h.app.Logger.Warn("ai usage record action failed", "user", user.Username, "error", err)
	}
	response.OK(w, action)
}

// HandleGetAction returns a single Action by id. The frontend uses
// this to refresh the consent dialog after a server-side change.
func (h *Handler) HandleGetAction(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := chiURLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	action, err := h.svc.getAction(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("ai action get failed", "user", user.Username, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if action == nil {
		response.Err(w, http.StatusNotFound, "action not found")
		return
	}
	if action.UserID != user.ID {
		response.Err(w, http.StatusForbidden, "not your action")
		return
	}
	response.OK(w, action)
}

// HandleListActions returns the user's recent actions, newest first.
// The frontend uses this to render the history panel.
func (h *Handler) HandleListActions(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v := parseIntDefault(l, 50); v > 0 && v <= 200 {
			limit = v
		}
	}
	actions, err := h.svc.listActions(r.Context(), user.ID, limit)
	if err != nil {
		h.app.Logger.Error("ai action list failed", "user", user.Username, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, actions)
}

// HandleListSkills returns system + user skills. Available to any
// authenticated user with ai_assistant.view so the panel can render
// the skill picker.
func (h *Handler) HandleListSkills(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	skills, err := h.svc.ListSkills(r.Context())
	if err != nil {
		h.app.Logger.Error("ai skills list failed", "user", user.Username, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, skills)
}

// HandleCreateSkill inserts a user-defined skill. The system flag
// is forced off in the service.
func (h *Handler) HandleCreateSkill(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in SkillInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	skill, err := h.svc.CreateSkill(r.Context(), in, user.ID)
	if err != nil {
		h.app.Logger.Error("ai skill create failed", "user", user.Username, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.Created(w, skill)
}

// HandleUpdateSkill modifies a user-defined skill. System skills
// return 403.
func (h *Handler) HandleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := chiURLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	var in SkillInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	skill, err := h.svc.UpdateSkill(r.Context(), id, in)
	if err != nil {
		h.app.Logger.Error("ai skill update failed", "user", user.Username, "id", id, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.OK(w, skill)
}

// HandleDeleteSkill removes a user-defined skill.
func (h *Handler) HandleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := chiURLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.svc.DeleteSkill(r.Context(), id); err != nil {
		h.app.Logger.Error("ai skill delete failed", "user", user.Username, "id", id, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.OK(w, map[string]bool{"deleted": true})
}

// HandleSearchKnowledge exposes a quick admin search over the
// embeddings index. Useful for tuning; the model uses
// knowledge.search internally.
func (h *Handler) HandleSearchKnowledge(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	topK := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if v := parseIntDefault(l, 5); v > 0 && v <= 20 {
			topK = v
		}
	}
	hits, err := h.svc.SearchKnowledge(r.Context(), q, topK)
	if err != nil {
		h.app.Logger.Error("ai knowledge search failed", "user", user.Username, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]any{
		"query":   q,
		"topK":    topK,
		"results": hits,
	})
}

// HandleSeedKnowledge runs the default knowledge seeder. Idempotent
// (skips rows that already exist with the same source+ref_id).
// Available to managers only; intended for first-run / bootstrap.
func (h *Handler) HandleSeedKnowledge(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	if err := h.svc.SeedDefaultKnowledge(r.Context()); err != nil {
		h.app.Logger.Error("ai knowledge seed failed", "user", user.Username, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, map[string]bool{"seeded": true})
}

// HandleShareConversation grants view/comment access to a user or
// role. Owner-only; non-owners get 403.
func (h *Handler) HandleShareConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	convID := chiURLParam(r, "id")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	var req ShareRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	share, err := h.svc.ShareConversation(r.Context(), convID, req, user.ID)
	if err != nil {
		h.app.Logger.Error("ai share failed", "user", user.Username, "conv", convID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.Created(w, share)
}

// HandleUnshareConversation removes a share entry. Owner-only.
func (h *Handler) HandleUnshareConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	convID := chiURLParam(r, "id")
	target := chiURLParam(r, "target")
	if convID == "" || target == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.svc.UnshareConversation(r.Context(), convID, target, user.ID); err != nil {
		h.app.Logger.Error("ai unshare failed", "user", user.Username, "conv", convID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.OK(w, map[string]bool{"deleted": true})
}

// HandleListShares returns every share entry for a conversation.
// Owner-only; a non-owner gets an empty list rather than a 403 so
// the panel can render the (no shares yet) state cleanly.
func (h *Handler) HandleListShares(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	convID := chiURLParam(r, "id")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	ctx := withUserID(r.Context(), user.ID)
	shares, err := h.svc.ListShares(ctx, convID)
	if err != nil {
		h.app.Logger.Error("ai share list failed", "user", user.Username, "conv", convID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, shares)
}

// HandlePinConversation pins a conversation to the top of the list.
func (h *Handler) HandlePinConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	convID := chiURLParam(r, "id")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.svc.PinConversation(r.Context(), convID, user.ID); err != nil {
		h.app.Logger.Error("ai pin failed", "user", user.Username, "conv", convID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.OK(w, map[string]bool{"pinned": true})
}

// HandleUnpinConversation unpins a conversation.
func (h *Handler) HandleUnpinConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	convID := chiURLParam(r, "id")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.svc.UnpinConversation(r.Context(), convID, user.ID); err != nil {
		h.app.Logger.Error("ai unpin failed", "user", user.Username, "conv", convID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	response.OK(w, map[string]bool{"pinned": false})
}

// HandleExportConversation returns a self-contained bundle for the
// conversation. ?format=markdown returns a rendered .md body
// (Content-Type: text/markdown) so the browser can download it
// directly; default is JSON.
func (h *Handler) HandleExportConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	convID := chiURLParam(r, "id")
	if convID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	format := ExportJSON
	if f := r.URL.Query().Get("format"); f == "markdown" || f == "md" {
		format = ExportMarkdown
	}
	ctx := withUserID(r.Context(), user.ID)
	bundle, err := h.svc.ExportConversation(ctx, convID, format)
	if err != nil {
		h.app.Logger.Error("ai export failed", "user", user.Username, "conv", convID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if format == ExportMarkdown {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="conversation-`+convID+`.md"`)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(bundle.Markdown)); err != nil {
			h.app.Logger.Warn("ai export: writing markdown response failed", "conv", convID, "error", err)
		}
		return
	}
	response.OK(w, bundle)
}

// chiURLParam reads a path parameter from the chi route context.
// Falls back to scanning r.URL.Path so the handler also works under
// test routers that don't go through chi.
func chiURLParam(r *http.Request, key string) string {
	if v := chi.URLParam(r, key); v != "" {
		return v
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, p := range parts {
		if p == key && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// ctxAuditHook is the request-less counterpart of auditHook, used by the
// chat-driven execution path where the only handle on the caller is the
// context. Entries carry the actor but no client IP.
type ctxAuditHook struct {
	logger *audit.Logger
	user   *auth.User
}

func (h ctxAuditHook) LogToolCall(req ai_tools.CallRequest, result *ai_tools.CallResult, dur time.Duration) {
	if h.logger == nil {
		return
	}
	entry := audit.Entry{
		Action:        "ai.tool.call",
		EntityType:    "ai_assistant",
		EntityName:    req.Name,
		EntityID:      req.Scope.ConversationID,
		EnvironmentID: req.Scope.EnvID,
		Details: "duration_ms=" + strconv.FormatInt(dur.Milliseconds(), 10) +
			" bytes=" + strconv.Itoa(result.Bytes) +
			" truncated=" + strconv.FormatBool(result.Truncated),
	}
	if h.user != nil {
		h.logger.LogWithUser(nil, h.user.ID, h.user.Username, entry)
		return
	}
	h.logger.Log(nil, entry)
}

func (h *Handler) auditAIAction(r *http.Request, actionName, id, tool, envID, details string) {
	if h.app.AuditLog == nil {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        actionName,
		EntityType:    "ai_assistant_action",
		EntityID:      id,
		EntityName:    tool,
		EnvironmentID: envID,
		Details:       details,
	})
}

// --- Phase 7 usage caps ---

// ensureUsageBudget loads the caller's usage row and verifies the
// supplied counter against DefaultCaps. Returns the row, or writes
// a 429 + ErrUsageCapExceeded to the response when the user is over
// the cap. The bool is true when a response has been written.
//
// The caller should NOT have written any body yet when this is
// invoked — the existing handlers gate auth + body decode, then
// call this before doing any work.
func (h *Handler) ensureUsageBudget(w http.ResponseWriter, r *http.Request, userID string, field string) (*UsageRow, bool) {
	row, err := h.svc.LoadUsage(r.Context(), userID)
	if err != nil {
		// Fail-open: a missing usage row is not an outage. A real DB
		// failure warrants a 500.
		h.app.Logger.Error("ai usage load failed", "user", userID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return nil, true
	}
	caps := DefaultCaps()
	// Project the per-field cap depending on which counter the
	// caller is about to bump, so a tool-call doesn't block on the
	// turns cap and vice-versa.
	check := UsageCaps{
		DailyTurns:       caps.DailyTurns,
		DailyToolCalls:   caps.DailyToolCalls,
		DailyActionCalls: caps.DailyActionCalls,
		DailyTokensIn:    caps.DailyTokensIn,
		DailyTokensOut:   caps.DailyTokensOut,
	}
	switch field {
	case "turns":
		check.DailyToolCalls = 0
		check.DailyActionCalls = 0
		check.DailyTokensIn = 0
		check.DailyTokensOut = 0
	case "tool_calls":
		check.DailyTurns = 0
		check.DailyActionCalls = 0
		check.DailyTokensIn = 0
		check.DailyTokensOut = 0
	case "actions":
		check.DailyTurns = 0
		check.DailyToolCalls = 0
		check.DailyTokensIn = 0
		check.DailyTokensOut = 0
	}
	if err := row.CheckUsage(check); err != nil {
		response.ErrCode(w, r, http.StatusTooManyRequests, i18n.ErrRateLimitExceed)
		return nil, true
	}
	return row, false
}
